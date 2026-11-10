package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/clientip"
	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/localauth"
)

// localSessionControl ends the sessions of local accounts.
type localSessionControl interface {
	EndLocalSessions(accountID string)
	EndOtherLocalSessions(request *http.Request, accountID string)
}

func (s *Server) localAccountRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/settings/local-accounts", s.admin(s.localAccounts(s.createLocalAccount)))
	mux.HandleFunc("PUT /api/v1/settings/local-accounts/{id}", s.admin(s.localAccounts(s.updateLocalAccount)))
	mux.HandleFunc("DELETE /api/v1/settings/local-accounts/{id}", s.admin(s.localAccounts(s.deleteLocalAccount)))
	mux.HandleFunc("POST /api/v1/settings/local-accounts/{id}/enrollment", s.admin(s.localAccounts(s.resetLocalAccount)))
	mux.HandleFunc("PUT /api/v1/settings/local-sign-in", s.admin(s.setLocalSignIn))
	mux.HandleFunc("POST /api/v1/enrollment", s.openEnrollment)
	mux.HandleFunc("POST /api/v1/enrollment/complete", s.completeEnrollment)
	mux.HandleFunc("POST /api/v1/account/password", s.localViewer(s.changePassword))
	mux.HandleFunc("POST /api/v1/account/recovery-codes", s.localViewer(s.replaceRecoveryCodes))
}

// localAccounts requires local account support before an administrator handler runs.
func (s *Server) localAccounts(handler adminHandler) adminHandler {
	return func(w http.ResponseWriter, r *http.Request, actor config.User) error {
		if s.Local == nil {
			return &problemError{status: http.StatusNotFound, code: "not_found", message: "Local accounts are not available."}
		}
		return handler(w, r, actor)
	}
}

func (s *Server) endLocalSessions(accountID string) {
	if control, ok := s.Auth.(localSessionControl); ok {
		control.EndLocalSessions(accountID)
	}
}

// localAccountViews adds each account's enrollment state from its credential.
func (s *Server) localAccountViews(ctx context.Context, accounts []config.LocalAccount) ([]contract.LocalAccount, error) {
	result := make([]contract.LocalAccount, 0, len(accounts))
	for _, account := range accounts {
		view := contract.LocalAccount{
			Id:       account.ID,
			Username: account.Username,
			Name:     account.Name,
			Admin:    account.Admin,
			Scopes:   scopesView(account.Scopes),
			State:    contract.Active,
		}
		if s.Local != nil {
			stored, err := s.Local.Store.Load(ctx, account.ID)
			switch {
			case errors.Is(err, localauth.ErrNotFound):
				view.State = contract.EnrollmentExpired
			case err != nil:
				return nil, err
			case stored.Credential.Pending():
				expires := stored.Credential.Enrollment.ExpiresAt
				view.EnrollmentExpiresAt = &expires
				view.State = contract.EnrollmentPending
				if !time.Now().Before(expires) {
					view.State = contract.EnrollmentExpired
				}
			}
		}
		result = append(result, view)
	}
	return result, nil
}

func (s *Server) enrollmentResponse(w http.ResponseWriter, accountID string, link localauth.Link) {
	jsonResponse(w, http.StatusOK, contract.LocalAccountEnrollment{
		AccountId:     accountID,
		EnrollmentUrl: s.Origin + "/enroll#token=" + link.Token,
		ExpiresAt:     link.ExpiresAt,
	})
}

