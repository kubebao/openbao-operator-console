// Package auth authenticates viewers and applies administrator-controlled scopes.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
)

type session struct {
	subject string
	groups  []string
	expires time.Time
}
type providerState struct {
	oauth       oauth2.Config
	verifier    *oidc.IDTokenVerifier
	client      *http.Client
	groupsClaim string
}
type transaction struct {
	verifier, nonce string
	expires         time.Time
	provider        providerState
	generation      int64
	returnTo        string
	activate        func(context.Context, string, []string) error
	setup           bool
}
type (
	Resolver      func(context.Context, string, []string) (config.User, bool)
	Authenticator struct {
		mu           sync.Mutex
		demo         bool
		users        map[string]config.User
		sessions     map[string]session
		transactions map[string]transaction
		provider     providerState
		generation   int64
		resolver     Resolver
		pending      bool
	}
)

func NewDemo() *Authenticator { return &Authenticator{demo: true} }

// NewPending rejects fleet access until browser setup installs a verified provider.
func NewPending() *Authenticator {
	return &Authenticator{
		pending:      true,
		users:        map[string]config.User{},
		sessions:     map[string]session{},
		transactions: map[string]transaction{},
	}
}

func NewOIDC(ctx context.Context, issuer, clientID, clientSecret, redirect string, users []config.User) (*Authenticator, error) {
	callback, err := url.Parse(redirect)
	if err != nil || callback.Scheme != "https" || callback.Host == "" || callback.Path != "/auth/callback" || callback.RawQuery != "" ||
		callback.Fragment != "" ||
		callback.User != nil {
		return nil, errors.New("OIDC callback must be an HTTPS URL ending in /auth/callback")
	}
	identity, err := url.Parse(issuer)
	if err != nil || !httpsEndpoint(identity) || identity.RawQuery != "" || identity.Fragment != "" || clientID == "" {
		return nil, errors.New("OIDC requires an HTTPS issuer and client ID")
	}
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, errors.New("discover OIDC provider")
	}
	var discovery struct {
		JWKS string `json:"jwks_uri"`
	}
	if err := provider.Claims(&discovery); err != nil {
		return nil, errors.New("decode OIDC discovery")
	}
	for _, endpoint := range []string{provider.Endpoint().AuthURL, provider.Endpoint().TokenURL, discovery.JWKS} {
		parsed, err := url.Parse(endpoint)
		if err != nil || !httpsEndpoint(parsed) {
			return nil, errors.New("OIDC endpoints must use HTTPS")
		}
	}
	client, ok := ctx.Value(oauth2.HTTPClient).(*http.Client)
	if !ok || client == nil {
		client = http.DefaultClient
	}
	result := &Authenticator{
		users:        map[string]config.User{},
		sessions:     map[string]session{},
		transactions: map[string]transaction{},
		provider: providerState{
			oauth: oauth2.Config{
				ClientID:     clientID,
				ClientSecret: clientSecret,
				Endpoint:     provider.Endpoint(),
				RedirectURL:  redirect,
				Scopes:       []string{oidc.ScopeOpenID, "profile"},
			},
			verifier:    provider.Verifier(&oidc.Config{ClientID: clientID}),
			client:      client,
			groupsClaim: "groups",
		},
	}
	for _, user := range users {
		result.users[user.Subject] = user
	}
	return result, nil
}

func (a *Authenticator) SetResolver(resolver Resolver) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.resolver = resolver
}

func (a *Authenticator) SetGroupsClaim(claim string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.provider.groupsClaim = claim
}

func (a *Authenticator) ReplaceProvider(candidate *Authenticator) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.provider = candidate.provider
	a.pending = false
	a.generation++
	a.sessions = map[string]session{}
	a.transactions = map[string]transaction{}
}

func (a *Authenticator) resolve(ctx context.Context, subject string, groups []string) (config.User, bool) {
	a.mu.Lock()
	resolver := a.resolver
	user, allowed := a.users[subject]
	a.mu.Unlock()
	if resolver != nil {
		return resolver(ctx, subject, groups)
	}
	return user, allowed
}

