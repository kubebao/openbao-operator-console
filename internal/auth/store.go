package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

const (
	maxSessions = 10000
	// maxConsumed bounds the replay set. When it is full the oldest entry is evicted; a
	// new login is never refused. The provider still rejects a reused authorization code.
	maxConsumed = 32768
	// maxTrials bounds pending identity provider trials. Only administrators and unlocked
	// setup sessions start them; the oldest is evicted when the set is full.
	maxTrials     = 32
	loginLifetime = 5 * time.Minute
)

var errCapacity = errors.New("login capacity reached")

type session struct {
	principal config.Principal
	expires   time.Time
}

// trial is a pending sign-in through a candidate identity provider. It stays on the
// server because it holds the candidate provider and the activation step.
type trial struct {
	provider *Provider
	activate Activation
	setup    bool
	expires  time.Time
}

// store keeps sessions, consumed login states, and pending identity provider trials in
// memory. Ordinary pending logins live in sealed browser cookies, so unauthenticated
// traffic adds nothing here. Replacing the identity provider starts a new generation,
// which ends every session and pending login.
type store struct {
	mu         sync.Mutex
	generation int64
	sessions   map[string]session
	consumed   map[string]time.Time
	order      []string // consumed states, oldest first
	trials     map[string]trial
	trialOrder []string // trial IDs, oldest first
}

func newStore() *store {
	return &store{sessions: map[string]session{}, consumed: map[string]time.Time{}, trials: map[string]trial{}}
}

func randomToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

// beginTrial records a candidate provider trial and returns its ID. It removes expired
// trials and evicts the oldest when the set is full.
func (s *store) beginTrial(pending trial) (string, error) {
	id, err := randomToken()
	if err != nil {
		return "", err
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.trialOrder) > 0 {
		oldest := s.trialOrder[0]
		entry, found := s.trials[oldest]
		if found && now.Before(entry.expires) && len(s.trials) < maxTrials {
			break
		}
		delete(s.trials, oldest)
		s.trialOrder = s.trialOrder[1:]
	}
	pending.expires = now.Add(loginLifetime)
	s.trials[id] = pending
	s.trialOrder = append(s.trialOrder, id)
	return id, nil
}

// consume marks a login state as used and returns its trial, if it names one. It
// reports false for a login from an earlier generation, a replayed state, or a trial
// that expired or was evicted.
func (s *store) consume(login pendingLogin, now time.Time) (trial, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if login.Generation != s.generation {
		return trial{}, false
	}
	for len(s.order) > 0 {
		oldest := s.order[0]
		if now.Before(s.consumed[oldest]) && len(s.consumed) < maxConsumed {
			break
		}
		delete(s.consumed, oldest)
		s.order = s.order[1:]
	}
	if _, used := s.consumed[login.State]; used {
		return trial{}, false
	}
	s.consumed[login.State] = time.Unix(login.Expires, 0)
	s.order = append(s.order, login.State)
	if login.Trial == "" {
		return trial{}, true
	}
	pending, found := s.trials[login.Trial]
	delete(s.trials, login.Trial)
	return pending, found && now.Before(pending.expires)
}

// open creates a session if the generation is still current.
func (s *store) open(generation int64, active session) (string, error) {
	id, err := randomToken()
	if err != nil {
		return "", err
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.generation {
		return "", errCapacity
	}
	if len(s.sessions) >= maxSessions {
		for key, existing := range s.sessions {
			if !now.Before(existing.expires) {
				delete(s.sessions, key)
			}
		}
	}
	if len(s.sessions) >= maxSessions {
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
	s.trials = map[string]trial{}
	s.trialOrder = nil
}
