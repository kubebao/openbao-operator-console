package renewal

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/settings"
)

type Issuer interface {
	Issue(context.Context, config.Connection, time.Time) (config.Credential, time.Time, error)
}
type Repository interface {
	CredentialTargets() []settings.RenewalTarget
	UpdateCredential(context.Context, settings.RenewalTarget, settings.CredentialState) error
}
type Manager struct {
	Repository Repository
	Issuer     Issuer
	Logger     *slog.Logger
}

func (m Manager) Run(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		m.Round(ctx, time.Now().UTC())
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m Manager) Round(ctx context.Context, now time.Time) {
	started := time.Now()
	var pending sync.WaitGroup
	semaphore := make(chan struct{}, 4)
	for _, target := range m.Repository.CredentialTargets() {
		if target.Connection.Renewal == nil || target.Connection.Disabled || target.State.State == settings.RenewalReconnectRequired {
			continue
		}
		scheduled := target.State.NextAttemptAt != nil && now.Before(*target.State.NextAttemptAt)
		expired := target.State.ExpiryVerified && target.State.ExpiresAt != nil && !now.Before(*target.State.ExpiresAt)
		if scheduled && !expired {
			continue
		}
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			pending.Wait()
			return
		}
		pending.Add(1)
		go func() {
			defer pending.Done()
			defer func() { <-semaphore }()
			m.renew(ctx, target, now.Add(time.Since(started)))
		}()
	}
	pending.Wait()
}

func (m Manager) renew(ctx context.Context, target settings.RenewalTarget, now time.Time) {
	state := target.State
	if state.ExpiryVerified && state.ExpiresAt != nil && !now.Before(*state.ExpiresAt) {
		state.State = settings.RenewalReconnectRequired
		state.NextAttemptAt = nil
	} else {
		attempt, cancel := context.WithTimeout(ctx, 45*time.Second)
		credential, expires, err := m.Issuer.Issue(attempt, target.Connection, now)
		cancel()
		if ctx.Err() != nil {
			return
		}
		state.LastAttemptAt = &now
		if err == nil {
			state.Credential = credential
			state.ExpiresAt = &expires
			state.ExpiryVerified = true
			state.State = settings.RenewalHealthy
			state.Failures = 0
			state.LastSuccessAt = &now
			delay := min(expires.Sub(now)/3, 8*time.Hour)
			delay -= time.Duration(rand.Float64() * 0.1 * float64(delay)) //nolint:gosec // Scheduling jitter, not a secret.
			next := now.Add(delay)
			state.NextAttemptAt = &next
		} else {
			state.Failures = min(state.Failures+1, 10)
			state.State = settings.RenewalRetrying
			var failure *Failure
			if errors.As(err, &failure) {
				state.State = failure.State
			}
			if state.State == settings.RenewalReconnectRequired {
				state.NextAttemptAt = nil
			} else {
				delay := min(30*time.Second*time.Duration(1<<min(state.Failures-1, 4)), 5*time.Minute)
				if state.ExpiryVerified && state.ExpiresAt != nil {
					delay = min(delay, max(time.Second, state.ExpiresAt.Sub(now)/2))
				}
				next := now.Add(delay)
				state.NextAttemptAt = &next
			}
		}
	}
	if err := m.Repository.UpdateCredential(ctx, target, state); err != nil && !errors.Is(err, settings.ErrConflict) && ctx.Err() == nil {
		if m.Logger != nil {
			m.Logger.Warn("Credential renewal could not be stored", "connection", target.Connection.ID)
		}
	}
}