func (a *Authenticator) Viewer(request *http.Request) (config.User, bool) {
	if a.demo {
		return config.User{Subject: "local-demo", Name: "Platform team"}, true
	}
	cookie, err := request.Cookie("__Host-console-session")
	if err != nil {
		return config.User{}, false
	}
	a.mu.Lock()
	s, exists := a.sessions[cookie.Value]
	if !exists || !time.Now().Before(s.expires) {
		delete(a.sessions, cookie.Value)
		a.mu.Unlock()
		return config.User{}, false
	}
	a.mu.Unlock()
	return a.resolve(request.Context(), s.subject, s.groups)
}

func randomToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
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

func (a *Authenticator) begin(
	w http.ResponseWriter,
	provider providerState,
	returnTo string,
	setup bool,
	activate func(context.Context, string, []string) error,
) (string, error) {
	state, err := randomToken()
	if err != nil {
		return "", err
	}
	nonce, err := randomToken()
	if err != nil {
		return "", err
	}
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	for id, tx := range a.transactions {
		if !now.Before(tx.expires) {
			delete(a.transactions, id)
		}
	}
	for id, s := range a.sessions {
		if !now.Before(s.expires) {
			delete(a.sessions, id)
		}
	}
	if len(a.transactions) >= 1000 || len(a.sessions) >= 10000 {
		return "", errors.New("login capacity reached")
	}
	verifier := oauth2.GenerateVerifier()
	a.transactions[state] = transaction{
		verifier:   verifier,
		nonce:      nonce,
		expires:    now.Add(5 * time.Minute),
		provider:   provider,
		generation: a.generation,
		returnTo:   safeReturn(returnTo),
		activate:   activate,
		setup:      setup,
	}
	http.SetCookie(
		w,
		&http.Cookie{
			Name:     "__Host-console-state",
			Value:    state,
			Path:     "/",
			Secure:   true,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   300,
		},
	)
	return provider.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce)), nil
}

func (a *Authenticator) Login(w http.ResponseWriter, request *http.Request) {
	if a.demo {
		http.Redirect(w, request, "/", http.StatusSeeOther)
		return
	}
	a.mu.Lock()
	provider := a.provider
	pending := a.pending
	a.mu.Unlock()
	if pending {
		http.Redirect(w, request, "/setup", http.StatusSeeOther)
		return
	}
	location, err := a.begin(w, provider, request.URL.Query().Get("returnTo"), false, nil)
	if err != nil {
		http.Redirect(w, request, "/?signin="+string(SignInUnavailable), http.StatusSeeOther)
		return
	}
	http.Redirect(w, request, location, http.StatusFound)
}

// BeginTrial binds a candidate identity provider to an administrator-initiated
// browser flow. Configuration changes only after a verified administrator login.
func (a *Authenticator) BeginTrial(
	w http.ResponseWriter,
	candidate *Authenticator,
	activate func(context.Context, string, []string) error,
) (string, error) {
	return a.begin(w, candidate.provider, "/settings?section=identity", false, activate)
}

// BeginSetup uses the normal signed OIDC flow, with a first-claim callback and
// bounded failure destinations that never include provider errors or secrets.
func (a *Authenticator) BeginSetup(
	w http.ResponseWriter,
	candidate *Authenticator,
	activate func(context.Context, string, []string) error,
) (string, error) {
	return a.begin(w, candidate.provider, "/settings/clusters/new", true, activate)
}

// SignInResult is the bounded reason the sign-in page shows after a failed or ended login.
// Provider errors never reach the browser.
type SignInResult string

const (
	SignInExpired     SignInResult = "expired"
	SignInDenied      SignInResult = "denied"
	SignInFailed      SignInResult = "failed"
	SignInUnavailable SignInResult = "unavailable"
	SignInSignedOut   SignInResult = "signed-out"
)