// createLocalAccount stores a pending credential first and then enrolls the account. If
// the configuration changed meanwhile, the credential belongs to no account and is
// removed at the next start.
func (s *Server) createLocalAccount(w http.ResponseWriter, r *http.Request, actor config.User) error {
	input, err := decodeBody[contract.LocalAccountInput](w, r)
	if err != nil {
		return err
	}
	if !localUsername.MatchString(input.Username) || len(input.Name) > 256 {
		return badRequest("Use a lowercase username of letters, digits, dots, hyphens, or underscores.")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	id, err := config.NewLocalAccountID()
	if err != nil {
		return retryLater("The account could not be created.")
	}
	link, err := s.Local.IssueLink(ctx, id)
	if err != nil {
		return unavailable(err, "The account could not be created.")
	}
	account := config.LocalAccount{
		ID:       id,
		Username: input.Username,
		Name:     input.Name,
		Admin:    input.Admin,
		Scopes:   scopesInput(input.Scopes),
	}
	if _, err := s.Settings.AddLocalAccount(ctx, input.Revision, account, actor.Subject); err != nil {
		return unavailable(err, "The account could not be created.")
	}
	s.enrollmentResponse(w, id, link)
	return nil
}

func (s *Server) updateLocalAccount(w http.ResponseWriter, r *http.Request, actor config.User) error {
	input, err := decodeBody[contract.LocalAccountUpdate](w, r)
	if err != nil {
		return err
	}
	if len(input.Name) > 256 {
		return badRequest("Use a name of at most 256 characters.")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	update := config.LocalAccount{ID: r.PathValue("id"), Name: input.Name, Admin: input.Admin, Scopes: scopesInput(input.Scopes)}
	if _, err := s.Settings.UpdateLocalAccount(ctx, input.Revision, update, actor.Subject); err != nil {
		return unavailable(err, "The account could not be saved.")
	}
	return s.writeSettings(ctx, w)
}

func (s *Server) deleteLocalAccount(w http.ResponseWriter, r *http.Request, actor config.User) error {
	revision, err := strconv.ParseInt(r.URL.Query().Get("revision"), 10, 64)
	if err != nil {
		return badRequest("Supply the settings revision.")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	id := r.PathValue("id")
	if _, err := s.Settings.RemoveLocalAccount(ctx, revision, id, actor.Subject); err != nil {
		return unavailable(err, "The account could not be removed.")
	}
	s.endLocalSessions(id)
	current, _ := s.Settings.Current()
	if err := s.Local.Store.Prune(ctx, current.LocalAccountIDs()); err != nil && s.Logger != nil {
		// The account is gone and cannot sign in; startup removes the credential.
		s.Logger.Warn("Credential of a removed local account was not deleted", "error", err)
	}
	return s.writeSettings(ctx, w)
}

// resetLocalAccount revokes the account's credential and sessions and issues a new link.
func (s *Server) resetLocalAccount(w http.ResponseWriter, r *http.Request, actor config.User) error {
	id := r.PathValue("id")
	current, _ := s.Settings.Current()
	found := false
	for _, account := range current.LocalAccounts {
		found = found || account.ID == id
	}
	if !found {
		return badRequest("The local account is not enrolled.")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	link, err := s.Local.IssueLink(ctx, id)
	if err != nil {
		return unavailable(err, "The account could not be reset.")
	}
	s.endLocalSessions(id)
	if s.Logger != nil {
		s.Logger.Info("Local account reset", "account", id, "actor", actor.Subject)
	}
	s.enrollmentResponse(w, id, link)
	return nil
}

func (s *Server) setLocalSignIn(w http.ResponseWriter, r *http.Request, actor config.User) error {
	input, err := decodeBody[contract.LocalSignInUpdate](w, r)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if _, err := s.Settings.SetLocalSignIn(ctx, input.Revision, input.Enabled, actor.Subject); err != nil {
		return unavailable(err, "Local sign-in could not be changed.")
	}
	return s.writeSettings(ctx, w)
}

func (s *Server) writeSettings(ctx context.Context, w http.ResponseWriter) error {
	result, err := s.settingsView(ctx)
	if err != nil {
		return unavailable(err, "Saved; reload to read the result.")
	}
	jsonResponse(w, http.StatusOK, result)
	return nil
}

// publicLocal checks a public local-account request: local accounts are available,
// the origin matches, and the client is within its failure limit.
func (s *Server) publicLocal(w http.ResponseWriter, r *http.Request) bool {
	if s.Local == nil || s.Settings == nil {
		problem(w, http.StatusNotFound, "not_found", "Local accounts are not available.")
		return false
	}
	if !s.sameOrigin(r) {
		problem(w, http.StatusForbidden, "forbidden", "Invalid request origin.")
		return false
	}
	if s.signInLimits.blocked(clientip.From(r), time.Now()) {
		w.Header().Set("Retry-After", "60")
		problem(w, http.StatusTooManyRequests, "rate_limited", "Too many failed attempts. Retry in one minute.")
		return false
	}
	configuration, _ := s.Settings.Current()
	if configuration.LocalSignInDisabled {
		problem(w, http.StatusForbidden, "forbidden", "Local sign-in is disabled.")
		return false
	}
	return true
}

// linkFailure counts a rejected link against the client and writes the response.
func (s *Server) linkFailure(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, localauth.ErrInvalidLink) {
		s.signInLimits.failure(clientip.From(r), time.Now())
		s.logSignIn(r, "Enrollment link rejected", "", "invalid_link")
		problem(
			w,
			http.StatusUnauthorized,
			"invalid_token",
			"This enrollment link is invalid, used, or expired. Ask an administrator for a new link.",
		)
		return
	}
	problem(w, http.StatusServiceUnavailable, "unavailable", "Enrollment is temporarily unavailable. Retry.")
}

// enrolledAccount returns the configured account that a link names. A link for a
// removed account is invalid.
func (s *Server) enrolledAccount(token string) (config.LocalAccount, bool) {
	accountID, ok := localauth.LinkAccount(token)
	if !ok {
		return config.LocalAccount{}, false
	}
	configuration, _ := s.Settings.Current()
	for _, account := range configuration.LocalAccounts {
		if account.ID == accountID {
			return account, true
		}
	}
	return config.LocalAccount{}, false
}

func (s *Server) openEnrollment(w http.ResponseWriter, r *http.Request) {
	if !s.publicLocal(w, r) {
		return
	}
	input, err := decodeBody[contract.EnrollmentToken](w, r)
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid", "Open the complete enrollment link.")
		return
	}
	account, found := s.enrolledAccount(input.Token)
	if !found {
		s.linkFailure(w, r, localauth.ErrInvalidLink)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	opened, err := s.Local.OpenLink(ctx, input.Token)
	if err != nil {
		s.linkFailure(w, r, err)
		return
	}
	// Authenticator apps split the label at a colon, so the host is shown without a port.
	label := account.Username
	if origin, err := url.Parse(s.Origin); err == nil && origin.Hostname() != "" {
		label += "@" + origin.Hostname()
	}
	uri := localauth.TOTPURI(opened.TOTPSecret, label)
	code, err := localauth.NewQRCode(uri)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "unavailable", "Enrollment is temporarily unavailable. Retry.")
		return
	}
	jsonResponse(w, http.StatusOK, contract.EnrollmentStart{
		Username:  account.Username,
		Name:      account.Name,
		ExpiresAt: opened.ExpiresAt,
		Authenticator: contract.AuthenticatorEnrollment{
			Key:    localauth.TOTPKey(opened.TOTPSecret),
			Uri:    uri,
			QrSize: code.Size,
			QrPath: code.Path,
		},
	})
}

func (s *Server) completeEnrollment(w http.ResponseWriter, r *http.Request) {
	if !s.publicLocal(w, r) {
		return
	}
	input, err := decodeBody[contract.EnrollmentInput](w, r)
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid", "Enter a password and an authenticator code.")
		return
	}
	account, found := s.enrolledAccount(input.Token)
	if !found {
		s.linkFailure(w, r, localauth.ErrInvalidLink)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	accountID, codes, err := s.Local.CompleteLink(ctx, input.Token, account.Username, input.Password, input.Code)
	var weak localauth.PasswordError
	switch {
	case errors.As(err, &weak):
		problem(w, http.StatusBadRequest, "invalid", "Choose a different password: "+weak.Error()+".")
		return
	case errors.Is(err, localauth.ErrRejected):
		problem(w, http.StatusBadRequest, "invalid", "The authenticator code is not valid. Check the device time and enter a new code.")
		return
	case err != nil:
		s.linkFailure(w, r, err)
		return
	}
	principal := config.Principal{Issuer: config.LocalIssuer, Subject: accountID}
	if sessions, ok := s.Auth.(localSessions); ok {
		if err := sessions.OpenSession(w, r, principal); err != nil && s.Logger != nil {
			s.Logger.Warn("Session after enrollment could not be opened", "error", err)
		}
	}
	if s.Logger != nil {
		s.Logger.Info("Local account enrolled", "account", account.Username, "remote", clientip.From(r))
	}
	jsonResponse(w, http.StatusOK, contract.LocalEnrollmentResult{RecoveryCodes: codes})
}

// localViewerHandler serves a viewer signed in with a local account.
type localViewerHandler func(w http.ResponseWriter, r *http.Request, viewer config.User, account config.LocalAccount) error

func (s *Server) localViewer(handler localViewerHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		viewer, allowed := s.Auth.Viewer(r)
		if !allowed {
			problem(w, http.StatusUnauthorized, "unauthenticated", "Sign in to manage your account.")
			return
		}
		if !s.sameOrigin(r) {
			problem(w, http.StatusForbidden, "forbidden", "Invalid request origin.")
			return
		}
		if s.Local == nil || s.Settings == nil || viewer.LocalAccountID == "" {
			problem(w, http.StatusForbidden, "forbidden", "Only local accounts manage their password here.")
			return
		}
		configuration, _ := s.Settings.Current()
		for _, account := range configuration.LocalAccounts {
			if account.ID == viewer.LocalAccountID {
				if err := handler(w, r, viewer, account); err != nil {
					s.fail(w, r, err)
				}
				return
			}
		}
		problem(w, http.StatusUnauthorized, "unauthenticated", "Sign in to manage your account.")
	}
}

