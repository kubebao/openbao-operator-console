package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/kubebao/openbao-operator-console/internal/clientip"
	"github.com/kubebao/openbao-operator-console/internal/config"
)

const (
	maxSessionLifetime = 8 * time.Hour
	maxGroups          = 1000
)

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

// identity is the verified result of a provider login.
type identity struct {
	principal config.Principal
	expiry    time.Time
}

// transaction is one login being completed. It keeps the provider it started with, so a
// provider trial completes against the candidate rather than the active provider.
type transaction struct {
	verifier, nonce string
	provider        *Provider
	generation      int64
	returnTo        string
	activate        Activation
	setup           bool
}

// failure is a bounded callback error. Its message reaches only an administrator during
// an identity trial; other logins see the sign-in result. The reason names the failure
// class in the security log.
type failure struct {
	status  int
	message string
	result  SignInResult
	reason  string
}

func (a *Authenticator) Callback(w http.ResponseWriter, request *http.Request) {
	if a.demo != nil {
		http.NotFound(w, request)
		return
	}
	tx, ok := a.takeTransaction(w, request)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 15*time.Second)
	defer cancel()
	verified, problem := tx.provider.identify(ctx, request.URL.Query().Get("code"), tx)
	if problem != nil {
		a.fail(w, request, tx, *problem, "")
		return
	}
	generation := tx.generation
	if tx.activate != nil {
		if err := tx.activate(ctx, verified.principal); err != nil {
			a.fail(w, request, tx, failure{
				http.StatusForbidden,
				"Identity provider was not activated. Return to Settings, reload, and retry as a console administrator.",
				SignInDenied,
				"activation_refused",
			}, verified.principal.Subject)
			return
		}
		// Activation may replace the provider, which starts a new generation.
		generation = a.logins.current()
	}
	if _, allowed := a.resolve(ctx, verified.principal); !allowed {
		a.fail(
			w,
			request,
			tx,
			failure{http.StatusForbidden, "Console access is not enrolled", SignInDenied, "not_enrolled"},
			verified.principal.Subject,
		)
		return
	}
	expires := time.Now().Add(maxSessionLifetime)
	if verified.expiry.Before(expires) {
		expires = verified.expiry
	}
	id, err := a.logins.open(generation, session{principal: verified.principal, expires: expires})
	if errors.Is(err, errCapacity) {
		a.fail(w, request, tx, failure{http.StatusBadRequest, "Login expired", SignInExpired, "session_unavailable"}, "")
		return
	}
	if err != nil {
		a.fail(w, request, tx, failure{http.StatusInternalServerError, "Session creation failed", SignInFailed, "session_unavailable"}, "")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
	})
	http.Redirect(w, request, tx.returnTo, http.StatusSeeOther)
}

// takeTransaction opens the sealed login named by the state cookie and query parameter
// and consumes its state, so a callback cannot be replayed. On failure it redirects to
// the sign-in or setup page and reports false.
func (a *Authenticator) takeTransaction(w http.ResponseWriter, request *http.Request) (transaction, bool) {
	active := a.activeProvider()
	pending := active == nil
	state := request.URL.Query().Get("state")
	cookie, err := request.Cookie(stateCookie)
	if err != nil || state == "" {
		a.reject(w, request, pending, SignInFailed, "state_missing")
		return transaction{}, false
	}
	login, sealed := a.sealer.open(cookie.Value)
	if !sealed || subtle.ConstantTimeCompare([]byte(login.State), []byte(state)) != 1 {
		a.reject(w, request, pending, SignInFailed, "state_invalid")
		return transaction{}, false
	}
	http.SetCookie(
		w,
		&http.Cookie{Name: stateCookie, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1},
	)
	now := a.now()
	if now.Unix() >= login.Expires {
		a.reject(w, request, pending, SignInExpired, "expired")
		return transaction{}, false
	}
	candidate, valid := a.logins.consume(login, now)
	if !valid {
		a.reject(w, request, pending, SignInExpired, "replayed_or_superseded")
		return transaction{}, false
	}
	tx := transaction{
		verifier:   login.Verifier,
		nonce:      login.Nonce,
		provider:   active,
		generation: login.Generation,
		returnTo:   safeReturn(login.ReturnTo),
	}
	if login.Trial != "" {
		tx.provider, tx.activate, tx.setup = candidate.provider, candidate.activate, candidate.setup
	}
	if tx.provider == nil {
		a.reject(w, request, pending, SignInExpired, "provider_unavailable")
		return transaction{}, false
	}
	return tx, true
}