func callbackFailure(w http.ResponseWriter, request *http.Request, setup bool, result SignInResult) {
	if setup {
		if result != SignInExpired {
			result = SignInFailed
		}
		http.Redirect(w, request, "/setup?result="+string(result), http.StatusSeeOther)
		return
	}
	http.Redirect(w, request, "/?signin="+string(result), http.StatusSeeOther)
}

func (a *Authenticator) Callback(w http.ResponseWriter, request *http.Request) {
	if a.demo {
		http.NotFound(w, request)
		return
	}
	state := request.URL.Query().Get("state")
	cookie, err := request.Cookie("__Host-console-state")
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 {
		a.mu.Lock()
		pending := a.pending
		a.mu.Unlock()
		callbackFailure(w, request, pending, SignInFailed)
		return
	}
	a.mu.Lock()
	tx, exists := a.transactions[state]
	delete(a.transactions, state)
	generation := a.generation
	pending := a.pending
	a.mu.Unlock()
	http.SetCookie(
		w,
		&http.Cookie{Name: "__Host-console-state", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1},
	)
	if !exists || !time.Now().Before(tx.expires) || tx.generation != generation {
		callbackFailure(w, request, tx.setup || pending, SignInExpired)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 15*time.Second)
	defer cancel()
	ctx = oidc.ClientContext(ctx, tx.provider.client)
	token, err := tx.provider.oauth.Exchange(ctx, request.URL.Query().Get("code"), oauth2.VerifierOption(tx.verifier))
	if err != nil {
		tx.fail(w, request, http.StatusUnauthorized, "Login exchange failed", SignInFailed)
		return
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		tx.fail(w, request, http.StatusUnauthorized, "Identity token missing", SignInFailed)
		return
	}
	verified, err := tx.provider.verifier.Verify(ctx, raw)
	if err != nil || subtle.ConstantTimeCompare([]byte(verifiedNonce(verified)), []byte(tx.nonce)) != 1 {
		tx.fail(w, request, http.StatusUnauthorized, "Identity verification failed", SignInFailed)
		return
	}
	groups := []string{}
	if tx.provider.groupsClaim != "" {
		var claims map[string]json.RawMessage
		if err := verified.Claims(&claims); err != nil {
			tx.fail(w, request, http.StatusUnauthorized, "Identity claims invalid", SignInFailed)
			return
		}
		if value, found := claims[tx.provider.groupsClaim]; found {
			if json.Unmarshal(value, &groups) != nil || len(groups) > 1000 {
				tx.fail(w, request, http.StatusUnauthorized, "Identity groups invalid", SignInFailed)
				return
			}
		}
	}
	if tx.activate != nil {
		if err := tx.activate(ctx, verified.Subject, groups); err != nil {
			tx.fail(
				w,
				request,
				http.StatusForbidden,
				"Identity provider was not activated. Return to Settings and retry with an enrolled administrator.",
				SignInDenied,
			)
			return
		}
		a.mu.Lock()
		generation = a.generation
		a.mu.Unlock()
	}
	if _, allowed := a.resolve(ctx, verified.Subject, groups); !allowed {
		tx.fail(w, request, http.StatusForbidden, "Console access is not enrolled", SignInDenied)
		return
	}
	id, err := randomToken()
	if err != nil {
		tx.fail(w, request, http.StatusInternalServerError, "Session creation failed", SignInFailed)
		return
	}
	expires := time.Now().Add(8 * time.Hour)
	if verified.Expiry.Before(expires) {
		expires = verified.Expiry
	}
	a.mu.Lock()
	if generation != a.generation || len(a.sessions) >= 10000 {
		a.mu.Unlock()
		tx.fail(w, request, http.StatusBadRequest, "Login expired", SignInExpired)
		return
	}
	a.sessions[id] = session{subject: verified.Subject, groups: groups, expires: expires}
	a.mu.Unlock()
	http.SetCookie(
		w,
		&http.Cookie{
			Name:     "__Host-console-session",
			Value:    id,
			Path:     "/",
			Secure:   true,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Expires:  expires,
		},
	)
	http.Redirect(w, request, tx.returnTo, http.StatusSeeOther)
}