// verifyOwner checks the account's current password and a second factor, with the
// same delays as sign-in.
func (s *Server) verifyOwner(ctx context.Context, account config.LocalAccount, password, code string) error {
	_, err := s.Local.Authenticate(ctx, localauth.Attempt{
		Username: account.Username, AccountID: account.ID, Password: password, Code: code,
	})
	var throttled localauth.ThrottledError
	switch {
	case errors.As(err, &throttled):
		return &problemError{
			status:  http.StatusTooManyRequests,
			code:    "rate_limited",
			message: "Too many failed attempts. Retry in " + strconv.Itoa(int(throttled.RetryAfter.Seconds())+1) + " seconds.",
		}
	case errors.Is(err, localauth.ErrRejected):
		return badRequest("The current password or code is incorrect.")
	case err != nil:
		return retryLater("Your account could not be checked. Retry.")
	}
	return nil
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request, _ config.User, account config.LocalAccount) error {
	input, err := decodeBody[contract.AccountPasswordInput](w, r)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := localauth.CheckPassword(input.NewPassword, account.Username); err != nil {
		return badRequest("Choose a different password: " + err.Error() + ".")
	}
	if err := s.verifyOwner(ctx, account, input.CurrentPassword, input.Code); err != nil {
		return err
	}
	if err := s.Local.ChangePassword(ctx, account.ID, account.Username, input.NewPassword); err != nil {
		return retryLater("The password could not be changed. Retry.")
	}
	if control, ok := s.Auth.(localSessionControl); ok {
		control.EndOtherLocalSessions(r, account.ID)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) replaceRecoveryCodes(w http.ResponseWriter, r *http.Request, _ config.User, account config.LocalAccount) error {
	input, err := decodeBody[contract.AccountVerification](w, r)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := s.verifyOwner(ctx, account, input.Password, input.Code); err != nil {
		return err
	}
	codes, err := s.Local.NewRecoveryCodes(ctx, account.ID)
	if err != nil {
		return retryLater("The recovery codes could not be replaced. Retry.")
	}
	jsonResponse(w, http.StatusOK, contract.LocalEnrollmentResult{RecoveryCodes: codes})
	return nil
}
