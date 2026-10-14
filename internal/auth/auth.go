// Package auth authenticates viewers and applies administrator-controlled scopes.
package auth

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

const (
	sessionCookie = "__Host-console-session"
	stateCookie   = "__Host-console-state"
	// maxReturnTo bounds the local return path carried in the login cookie.
	maxReturnTo = 512
)

// Resolver maps a verified principal to current console access.
type Resolver func(context.Context, config.Principal) (config.User, bool)

// Activation runs after a verified trial or setup login and before the session opens.
type Activation func(context.Context, config.Principal) error

// Authenticator signs viewers in. It runs in one of three modes: a fixed demonstration
// viewer, pending browser setup without a provider, or OIDC with a verified provider.
// Access is resolved on every request, so access changes apply to existing sessions.
type Authenticator struct {
	// Logger records failed sign-ins as security events. Nil discards them.
	Logger *slog.Logger

	demo    *config.User
	resolve Resolver
	logins  *store
	sealer  *sealer
	now     func() time.Time

	mu       sync.Mutex
	provider *Provider // nil while setup is pending
}

// NewDemo signs every request in as a fixed, scoped demonstration viewer.
func NewDemo(viewer config.User) *Authenticator { return &Authenticator{demo: &viewer} }

// NewPending rejects fleet access until browser setup installs a verified provider.
func NewPending(resolve Resolver) *Authenticator {
	return &Authenticator{resolve: resolve, logins: newStore(), sealer: newSealer(), now: time.Now}
}

// New signs viewers in through provider.
func New(provider *Provider, resolve Resolver) *Authenticator {
	authenticator := NewPending(resolve)
	authenticator.provider = provider
	return authenticator
}

func (a *Authenticator) activeProvider() *Provider {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.provider
}

// ReplaceProvider activates a verified provider and ends every session and pending login.
func (a *Authenticator) ReplaceProvider(provider *Provider) {
	a.mu.Lock()
	a.provider = provider
	a.mu.Unlock()
	a.logins.reset()
}

func (a *Authenticator) Viewer(request *http.Request) (config.User, bool) {
	if a.demo != nil {
		return *a.demo, true
	}
	cookie, err := request.Cookie(sessionCookie)
	if err != nil {
		return config.User{}, false
	}
	active, ok := a.logins.lookup(cookie.Value)
	if !ok {
		return config.User{}, false
	}
	return a.resolve(request.Context(), active.principal)
}

func safeReturn(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || len(value) > maxReturnTo || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") ||
		parsed.Host != "" || parsed.IsAbs() ||
		strings.ContainsAny(value, "\\\r\n") ||
		strings.HasPrefix(parsed.Path, "/auth/") {
		return "/"
	}
	return value
}

// begin starts a PKCE login with a nonce against provider and returns the provider's
// authorization URL. The login is sealed into the state cookie; only a candidate
// provider trial keeps server-side state.
func (a *Authenticator) begin(w http.ResponseWriter, provider *Provider, returnTo, trialID string) (string, error) {
	state, err := randomToken()
	if err != nil {
		return "", err
	}
	nonce, err := randomToken()
	if err != nil {
		return "", err
	}
	verifier := oauth2.GenerateVerifier()
	value, err := a.sealer.seal(pendingLogin{
		State:      state,
		Verifier:   verifier,
		Nonce:      nonce,
		ReturnTo:   safeReturn(returnTo),
		Expires:    a.now().Add(loginLifetime).Unix(),
		Generation: a.logins.current(),
		Trial:      trialID,
	})
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookie,
		Value:    value,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(loginLifetime.Seconds()),
	})
	return provider.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce)), nil
}

func (a *Authenticator) Login(w http.ResponseWriter, request *http.Request) {
	if a.demo != nil {
		http.Redirect(w, request, "/", http.StatusSeeOther)
		return
	}
	provider := a.activeProvider()
	if provider == nil {
		http.Redirect(w, request, "/setup", http.StatusSeeOther)
		return
	}
	location, err := a.begin(w, provider, request.URL.Query().Get("returnTo"), "")
	if err != nil {
		http.Redirect(w, request, "/?signin="+string(SignInUnavailable), http.StatusSeeOther)
		return
	}
	// The location is the provider's authorization URL. returnTo stays in the sealed
	// login cookie after safeReturn limits it to a local path.
	http.Redirect(w, request, location, http.StatusFound) //nolint:gosec // Provider URL, not request input.
}

// beginTrial starts a sign-in through a candidate provider. The candidate and its
// activation step stay on the server in a small bounded set.
func (a *Authenticator) beginTrial(
	w http.ResponseWriter,
	candidate *Provider,
	returnTo string,
	setup bool,
	activate Activation,
) (string, error) {
	id, err := a.logins.beginTrial(trial{provider: candidate, activate: activate, setup: setup})
	if err != nil {
		return "", err
	}
	return a.begin(w, candidate, returnTo, id)
}

// BeginTrial binds a candidate identity provider to an administrator-initiated
// browser flow. Configuration changes only after a verified administrator login.
func (a *Authenticator) BeginTrial(
	w http.ResponseWriter,
	candidate *Provider,
	activate Activation,
) (string, error) {
	return a.beginTrial(w, candidate, "/settings?section=identity", false, activate)
}

// BeginSetup uses the normal signed OIDC flow, with a first-claim callback and
// bounded failure destinations that never include provider errors or secrets.
func (a *Authenticator) BeginSetup(
	w http.ResponseWriter,
	candidate *Provider,
	activate Activation,
) (string, error) {
	return a.beginTrial(w, candidate, "/settings/clusters/new", true, activate)
}

func (a *Authenticator) Logout(w http.ResponseWriter, request *http.Request) {
	if cookie, err := request.Cookie(sessionCookie); err == nil && a.logins != nil {
		a.logins.end(cookie.Value)
	}
	http.SetCookie(
		w,
		&http.Cookie{Name: sessionCookie, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1},
	)
	http.Redirect(w, request, "/?signin="+string(SignInSignedOut), http.StatusSeeOther)
}
