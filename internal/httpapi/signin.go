package httpapi

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/clientip"
	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/localauth"
)

const (
	// clientFailureLimit bounds failed local sign-ins per client and minute. The
	// per-account delay is the main defense; this limit slows password spraying across
	// accounts. It counts failures only, so a client that shares an address with an
	// attacker can still sign in once the minute passes.
	clientFailureLimit = 30
	// maxLimitedClients bounds the client table. A full table refuses new failing clients
	// until entries expire.
	maxLimitedClients = 4096
)

// localSessions opens sessions for verified local accounts.
type localSessions interface {
	OpenSession(http.ResponseWriter, *http.Request, config.Principal) error
	SingleSignOn() bool
}

// clientLimiter counts failures per client address in one-minute windows.
type clientLimiter struct {
	mu      sync.Mutex
	clients map[string]setupRate
}

func (l *clientLimiter) blocked(client string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	rate, found := l.clients[client]
	if !found {
		return false
	}
	if now.Sub(rate.start) >= time.Minute {
		delete(l.clients, client)
		return false
	}
	return rate.count >= clientFailureLimit
}

func (l *clientLimiter) failure(client string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.clients == nil {
		l.clients = map[string]setupRate{}
	}
	if _, found := l.clients[client]; !found && len(l.clients) >= maxLimitedClients {
		for id, rate := range l.clients {
			if now.Sub(rate.start) >= time.Minute {
				delete(l.clients, id)
			}
		}
		if len(l.clients) >= maxLimitedClients {
			return
		}
	}
	rate := l.clients[client]
	rate.allow(now, math.MaxInt)
	l.clients[client] = rate
}

func (s *Server) signInRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/sign-in", s.signInOptions)
	mux.HandleFunc("POST /api/v1/sign-in/local", s.signInLocal)
}

func (s *Server) signInOptions(w http.ResponseWriter, _ *http.Request) {
	result := contract.SignInOptions{}
	if sessions, ok := s.Auth.(localSessions); ok {
		result.SingleSignOn = sessions.SingleSignOn()
	}
	if s.Local != nil && s.Settings != nil {
		configuration, _ := s.Settings.Current()
		result.LocalAccounts = len(configuration.LocalAccounts) > 0
	}
	jsonResponse(w, http.StatusOK, result)
}

func (s *Server) logSignIn(request *http.Request, message, account, reason string) {
	if s.Logger != nil {
		s.Logger.Warn(message, "account", account, "reason", reason, "remote", clientip.From(request))
	}
}

// signInLocal verifies a username, password, and second factor and opens a session.
// Every rejected attempt returns the same response, whether the account exists or not.
func (s *Server) signInLocal(w http.ResponseWriter, request *http.Request) {
	sessions, ok := s.Auth.(localSessions)
	if s.Local == nil || s.Settings == nil || !ok {
		problem(w, http.StatusNotFound, "not_found", "Local sign-in is not available.")
		return
	}
	if !s.sameOrigin(request) {
		problem(w, http.StatusForbidden, "forbidden", "Invalid request origin.")
		return
	}
	input, err := decodeBody[contract.LocalSignInInput](w, request)
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid", "Enter a username, password, and code.")
		return
	}
	client := clientip.From(request)
	now := time.Now()
	if s.signInLimits.blocked(client, now) {
		s.logSignIn(request, "Local sign-in rejected", localauth.ThrottleKey(input.Username), "client_rate_limited")
		w.Header().Set("Retry-After", "60")
		problem(w, http.StatusTooManyRequests, "rate_limited", "Too many failed sign-ins. Retry in one minute.")
		return
	}
	configuration, _ := s.Settings.Current()
	account, found := configuration.FindLocalAccount(input.Username)
	attempt := localauth.Attempt{Username: input.Username, Password: input.Password, Code: input.Code}
	if found {
		attempt.AccountID = account.ID
	}
	ctx, cancel := context.WithTimeout(request.Context(), 15*time.Second)
	defer cancel()
	result, err := s.Local.Authenticate(ctx, attempt)
	var throttled localauth.ThrottledError
	switch {
	case errors.As(err, &throttled):
		s.logSignIn(request, "Local sign-in rejected", localauth.ThrottleKey(input.Username), "account_delayed")
		seconds := int(math.Ceil(throttled.RetryAfter.Seconds()))
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		problem(
			w,
			http.StatusTooManyRequests,
			"rate_limited",
			"Too many failed sign-ins for this username. Retry in "+strconv.Itoa(seconds)+" seconds.",
		)
		return
	case errors.Is(err, localauth.ErrRejected):
		s.signInLimits.failure(client, now)
		s.logSignIn(request, "Local sign-in rejected", localauth.ThrottleKey(input.Username), "invalid_credentials")
		problem(w, http.StatusUnauthorized, "unauthenticated", "The username, password, or code is incorrect.")
		return
	case err != nil:
		problem(w, http.StatusServiceUnavailable, "unavailable", "Sign-in is temporarily unavailable. Retry.")
		return
	}
	principal := config.Principal{Issuer: config.LocalIssuer, Subject: account.ID}
	// The account may have been removed while its password was checked.
	if _, allowed := s.Settings.Resolve(ctx, principal); !allowed {
		problem(w, http.StatusUnauthorized, "unauthenticated", "The username, password, or code is incorrect.")
		return
	}
	if err := sessions.OpenSession(w, request, principal); err != nil {
		problem(w, http.StatusServiceUnavailable, "unavailable", "Sign-in is temporarily unavailable. Retry.")
		return
	}
	if s.Logger != nil {
		s.Logger.Info(
			"Local sign-in",
			"account", account.Username,
			"recoveryCodeUsed", result.RecoveryCodeUsed,
			"remote", clientip.From(request),
		)
	}
	jsonResponse(w, http.StatusOK, contract.LocalSignInResult{
		RecoveryCodeUsed:  result.RecoveryCodeUsed,
		RecoveryCodesLeft: result.RecoveryCodesLeft,
	})
}
