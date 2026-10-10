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
	subject string
	groups  []string
	expiry  time.Time
}

// failure is a bounded callback error. Its message reaches only an administrator during
// an identity trial; other logins see the sign-in result.
type failure struct {
	status  int
	message string
	result  SignInResult
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
		tx.fail(w, request, *problem)
		return
	}
	generation := tx.generation
	if tx.activate != nil {
		if err := tx.activate(ctx, verified.subject, verified.groups); err != nil {
			tx.fail(w, request, failure{
				http.StatusForbidden,
				"Identity provider was not activated. Return to Settings and retry with an enrolled administrator.",
				SignInDenied,
			})
			return
		}
		// Activation may replace the provider, which starts a new generation.
		generation = a.logins.current()
	}
	if _, allowed := a.resolve(ctx, verified.subject, verified.groups); !allowed {
		tx.fail(w, request, failure{http.StatusForbidden, "Console access is not enrolled", SignInDenied})
		return
	}
	expires := time.Now().Add(maxSessionLifetime)
	if verified.expiry.Before(expires) {
		expires = verified.expiry
	}
	id, err := a.logins.open(generation, session{subject: verified.subject, groups: verified.groups, expires: expires})
	if errors.Is(err, errCapacity) {
		tx.fail(w, request, failure{http.StatusBadRequest, "Login expired", SignInExpired})
		return
	}
	if err != nil {
		tx.fail(w, request, failure{http.StatusInternalServerError, "Session creation failed", SignInFailed})
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

// takeTransaction consumes the login named by the state cookie and query parameter. On
// failure it redirects to the sign-in or setup page and reports false.
func (a *Authenticator) takeTransaction(w http.ResponseWriter, request *http.Request) (transaction, bool) {
	pending := a.activeProvider() == nil
	state := request.URL.Query().Get("state")
	cookie, err := request.Cookie(stateCookie)
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 {
		callbackFailure(w, request, pending, SignInFailed)
		return transaction{}, false
	}
	tx, valid := a.logins.take(state)
	http.SetCookie(
		w,
		&http.Cookie{Name: stateCookie, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1},
	)
	if !valid {
		callbackFailure(w, request, tx.setup || pending, SignInExpired)
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
		return identity{}, &failure{http.StatusUnauthorized, "Login exchange failed", SignInFailed}
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return identity{}, &failure{http.StatusUnauthorized, "Identity token missing", SignInFailed}
	}
	verified, err := p.verifier.Verify(ctx, raw)
	if err != nil || subtle.ConstantTimeCompare([]byte(verifiedNonce(verified)), []byte(tx.nonce)) != 1 {
		return identity{}, &failure{http.StatusUnauthorized, "Identity verification failed", SignInFailed}
	}
	groups, problem := p.groups(verified)
	if problem != nil {
		return identity{}, problem
	}
	return identity{subject: verified.Subject, groups: groups, expiry: verified.Expiry}, nil
}

func (p *Provider) groups(token *oidc.IDToken) ([]string, *failure) {
	groups := []string{}
	if p.groupsClaim == "" {
		return groups, nil
	}
	var claims map[string]json.RawMessage
	if err := token.Claims(&claims); err != nil {
		return nil, &failure{http.StatusUnauthorized, "Identity claims invalid", SignInFailed}
	}
	if value, found := claims[p.groupsClaim]; found {
		if json.Unmarshal(value, &groups) != nil || len(groups) > maxGroups {
			return nil, &failure{http.StatusUnauthorized, "Identity groups invalid", SignInFailed}
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

// Identity trials keep an error response: the administrator still has a session, so the
// sign-in page would not render.
func (tx transaction) fail(w http.ResponseWriter, request *http.Request, problem failure) {
	if tx.activate != nil && !tx.setup {
		http.Error(w, problem.message, problem.status)
		return
	}
	callbackFailure(w, request, tx.setup, problem.result)
}
