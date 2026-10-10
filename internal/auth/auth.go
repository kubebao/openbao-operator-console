// Package auth authenticates viewers and applies administrator-controlled scopes.
package auth

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

const (
	sessionCookie = "__Host-console-session"
	stateCookie   = "__Host-console-state"
)

// Resolver maps an authenticated subject and its groups to current console access.
type Resolver func(context.Context, string, []string) (config.User, bool)

// Authenticator signs viewers in. It runs in one of three modes: a fixed demonstration
// viewer, pending browser setup without a provider, or OIDC with a verified provider.
// Access is resolved on every request, so access changes apply to existing sessions.
type Authenticator struct {
	demo    *config.User
	resolve Resolver
	logins  *store

	mu       sync.Mutex
	provider *Provider // nil while setup is pending
}

// NewDemo signs every request in as a fixed, scoped demonstration viewer.
func NewDemo(viewer config.User) *Authenticator { return &Authenticator{demo: &viewer} }

// NewPending rejects fleet access until browser setup installs a verified provider.
func NewPending(resolve Resolver) *Authenticator {
	return &Authenticator{resolve: resolve, logins: newStore()}
}

// New signs viewers in through provider.
func New(provider *Provider, resolve Resolver) *Authenticator {
	return &Authenticator{resolve: resolve, logins: newStore(), provider: provider}
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
	return a.resolve(request.Context(), active.subject, active.groups)
}

func safeReturn(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || parsed.Host != "" || parsed.IsAbs() ||
		strings.ContainsAny(value, "\\\r\n") ||
		strings.HasPrefix(parsed.Path, "/auth/") {
		return "/"
	}
	return value
}

// begin starts a PKCE login with a nonce against provider and returns the provider's
// authorization URL.
func (a *Authenticator) begin(
	w http.ResponseWriter,
	provider *Provider,
	returnTo string,
	setup bool,
	activate func(context.Context, string, []string) error,
) (string, error) {
	nonce, err := randomToken()
	if err != nil {
		return "", err
	}
	verifier := oauth2.GenerateVerifier()
	state, err := a.logins.begin(transaction{
		verifier: verifier,
		nonce:    nonce,
		provider: provider,
		returnTo: safeReturn(returnTo),
		activate: activate,
		setup:    setup,
	})
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookie,
		Value:    state,
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
	location, err := a.begin(w, provider, request.URL.Query().Get("returnTo"), false, nil)
	if err != nil {
		http.Redirect(w, request, "/?signin="+string(SignInUnavailable), http.StatusSeeOther)
		return
	}
	// The location is the provider's authorization URL. returnTo stays in the server-side
	// transaction after safeReturn limits it to a local path.
	http.Redirect(w, request, location, http.StatusFound) //nolint:gosec // Provider URL, not request input.
}

// BeginTrial binds a candidate identity provider to an administrator-initiated
// browser flow. Configuration changes only after a verified administrator login.
func (a *Authenticator) BeginTrial(
	w http.ResponseWriter,
	candidate *Provider,
	activate func(context.Context, string, []string) error,
) (string, error) {
	return a.begin(w, candidate, "/settings?section=identity", false, activate)
}

// BeginSetup uses the normal signed OIDC flow, with a first-claim callback and
// bounded failure destinations that never include provider errors or secrets.
func (a *Authenticator) BeginSetup(
	w http.ResponseWriter,
	candidate *Provider,
	activate func(context.Context, string, []string) error,
) (string, error) {
	return a.begin(w, candidate, "/settings/clusters/new", true, activate)
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
