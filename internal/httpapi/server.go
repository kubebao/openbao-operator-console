// Package httpapi serves the typed console API and embedded frontend.
package httpapi

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
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

//go:embed all:assets
var assets embed.FS

type Server struct {
	Fleet           Fleet
	Auth            Authentication
	Settings        *settings.Service
	Demo            bool
	Origin          string
	ReaderName      string
	ReaderNamespace string
	Setup           *Setup
}

// Fleet provides the fleet as presented to viewers and its retained history. Activity
// applies the viewer's scopes before its limit.
type Fleet interface {
	Snapshot(context.Context) (fleet.Snapshot, error)
	History(context.Context, string) ([]fleet.Observation, error)
	Activity(context.Context, []config.Scope) ([]fleet.Activity, error)
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

const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"

func problem(w http.ResponseWriter, status int, code, message string) {
	jsonResponse(w, status, contract.Problem{Code: code, Message: message})
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
	mux.HandleFunc("GET /api/v1/deployments/{id}/history", s.viewer(s.readHistory))
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, _ *http.Request) {
		problem(w, http.StatusNotFound, "not_found", "Endpoint not found.")
	})
	mux.HandleFunc("GET /", frontend())
	return s.secure(mux)
}

// secure sets response headers for every route and holds the API until setup completes.
func (s *Server) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
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
			writeProblem(w, err)
		}
	}
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if _, err := s.Fleet.Snapshot(ctx); err != nil {
		http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if !s.Demo && r.Header.Get("Origin") != s.Origin {
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
		return retryLater("Fleet evidence is temporarily unavailable.")
	}
	if result.Activity, err = s.Fleet.Activity(ctx, user.Scopes); err != nil {
		return retryLater("Fleet evidence is temporarily unavailable.")
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
		return retryLater("Fleet evidence is temporarily unavailable.")
	}
	id := r.PathValue("id")
	visible := slices.ContainsFunc(result.Deployments, func(d fleet.Deployment) bool {
		return d.ID == id && user.CanView(d.ConnectionID, d.Namespace)
	})
	if !visible {
		return &problemError{status: http.StatusNotFound, code: "not_found", message: "Deployment not found."}
	}
	observations, err := s.Fleet.History(ctx, id)
	if err != nil {
		return retryLater("Observation history is temporarily unavailable.")
	}
	jsonResponse(w, http.StatusOK, contract.History{Observations: mapSlice(observations, observationContract)})
	return nil
}

// frontend serves embedded assets. Paths without a file extension fall back to the
// application shell for client-side routes.
func frontend() http.HandlerFunc {
	static, _ := fs.Sub(assets, "assets")
	files := http.FileServerFS(static)
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		if _, err := fs.Stat(static, name); err == nil {
			files.ServeHTTP(w, r)
			return
		}
		if strings.Contains(name, ".") {
			http.NotFound(w, r)
			return
		}
		index, err := fs.ReadFile(static, "index.html")
		if err != nil {
			http.Error(w, "Frontend unavailable. Run make build or use make dev.", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	}
}
