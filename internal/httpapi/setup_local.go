package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/localauth"
	"github.com/kubebao/openbao-operator-console/internal/settings"
)

var localUsername = regexp.MustCompile(config.LocalUsernamePattern)

// unlockedSession returns the setup session that the request's cookie names and counts
// one attempt against its limit. It writes the problem and reports false otherwise. The
// caller holds s.mu.
func (s *Setup) unlockedSession(w http.ResponseWriter, request *http.Request) (string, setupSession, bool) {
	if s.Local == nil {
		problem(w, http.StatusNotFound, "not_found", "Local accounts are not available.")
		return "", setupSession{}, false
	}
	if !s.available(w) {
		return "", setupSession{}, false
	}
	cookie, err := request.Cookie(setupCookie)
	if err != nil {
		problem(w, http.StatusUnauthorized, "setup_locked", "Enter the setup token before creating the administrator.")
		return "", setupSession{}, false
	}
	now := s.now()
	session, found := s.sessions[cookie.Value]
	if !found || !now.Before(session.expires) {
		problem(w, http.StatusUnauthorized, "setup_locked", "The setup session expired. Enter the setup token again.")
		return "", setupSession{}, false
	}
	if !session.trials.allow(now, 5) {
		w.Header().Set("Retry-After", "60")
		problem(w, http.StatusTooManyRequests, "rate_limited", "Too many setup attempts. Retry in one minute.")
		return "", setupSession{}, false
	}
	s.sessions[cookie.Value] = session
	return cookie.Value, session, true
}

// authenticator creates the TOTP secret for the first local administrator. The secret
// stays in the setup session until the administrator proves it with a code.
func (s *Setup) authenticator(w http.ResponseWriter, request *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, session, ok := s.unlockedSession(w, request)
	if !ok {
		return
	}
	secret, err := localauth.NewTOTPSecret()
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "unavailable", "The authenticator could not be prepared. Retry.")
		return
	}
	// The label names the console host, so administrators of several consoles can tell
	// their entries apart.
	account := "administrator"
	if parsed, err := url.Parse(s.callback); err == nil && parsed.Hostname() != "" {
		// Authenticator apps split the label at a colon, so the port is left out.
		account = parsed.Hostname()
	}
	uri := localauth.TOTPURI(secret, account)
	code, err := localauth.NewQRCode(uri)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "unavailable", "The authenticator could not be prepared. Retry.")
		return
	}
	session.totpSecret = secret
	s.sessions[id] = session
	jsonResponse(w, http.StatusOK, contract.AuthenticatorEnrollment{
		Key:    localauth.TOTPKey(secret),
		Uri:    uri,
		QrSize: code.Size,
		QrPath: code.Path,
	})
}

// localAdministrator creates the first local administrator, initializes the console, and
// signs the administrator in. The credential is stored first: if another setup claim
// wins, the credential belongs to no account and is removed at the next start.
func (s *Setup) localAdministrator(w http.ResponseWriter, request *http.Request) {
	input, err := decodeBody[contract.LocalAdministratorInput](w, request)
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid", "Check the administrator fields.")
		return
	}
	s.mu.Lock()
	id, session, ok := s.unlockedSession(w, request)
	s.mu.Unlock()
	if !ok {
		return
	}
	if session.totpSecret == nil {
		problem(w, http.StatusBadRequest, "invalid", "Add the authenticator before creating the administrator.")
		return
	}
	if !localUsername.MatchString(input.Username) || len(input.Name) > 256 {
		problem(w, http.StatusBadRequest, "invalid", "Use a lowercase username of letters, digits, dots, hyphens, or underscores.")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 15*time.Second)
	defer cancel()
	enrollment, err := s.Local.NewEnrollment(ctx, input.Username, input.Password, session.totpSecret, input.Code)
	var weak localauth.PasswordError
	switch {
	case errors.As(err, &weak):
		problem(w, http.StatusBadRequest, "invalid", "Choose a different password: "+weak.Error()+".")
		return
	case errors.Is(err, localauth.ErrRejected):
		problem(w, http.StatusBadRequest, "invalid", "The authenticator code is not valid. Check the device time and enter a new code.")
		return
	case err != nil:
		problem(w, http.StatusServiceUnavailable, "unavailable", "The administrator could not be created. Retry.")
		return
	}
	accountID, err := config.NewLocalAccountID()
	if err == nil {
		err = s.Local.Store.Create(ctx, accountID, enrollment.Credential)
	}
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "unavailable", "The administrator could not be created. Retry.")
		return
	}
	if err := s.claimLocal(ctx, id, session, accountID, input); err != nil {
		writeProblem(w, err)
		return
	}
	setupSessionCookie(w, "", -1)
	principal := config.Principal{Issuer: config.LocalIssuer, Subject: accountID}
	if err := s.authentication.OpenSession(w, request, principal); err != nil && s.Logger != nil {
		// Setup is complete; the administrator can still sign in from the sign-in page.
		s.Logger.Warn("Session after local setup could not be opened", "error", err)
	}
	jsonResponse(w, http.StatusOK, contract.LocalEnrollmentResult{RecoveryCodes: enrollment.RecoveryCodes})
}

// claimLocal initializes the configuration with the local administrator if the setup
// session that prepared the authenticator is still the active claim.
func (s *Setup) claimLocal(
	ctx context.Context,
	id string,
	prepared setupSession,
	accountID string,
	input contract.LocalAdministratorInput,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	active, found := s.sessions[id]
	now := s.now()
	if !found || !slices.Equal(active.totpSecret, prepared.totpSecret) || !now.Before(active.expires) ||
		!now.Before(s.expires) || s.settings.Initialized() {
		return &problemError{status: http.StatusConflict, code: "setup_complete", message: "Console setup is no longer available."}
	}
	configuration := s.seed
	configuration.LocalAccounts = append(slices.Clone(configuration.LocalAccounts), config.LocalAccount{
		ID:       accountID,
		Username: input.Username,
		Name:     input.Name,
		Admin:    true,
		Scopes:   []config.Scope{},
	})
	if err := s.settings.Initialize(ctx, configuration, config.LocalSubject(input.Username)); err != nil {
		if errors.Is(err, settings.ErrConflict) {
			return &problemError{status: http.StatusConflict, code: "setup_complete", message: "Console setup is already complete."}
		}
		return &problemError{
			status:  http.StatusServiceUnavailable,
			code:    "unavailable",
			message: "The console could not be initialized. Retry.",
		}
	}
	s.tokenDigest = [32]byte{}
	s.sessions = map[string]setupSession{}
	s.seed = config.Config{}
	s.authentication.CompleteLocalSetup()
	return nil
}
