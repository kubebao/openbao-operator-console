package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/auth"
	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/settings"
)

const setupCookie = "__Host-console-setup"
const setupSessionLifetime = 15 * time.Minute
const setupTokenLifetime = time.Hour

type setupSession struct {
	expires time.Time
	trial   uint64
}
type setupRate struct {
	start time.Time
	count int
}

// Setup protects the one-time claim of an uninitialized console. Only a token
// digest and short-lived browser tickets are retained in process memory.
type Setup struct {
	mu             sync.Mutex
	settings       *settings.Service
	authentication *auth.Authenticator
	seed           config.Config
	tokenDigest    [32]byte
	expires        time.Time
	callback       string
	sessions       map[string]setupSession
	peers          map[string]setupRate
	global         setupRate
	now            func() time.Time
}

func NewSetup(service *settings.Service, authentication *auth.Authenticator, token []byte, callback string, seed config.Config) (*Setup, error) {
	value := strings.TrimSpace(string(token))
	if !validSetupToken(value) {
		return nil, fmt.Errorf("setup token must contain 32 random bytes encoded as 64 hexadecimal or 43 URL-safe base64 characters")
	}
	parsed, err := url.Parse(callback)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Path != "/auth/callback" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return nil, fmt.Errorf("OIDC callback must be an HTTPS URL ending in /auth/callback")
	}
	if service == nil || authentication == nil || service.Initialized() {
		return nil, fmt.Errorf("browser setup requires an uninitialized configuration")
	}
	if err := seed.Validate(); err != nil {
		return nil, fmt.Errorf("setup enrollment is invalid")
	}
	return &Setup{settings: service, authentication: authentication, seed: seed, tokenDigest: sha256.Sum256([]byte(value)), expires: time.Now().Add(setupTokenLifetime), callback: callback, sessions: map[string]setupSession{}, peers: map[string]setupRate{}, now: time.Now}, nil
}

func validSetupToken(value string) bool {
	var data []byte
	var err error
	switch len(value) {
	case 64:
		data, err = hex.DecodeString(value)
	case 43:
		data, err = base64.RawURLEncoding.DecodeString(value)
	default:
		return false
	}
	if err != nil || len(data) != 32 {
		return false
	}
	// Reject placeholder files with one repeated byte. The installer must still
	// generate the value with a cryptographically secure random source.
	for _, b := range data[1:] {
		if b != data[0] {
			return true
		}
	}
	return false
}

func (s *Setup) status(request *http.Request) contract.SetupStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := contract.SetupStatus{State: contract.SetupStatusStateRequired, CallbackUrl: s.callback}
	if s.settings.Initialized() {
		result.State = contract.SetupStatusStateComplete
		return result
	}
	now := s.now()
	if !now.Before(s.expires) {
		result.State = contract.SetupStatusStateExpired
		return result
	}
	if cookie, err := request.Cookie(setupCookie); err == nil {
		if session, found := s.sessions[cookie.Value]; found && now.Before(session.expires) {
			result.State = contract.SetupStatusStateUnlocked
			result.SessionExpiresAt = &session.expires
		}
	}
	return result
}

func (s *Setup) rateAllowed(peer string, now time.Time) bool {
	for id, rate := range s.peers {
		if now.Sub(rate.start) >= time.Minute {
			delete(s.peers, id)
		}
	}
	if now.Sub(s.global.start) >= time.Minute {
		s.global = setupRate{start: now}
	}
	rate, found := s.peers[peer]
	if !found {
		if len(s.peers) >= 128 {
			return false
		}
		rate.start = now
	}
	if rate.count >= 5 || s.global.count >= 64 {
		return false
	}
	rate.count++
	s.global.count++
	s.peers[peer] = rate
	return true
}

func setupPeer(request *http.Request) string {
	peer, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		return request.RemoteAddr
	}
	return peer
}

func setupSessionCookie(w http.ResponseWriter, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: setupCookie, Value: value, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: age})
}

func (s *Setup) available(w http.ResponseWriter, request *http.Request) bool {
	if s.settings.Initialized() {
		problem(w, http.StatusConflict, "setup_complete", "Console setup is already complete.")
		return false
	}
	if !s.now().Before(s.expires) {
		problem(w, http.StatusGone, "setup_expired", "The setup window expired. Restart the console with a new setup token.")
		return false
	}
	return true
}

func (s *Setup) unlock(w http.ResponseWriter, request *http.Request) {
	input, err := decodeBody[contract.SetupUnlock](w, request)
	if err != nil {
		problem(w, 400, "invalid", "Enter a valid setup token.")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available(w, request) {
		return
	}
	now := s.now()
	if !s.rateAllowed(setupPeer(request), now) {
		w.Header().Set("Retry-After", "60")
		problem(w, http.StatusTooManyRequests, "rate_limited", "Too many setup attempts. Retry in one minute.")
		return
	}
	digest := sha256.Sum256([]byte(strings.TrimSpace(input.Token)))
	if subtle.ConstantTimeCompare(digest[:], s.tokenDigest[:]) != 1 {
		problem(w, http.StatusUnauthorized, "invalid_token", "The setup token was not accepted.")
		return
	}
	for id, session := range s.sessions {
		if !now.Before(session.expires) {
			delete(s.sessions, id)
		}
	}
	if cookie, err := request.Cookie(setupCookie); err == nil {
		delete(s.sessions, cookie.Value)
	}
	if len(s.sessions) >= 8 {
		problem(w, http.StatusTooManyRequests, "rate_limited", "Setup is already open in several browsers. Retry after those sessions expire.")
		return
	}
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		problem(w, 503, "unavailable", "Setup could not be opened. Retry.")
		return
	}
	id := base64.RawURLEncoding.EncodeToString(data)
	expires := now.Add(setupSessionLifetime)
	if s.expires.Before(expires) {
		expires = s.expires
	}
	s.sessions[id] = setupSession{expires: expires}
	setupSessionCookie(w, id, int(expires.Sub(now).Seconds()))
	jsonResponse(w, 200, contract.SetupStatus{State: contract.SetupStatusStateUnlocked, CallbackUrl: s.callback, SessionExpiresAt: &expires})
}

