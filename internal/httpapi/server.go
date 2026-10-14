// Package httpapi serves the typed console API and embedded frontend.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/auth"
	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
	"github.com/kubebao/openbao-operator-console/internal/settings"
)

type Server struct {
	Fleet Fleet
	// Database reports readiness in live mode. Demo mode has none and is always ready.
	Database Database
	// Logger records failed requests with a classified cause. Nil discards them.
	Logger          *slog.Logger
	Auth            Authentication
	Settings        *settings.Service
	Demo            bool
	Origin          string
	ReaderName      string
	ReaderNamespace string
	Setup           *Setup
}

// Fleet provides the fleet and pages of retained history and activity. Activity applies
// the viewer's scopes before its page limit.
type Fleet interface {
	Snapshot(context.Context) (fleet.Snapshot, error)
	History(context.Context, string, fleet.HistoryQuery) (fleet.HistoryPage, error)
	Activity(context.Context, []config.Scope, fleet.Page) (fleet.ActivityPage, error)
}

// Database is the readiness view of the console database.
type Database interface {
	Ping(context.Context) error
}

type Authentication interface {
	Viewer(*http.Request) (config.User, bool)
	Login(http.ResponseWriter, *http.Request)
	Callback(http.ResponseWriter, *http.Request)
	Logout(http.ResponseWriter, *http.Request)
}

func jsonResponse[T any](w http.ResponseWriter, status int, value T) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// problem writes RFC 9457 problem details. The console defines no problem type URIs, so
// the type is about:blank and the title is the status phrase. Clients branch on code.
func problem(w http.ResponseWriter, status int, code contract.ProblemCode, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(contract.Problem{
		Type:   contract.AboutBlank,
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
		Code:   code,
	})
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.setupRoutes(mux)
	s.settingsRoutes(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /readyz", s.ready)
	mux.HandleFunc("GET /auth/login", s.Auth.Login)
	mux.HandleFunc("GET /auth/callback", s.Auth.Callback)
	mux.HandleFunc("POST /auth/logout", s.logout)
	mux.HandleFunc("GET /api/v1/snapshot", s.viewer(s.readSnapshot))
	mux.HandleFunc("GET /api/v1/activity", s.viewer(s.readActivity))
	mux.HandleFunc("GET /api/v1/deployments/{id}/history", s.viewer(s.readHistory))
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, _ *http.Request) {
		problem(w, http.StatusNotFound, "not_found", "Endpoint not found.")
	})
	mux.HandleFunc("GET /", frontend(embeddedAssets()))
	return s.secure(mux)
}

// secure sets response headers for every route and holds the API until setup completes.
// An HTTPS public origin also enables HSTS for the console host only; subdomains and
// preload lists are left to the operator of the domain.
func (s *Server) secure(next http.Handler) http.Handler {
	strictTransport := strings.HasPrefix(canonicalOrigin(s.Origin), "https://")
	policy := contentSecurityPolicy("")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", policy)
		if strictTransport {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		if s.Setup != nil && !s.Settings.Initialized() && strings.HasPrefix(r.URL.Path, "/api/v1/") &&
			!strings.HasPrefix(r.URL.Path, "/api/v1/setup") {
			problem(w, http.StatusServiceUnavailable, "setup_required", "Complete console setup before using this API.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// viewerHandler serves a signed-in viewer. It writes its own success response and returns
// failures for viewer to map.
type viewerHandler func(w http.ResponseWriter, r *http.Request, user config.User) error

func (s *Server) viewer(handler viewerHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, allowed := s.Auth.Viewer(r)
		if !allowed {
			problem(w, http.StatusUnauthorized, "unauthenticated", "Sign in to view this fleet.")
			return
		}
		if err := handler(w, r, user); err != nil {
			s.fail(w, r, err)
		}
	}
}

// ready is a public probe, so it stays cheap: live mode needs loaded settings and a
// database that accepts connections. It reads no fleet evidence.
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if s.Demo {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if s.Settings == nil || s.Database == nil {
		http.Error(w, "Not ready", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.Database.Ping(ctx); err != nil {
		http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if !s.Demo && !s.sameOrigin(r) {
		http.Error(w, "Invalid origin", http.StatusForbidden)
		return
	}
	s.Auth.Logout(w, r)
}

func (s *Server) readSnapshot(w http.ResponseWriter, r *http.Request, user config.User) error {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	result, err := s.Fleet.Snapshot(ctx)
	if err != nil {
		return unavailable(err, "Fleet evidence is temporarily unavailable.")
	}
	jsonResponse(w, http.StatusOK, snapshotContract(auth.Scope(result, user), user, s.Demo))
	return nil
}

// readHistory authorizes the deployment against the current fleet before reading history,
// so a hidden or unenrolled deployment is indistinguishable from a missing one.
func (s *Server) readHistory(w http.ResponseWriter, r *http.Request, user config.User) error {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	result, err := s.Fleet.Snapshot(ctx)
	if err != nil {
		return unavailable(err, "Fleet evidence is temporarily unavailable.")
	}
	id := r.PathValue("id")
	visible := slices.ContainsFunc(result.Deployments, func(d fleet.Deployment) bool {
		return d.ID == id && user.CanView(d.ConnectionID, d.Namespace)
	})
	if !visible {
		return &problemError{status: http.StatusNotFound, code: "not_found", message: "Deployment not found."}
	}
	list := historyList(id, r.URL.Query().Get("since"))
	query, err := historyQuery(r.URL.Query(), list)
	if err != nil {
		return err
	}
	page, err := s.Fleet.History(ctx, id, query)
	if err != nil {
		return unavailable(err, "Observation history is temporarily unavailable.")
	}
	nextCursor, err := encodeCursor(list, page.Next)
	if err != nil {
		return unavailable(err, "Observation history is temporarily unavailable.")
	}
	jsonResponse(w, http.StatusOK, contract.History{
		Observations: mapSlice(page.Observations, observationContract),
		Baseline:     optional(page.Baseline, observationContract),
		NextCursor:   nextCursor,
	})
	return nil
}