// identify exchanges the authorization code with the PKCE verifier and verifies the ID
// token, its nonce, and the optional groups claim.
func (p *Provider) identify(ctx context.Context, code string, tx transaction) (identity, *failure) {
	ctx = oidc.ClientContext(ctx, p.client)
	token, err := p.oauth.Exchange(ctx, code, oauth2.VerifierOption(tx.verifier))
	if err != nil {
		return identity{}, &failure{http.StatusUnauthorized, "Login exchange failed", SignInFailed, "exchange_failed"}
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return identity{}, &failure{http.StatusUnauthorized, "Identity token missing", SignInFailed, "id_token_missing"}
	}
	verified, err := p.verifier.Verify(ctx, raw)
	if err != nil || subtle.ConstantTimeCompare([]byte(verifiedNonce(verified)), []byte(tx.nonce)) != 1 {
		return identity{}, &failure{http.StatusUnauthorized, "Identity verification failed", SignInFailed, "id_token_invalid"}
	}
	groups, problem := p.groups(verified)
	if problem != nil {
		return identity{}, problem
	}
	// The verifier checked that the token issuer equals the provider's configured issuer.
	return identity{
		principal: config.Principal{Issuer: verified.Issuer, Subject: verified.Subject, Groups: groups},
		expiry:    verified.Expiry,
	}, nil
}

func (p *Provider) groups(token *oidc.IDToken) ([]string, *failure) {
	groups := []string{}
	if p.groupsClaim == "" {
		return groups, nil
	}
	var claims map[string]json.RawMessage
	if err := token.Claims(&claims); err != nil {
		return nil, &failure{http.StatusUnauthorized, "Identity claims invalid", SignInFailed, "claims_invalid"}
	}
	if value, found := claims[p.groupsClaim]; found {
		if json.Unmarshal(value, &groups) != nil || len(groups) > maxGroups {
			return nil, &failure{http.StatusUnauthorized, "Identity groups invalid", SignInFailed, "groups_invalid"}
		}
	}
	return groups, nil
}

func verifiedNonce(token *oidc.IDToken) string {
	if token == nil {
		return ""
	}
	return token.Nonce
}

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

// logFailure records a failed sign-in without tokens, codes, or provider messages. A
// subject appears only after the provider verified it.
func (a *Authenticator) logFailure(request *http.Request, reason string, trial, setup bool, subject string) {
	if a.Logger == nil {
		return
	}
	attributes := []any{"reason", reason, "remote", clientip.From(request), "trial", trial, "setup", setup}
	if subject != "" {
		attributes = append(attributes, "subject", subject)
	}
	a.Logger.Warn("Sign-in failed", attributes...)
}

// reject ends a callback whose login cannot be used.
func (a *Authenticator) reject(w http.ResponseWriter, request *http.Request, setup bool, result SignInResult, reason string) {
	a.logFailure(request, reason, false, setup, "")
	callbackFailure(w, request, setup, result)
}

// fail ends a callback whose login was valid. Identity trials keep an error response:
// the administrator still has a session, so the sign-in page would not render.
func (a *Authenticator) fail(w http.ResponseWriter, request *http.Request, tx transaction, problem failure, subject string) {
	a.logFailure(request, problem.reason, tx.activate != nil, tx.setup, subject)
	if tx.activate != nil && !tx.setup {
		http.Error(w, problem.message, problem.status)
		return
	}
	callbackFailure(w, request, tx.setup, problem.result)
}