func (s *Setup) lock(w http.ResponseWriter, request *http.Request) {
	s.mu.Lock()
	if cookie, err := request.Cookie(setupCookie); err == nil {
		delete(s.sessions, cookie.Value)
	}
	s.mu.Unlock()
	setupSessionCookie(w, "", -1)
	jsonResponse(w, 200, s.status(request))
}

func (s *Setup) identity(w http.ResponseWriter, request *http.Request) {
	input, err := decodeBody[contract.IdentityInput](w, request)
	if err != nil {
		problem(w, 400, "invalid", "Check the identity provider fields.")
		return
	}
	cookie, err := request.Cookie(setupCookie)
	if err != nil {
		problem(w, 401, "setup_locked", "Enter the setup token before configuring the identity provider.")
		return
	}
	s.mu.Lock()
	if !s.available(w, request) {
		s.mu.Unlock()
		return
	}
	session, found := s.sessions[cookie.Value]
	now := s.now()
	if !found || !now.Before(session.expires) {
		s.mu.Unlock()
		problem(w, 401, "setup_locked", "The setup session expired. Enter the setup token again.")
		return
	}
	if !s.rateAllowed(setupPeer(request), now) {
		s.mu.Unlock()
		w.Header().Set("Retry-After", "60")
		problem(w, 429, "rate_limited", "Too many setup attempts. Retry in one minute.")
		return
	}
	session.trial++
	s.sessions[cookie.Value] = session
	s.mu.Unlock()
	if len(input.Issuer) > 4096 || len(input.ClientId) > 1024 || len(input.GroupsClaim) > 1024 || len(input.Ca) > 256<<10 || input.ClientSecret == nil || *input.ClientSecret == "" || len(*input.ClientSecret) > 64<<10 {
		problem(w, 400, "invalid", "Supply an issuer, client ID, and client secret within the supported limits.")
		return
	}
	identity := config.Identity{Issuer: input.Issuer, ClientID: input.ClientId, ClientSecret: *input.ClientSecret, CA: input.Ca, GroupsClaim: input.GroupsClaim}
	ctx, cancel := context.WithTimeout(request.Context(), 15*time.Second)
	defer cancel()
	providerContext, err := auth.ProviderContextPEM(ctx, []byte(identity.CA))
	if err != nil {
		problem(w, 400, "invalid", "Check the identity provider CA certificate.")
		return
	}
	candidate, err := auth.NewOIDC(providerContext, identity.Issuer, identity.ClientID, identity.ClientSecret, s.callback, nil)
	if err != nil {
		problem(w, 400, "invalid", "Identity discovery failed. Check the issuer, HTTPS trust, and client settings.")
		return
	}
	candidate.SetGroupsClaim(identity.GroupsClaim)
	location, err := s.authentication.BeginSetup(w, candidate, func(ctx context.Context, subject string, _ []string) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		active, found := s.sessions[cookie.Value]
		if !found || active.trial != session.trial || !s.now().Before(active.expires) || !s.now().Before(s.expires) || s.settings.Initialized() {
			return fmt.Errorf("setup claim is no longer available")
		}
		configuration := s.seed
		configuration.Identity = &identity
		configuration.Users = append([]config.User{}, configuration.Users...)
		enrolled := false
		for i := range configuration.Users {
			if configuration.Users[i].Subject == subject {
				configuration.Users[i].Admin = true
				enrolled = true
			}
		}
		if !enrolled {
			configuration.Users = append(configuration.Users, config.User{Subject: subject, Name: subject, Admin: true, Scopes: []config.Scope{}})
		}
		if err := s.settings.Initialize(ctx, configuration, subject); err != nil {
			return err
		}
		s.tokenDigest = [32]byte{}
		s.sessions = map[string]setupSession{}
		s.seed = config.Config{}
		s.authentication.ReplaceProvider(candidate)
		return nil
	})
	if err != nil {
		problem(w, 503, "unavailable", "Cannot begin identity verification. Retry.")
		return
	}
	jsonResponse(w, 200, contract.LoginRedirect{Url: location})
}

func (s *Server) setupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/setup", func(w http.ResponseWriter, request *http.Request) {
		if s.Setup == nil {
			jsonResponse(w, 200, contract.SetupStatus{State: contract.SetupStatusStateComplete, CallbackUrl: s.Origin + "/auth/callback"})
			return
		}
		jsonResponse(w, 200, s.Setup.status(request))
	})
	guard := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, request *http.Request) {
			if s.Setup == nil || s.Settings == nil || s.Settings.Initialized() {
				problem(w, 409, "setup_complete", "Console setup is already complete.")
				return
			}
			if request.Header.Get("Origin") != s.Origin {
				problem(w, 403, "forbidden", "Invalid request origin.")
				return
			}
			next(w, request)
		}
	}
	mux.HandleFunc("POST /api/v1/setup/unlock", guard(func(w http.ResponseWriter, request *http.Request) { s.Setup.unlock(w, request) }))
	mux.HandleFunc("POST /api/v1/setup/lock", guard(func(w http.ResponseWriter, request *http.Request) { s.Setup.lock(w, request) }))
	mux.HandleFunc("POST /api/v1/setup/identity", guard(func(w http.ResponseWriter, request *http.Request) { s.Setup.identity(w, request) }))
}
