package auth

import (
	"errors"
	"net/http"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

var errSessionsUnavailable = errors.New("sessions are unavailable")

func setSessionCookie(w http.ResponseWriter, id string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
	})
}

// OpenSession signs in a principal that the caller verified, such as a local account
// after its password and second factor. It ends the session the request already carries,
// and issues a new random session ID.
func (a *Authenticator) OpenSession(w http.ResponseWriter, request *http.Request, principal config.Principal) error {
	if a.demo != nil || a.logins == nil || principal.Issuer != config.LocalIssuer {
		return errSessionsUnavailable
	}
	if cookie, err := request.Cookie(sessionCookie); err == nil {
		a.logins.end(cookie.Value)
	}
	expires := a.now().Add(maxSessionLifetime)
	id, err := a.logins.open(a.logins.current(), session{principal: principal, expires: expires})
	if err != nil {
		return err
	}
	setSessionCookie(w, id, expires)
	return nil
}

// SingleSignOn reports whether an identity provider is active.
func (a *Authenticator) SingleSignOn() bool { return a.demo == nil && a.activeProvider() != nil }

// EndLocalSessions ends every session of a local account, such as after a reset or
// removal.
func (a *Authenticator) EndLocalSessions(accountID string) {
	if a.logins != nil {
		a.logins.endPrincipal(config.LocalIssuer, accountID, "")
	}
}

// EndOtherLocalSessions ends the sessions of a local account except the one request
// carries, such as after a password change.
func (a *Authenticator) EndOtherLocalSessions(request *http.Request, accountID string) {
	if a.logins == nil {
		return
	}
	keep := ""
	if cookie, err := request.Cookie(sessionCookie); err == nil {
		keep = cookie.Value
	}
	a.logins.endPrincipal(config.LocalIssuer, accountID, keep)
}