// Identity trials keep an error response: the administrator still has a session, so the
// sign-in page would not render.
func (tx transaction) fail(w http.ResponseWriter, request *http.Request, status int, message string, result SignInResult) {
	if tx.activate != nil && !tx.setup {
		http.Error(w, message, status)
		return
	}
	callbackFailure(w, request, tx.setup, result)
}

func verifiedNonce(token *oidc.IDToken) string {
	if token == nil {
		return ""
	}
	return token.Nonce
}

func (a *Authenticator) Logout(w http.ResponseWriter, request *http.Request) {
	if cookie, err := request.Cookie("__Host-console-session"); err == nil {
		a.mu.Lock()
		delete(a.sessions, cookie.Value)
		a.mu.Unlock()
	}
	http.SetCookie(
		w,
		&http.Cookie{Name: "__Host-console-session", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1},
	)
	http.Redirect(w, request, "/?signin="+string(SignInSignedOut), http.StatusSeeOther)
}

func Allowed(user config.User, deployment contract.Deployment) bool {
	for _, scope := range user.Scopes {
		if scope.ConnectionID != deployment.ConnectionId {
			continue
		}
		for _, namespace := range scope.Namespaces {
			if namespace == deployment.Namespace {
				return true
			}
		}
	}
	return false
}

func Scope(snapshot contract.Snapshot, user config.User, demo bool) contract.Snapshot {
	result := contract.Snapshot{
		Mode:          snapshot.Mode,
		CollectedAt:   snapshot.CollectedAt,
		Policy:        snapshot.Policy,
		Deployments:   []contract.Deployment{},
		Connections:   []contract.Connection{},
		Relationships: []contract.Relationship{},
		Activity:      []contract.Activity{},
		Viewer:        contract.Viewer{Name: user.Name, Subject: user.Subject, Admin: user.Admin, ConnectionIds: []string{}},
	}
	visible := make(map[string]bool)
	for _, deployment := range snapshot.Deployments {
		if demo || Allowed(user, deployment) {
			result.Deployments = append(result.Deployments, deployment)
			visible[deployment.Id] = true
		}
	}
	for _, connection := range snapshot.Connections {
		namespaces := []string{}
		for _, ns := range connection.Namespaces {
			if demo || Allowed(user, contract.Deployment{ConnectionId: connection.Id, Namespace: ns}) {
				namespaces = append(namespaces, ns)
			}
		}
		if len(namespaces) > 0 {
			connection.Namespaces = namespaces
			operators := []contract.Operator{}
			for _, operator := range connection.Operators {
				managed := []string{}
				for _, namespace := range operator.Namespaces {
					if demo || Allowed(user, contract.Deployment{ConnectionId: connection.Id, Namespace: namespace}) {
						managed = append(managed, namespace)
					}
				}
				if len(managed) > 0 {
					operator.Namespaces = managed
					operators = append(operators, operator)
				}
			}
			connection.Operators = operators
			capabilities := []contract.ApiCapability{}
			for _, capability := range connection.Capabilities {
				if demo || Allowed(user, contract.Deployment{ConnectionId: connection.Id, Namespace: capability.Namespace}) {
					capabilities = append(capabilities, capability)
				}
			}
			connection.Capabilities = capabilities
			result.Connections = append(result.Connections, connection)
			result.Viewer.ConnectionIds = append(result.Viewer.ConnectionIds, connection.Id)
		}
	}
	for _, relationship := range snapshot.Relationships {
		if visible[relationship.SourceId] && visible[relationship.TargetId] {
			result.Relationships = append(result.Relationships, relationship)
		}
	}
	for _, activity := range snapshot.Activity {
		if visible[activity.DeploymentId] {
			result.Activity = append(result.Activity, activity)
		}
	}
	return result
}
