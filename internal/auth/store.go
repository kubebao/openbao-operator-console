package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

const (
	maxTransactions = 1000
	maxSessions     = 10000
	loginLifetime   = 5 * time.Minute
)

var errCapacity = errors.New("login capacity reached")

type session struct {
	subject string
	groups  []string
	expires time.Time
}

// transaction is one pending login. It keeps the provider it started with, so a
// provider trial completes against the candidate rather than the active provider.
type transaction struct {
	verifier, nonce string
	expires         time.Time
	provider        *Provider
	generation      int64
	returnTo        string
	activate        func(context.Context, string, []string) error
	setup           bool
}

// store keeps login transactions and sessions in memory. Replacing the identity provider
// starts a new generation, which ends every session and pending login.
type store struct {
	mu           sync.Mutex
	generation   int64
	sessions     map[string]session
	transactions map[string]transaction
}

func newStore() *store {
	return &store{sessions: map[string]session{}, transactions: map[string]transaction{}}
}

func randomToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

// begin records a login in the current generation and returns its state. It removes
// expired entries first and refuses new logins at capacity.
func (s *store) begin(tx transaction) (string, error) {
	state, err := randomToken()
	if err != nil {
		return "", err
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, pending := range s.transactions {
		if !now.Before(pending.expires) {
			delete(s.transactions, id)
		}
	}
	for id, active := range s.sessions {
		if !now.Before(active.expires) {
			delete(s.sessions, id)
		}
	}
	if len(s.transactions) >= maxTransactions || len(s.sessions) >= maxSessions {
		return "", errCapacity
	}
	tx.expires = now.Add(loginLifetime)
	tx.generation = s.generation
	s.transactions[state] = tx
	return state, nil
}

// take removes a login so that its state cannot be replayed. It reports false when the
// login is unknown, expired, or from an earlier generation; the transaction is still
// returned so that the caller can choose the failure destination.
func (s *store) take(state string) (transaction, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, exists := s.transactions[state]
	delete(s.transactions, state)
	return tx, exists && time.Now().Before(tx.expires) && tx.generation == s.generation
}

// open creates a session if the generation is still current.
func (s *store) open(generation int64, active session) (string, error) {
	id, err := randomToken()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.generation || len(s.sessions) >= maxSessions {
		return "", errCapacity
	}
	s.sessions[id] = active
	return id, nil
}

func (s *store) lookup(id string) (session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	active, exists := s.sessions[id]
	if !exists || !time.Now().Before(active.expires) {
		delete(s.sessions, id)
		return session{}, false
	}
	return active, true
}

func (s *store) end(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func (s *store) current() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.generation
}

func (s *store) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.generation++
	s.sessions = map[string]session{}
	s.transactions = map[string]transaction{}
}
