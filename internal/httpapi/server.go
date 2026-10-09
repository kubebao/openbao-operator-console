// Package httpapi serves the typed console API and embedded frontend.
package httpapi

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
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

// Fleet provides the fleet as presented to viewers and its retained history.
type Fleet interface {
	Snapshot(context.Context) (fleet.Snapshot, error)
	History(context.Context, string) ([]fleet.Observation, error)
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
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if _, err := s.Fleet.Snapshot(ctx); err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /auth/login", s.Auth.Login)
	mux.HandleFunc("GET /auth/callback", s.Auth.Callback)
	mux.HandleFunc("POST /auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if !s.Demo && r.Header.Get("Origin") != s.Origin {
			http.Error(w, "Invalid origin", http.StatusForbidden)
			return
		}
		s.Auth.Logout(w, r)
	})
	mux.HandleFunc("GET /api/v1/snapshot", func(w http.ResponseWriter, r *http.Request) {
		user, allowed := s.Auth.Viewer(r)
		if !allowed {
			problem(w, http.StatusUnauthorized, "unauthenticated", "Sign in to view this fleet.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		result, err := s.Fleet.Snapshot(ctx)
		if err != nil {
			problem(w, http.StatusServiceUnavailable, "unavailable", "Fleet evidence is temporarily unavailable.")
			return
		}
		jsonResponse(w, http.StatusOK, snapshotContract(auth.Scope(result, user), user, s.Demo))
	})
	mux.HandleFunc("GET /api/v1/deployments/{id}/history", func(w http.ResponseWriter, r *http.Request) {
		user, allowed := s.Auth.Viewer(r)
		if !allowed {
			problem(w, http.StatusUnauthorized, "unauthenticated", "Sign in to view this fleet.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		result, err := s.Fleet.Snapshot(ctx)
		if err != nil {
			problem(w, http.StatusServiceUnavailable, "unavailable", "Fleet evidence is temporarily unavailable.")
			return
		}
		id := r.PathValue("id")
		visible := false
		for _, d := range result.Deployments {
			if d.ID == id && user.CanView(d.ConnectionID, d.Namespace) {
				visible = true
				break
			}
		}
		if !visible {
			problem(w, http.StatusNotFound, "not_found", "Deployment not found.")
			return
		}
		observations, err := s.Fleet.History(ctx, id)
		if err != nil {
			problem(w, http.StatusServiceUnavailable, "unavailable", "Observation history is temporarily unavailable.")
			return
		}
		jsonResponse(w, http.StatusOK, contract.History{Observations: mapSlice(observations, observationContract)})
	})
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, _ *http.Request) {
		problem(w, http.StatusNotFound, "not_found", "Endpoint not found.")
	})
	static, _ := fs.Sub(assets, "assets")
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		if _, err := fs.Stat(static, name); err == nil {
			http.FileServerFS(static).ServeHTTP(w, r)
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
	})
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
		mux.ServeHTTP(w, r)
	})
}
