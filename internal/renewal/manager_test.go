package renewal

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/dc-tec/openbao-operator-console/internal/config"
	"github.com/dc-tec/openbao-operator-console/internal/settings"
)

type lifecycleRepository struct {
	mu     sync.Mutex
	target settings.RenewalTarget
	fail   bool
	saves  int
}

func (r *lifecycleRepository) CredentialTargets() []settings.RenewalTarget {
	r.mu.Lock()
	defer r.mu.Unlock()
	return []settings.RenewalTarget{r.target}
}
func (r *lifecycleRepository) UpdateCredential(_ context.Context, target settings.RenewalTarget, state settings.CredentialState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return errors.New("database unavailable")
	}
	if target.ConfigurationRevision != r.target.ConfigurationRevision || target.CredentialRevision != r.target.CredentialRevision {
		return settings.ErrConflict
	}
	r.target.State = state
	r.target.Connection.Credential = &state.Credential
	r.target.CredentialRevision++
	r.saves++
	return nil
}

type lifecycleIssuer struct {
	requests int
	err      error
}

func (i *lifecycleIssuer) Issue(_ context.Context, c config.Connection, now time.Time) (config.Credential, time.Time, error) {
	i.requests++
	credential := *c.Credential
	credential.Token = "replacement"
	return credential, now.Add(24 * time.Hour), i.err
}
func lifecycle() (*lifecycleRepository, *lifecycleIssuer, Manager) {
	credential := config.Credential{Server: "https://api.example", Token: "initial"}
	r := &lifecycleRepository{target: settings.RenewalTarget{ConfigurationRevision: 1, CredentialRevision: 1, Connection: config.Connection{ID: "one", Credential: &credential, Renewal: &config.TokenRenewal{Namespace: "reader-system", ServiceAccount: "reader"}}, State: settings.CredentialState{Credential: credential, State: settings.RenewalPending}}}
	issuer := &lifecycleIssuer{}
	return r, issuer, Manager{Repository: r, Issuer: issuer, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}
func TestRenewalSchedulePersistsAndRetainsCredentialOnFailure(t *testing.T) {
	r, issuer, manager := lifecycle()
	now := time.Now().UTC()
	manager.Round(context.Background(), now)
	if issuer.requests != 1 || r.target.State.State != settings.RenewalHealthy || !r.target.State.ExpiryVerified || r.target.ConfigurationRevision != 1 || r.target.CredentialRevision != 2 {
		t.Fatal("renewal did not commit independently")
	}
	next := r.target.State.NextAttemptAt
	if next == nil || next.Before(now.Add(7*time.Hour)) || next.After(now.Add(8*time.Hour+time.Second)) {
		t.Fatal("renewal did not leave an expiry margin")
	}
	manager.Round(context.Background(), now.Add(time.Hour))
	if issuer.requests != 1 {
		t.Fatal("renewal ignored its stored schedule")
	}
	issuer.err = &Failure{State: settings.RenewalPermissionDenied}
	lastSuccess := *r.target.State.LastSuccessAt
	manager.Round(context.Background(), *next)
	if r.target.State.State != settings.RenewalPermissionDenied || r.target.State.Credential.Token != "replacement" || !r.target.State.LastSuccessAt.Equal(lastSuccess) {
		t.Fatal("failure discarded the usable credential")
	}
	issuer.err = nil
	manager.Round(context.Background(), *r.target.State.NextAttemptAt)
	if r.target.State.State != settings.RenewalHealthy || r.target.State.Failures != 0 {
		t.Fatal("renewal did not recover before expiry")
	}
}
func TestExpiredAndRejectedTokensStopUntilReconnect(t *testing.T) {
	for _, verified := range []bool{true, false} {
		r, issuer, manager := lifecycle()
		now := time.Now().UTC()
		expired := now.Add(-time.Second)
		r.target.State.ExpiresAt = &expired
		r.target.State.ExpiryVerified = verified
		if !verified {
			issuer.err = &Failure{State: settings.RenewalReconnectRequired}
		}
		manager.Round(context.Background(), now)
		if r.target.State.State != settings.RenewalReconnectRequired || r.target.State.NextAttemptAt != nil {
			t.Fatal("expiry did not require reconnection")
		}
		if verified && issuer.requests != 0 {
			t.Fatal("used a known expired token")
		}
		manager.Round(context.Background(), now.Add(time.Hour))
		if r.saves != 1 || (!verified && issuer.requests != 1) {
			t.Fatal("repeated requests after reconnect became required")
		}
	}
}
func TestPausedManualAndFailedPersistenceNeverReplaceCredentials(t *testing.T) {
	r, issuer, manager := lifecycle()
	now := time.Now().UTC()
	r.target.Connection.Disabled = true
	manager.Round(context.Background(), now)
	r.target.Connection.Disabled = false
	policy := r.target.Connection.Renewal
	r.target.Connection.Renewal = nil
	manager.Round(context.Background(), now)
	if issuer.requests != 0 {
		t.Fatal("renewed an unmanaged or paused reader")
	}
	r.target.Connection.Renewal = policy
	r.fail = true
	manager.Round(context.Background(), now)
	if r.target.State.Credential.Token != "initial" || r.target.CredentialRevision != 1 {
		t.Fatal("failed persistence replaced the live credential")
	}
}
