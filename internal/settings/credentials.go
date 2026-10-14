package settings

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

type RenewalState string

const (
	RenewalPending           RenewalState = "pending"
	RenewalHealthy           RenewalState = "healthy"
	RenewalRetrying          RenewalState = "retrying"
	RenewalPermissionDenied  RenewalState = "permissionDenied"
	RenewalReconnectRequired RenewalState = "reconnectRequired"
)

type CredentialState struct {
	Credential     config.Credential `json:"credential"`
	State          RenewalState      `json:"state"`
	ExpiresAt      *time.Time        `json:"expiresAt"`
	ExpiryVerified bool              `json:"expiryVerified"`
	LastAttemptAt  *time.Time        `json:"lastAttemptAt"`
	LastSuccessAt  *time.Time        `json:"lastSuccessAt"`
	NextAttemptAt  *time.Time        `json:"nextAttemptAt"`
	Failures       int               `json:"failures"`
}
type StoredCredential struct {
	Revision int64
	State    CredentialState
}
type CredentialRecord struct {
	ConnectionID string
	Revision     int64
	Payload      []byte
}
type RenewalTarget struct {
	Connection            config.Connection
	ConfigurationRevision int64
	CredentialRevision    int64
	State                 CredentialState
}

func (state CredentialState) validate() error {
	if err := state.Credential.Validate(); err != nil {
		return errors.New("invalid reader credential")
	}
	switch state.State {
	case RenewalPending, RenewalHealthy, RenewalRetrying, RenewalPermissionDenied, RenewalReconnectRequired:
	default:
		return errors.New("invalid credential renewal state")
	}
	if state.Failures < 0 || state.Failures > 10 || (state.ExpiryVerified && state.ExpiresAt == nil) {
		return errors.New("invalid credential renewal evidence")
	}
	scheduled := state.ExpiryVerified && state.LastSuccessAt != nil && state.NextAttemptAt != nil &&
		state.NextAttemptAt.Before(*state.ExpiresAt)
	if state.State == RenewalHealthy && !scheduled {
		return errors.New("healthy credential requires a verified expiry and renewal schedule")
	}
	if state.State == RenewalReconnectRequired && state.NextAttemptAt != nil {
		return errors.New("reconnection cannot have a renewal schedule")
	}
	return nil
}

func initialCredential(credential config.Credential) CredentialState {
	return CredentialState{Credential: credential, State: RenewalPending, ExpiresAt: credential.Expiry().ExpiresAt}
}

func cloneCredential(state CredentialState) CredentialState {
	copyTime := func(value *time.Time) *time.Time {
		if value == nil {
			return nil
		}
		copied := *value
		return &copied
	}
	state.ExpiresAt = copyTime(state.ExpiresAt)
	state.LastAttemptAt = copyTime(state.LastAttemptAt)
	state.LastSuccessAt = copyTime(state.LastSuccessAt)
	state.NextAttemptAt = copyTime(state.NextAttemptAt)
	return state
}

func (s *Service) CredentialTargets() []RenewalTarget {
	s.mu.RLock()
	defer s.mu.RUnlock()
	current := clone(s.configuration)
	result := []RenewalTarget{}
	for _, connection := range current.Connections {
		if stored, found := s.credentials[connection.ID]; found {
			result = append(
				result,
				RenewalTarget{
					Connection:            connection,
					ConfigurationRevision: s.revision,
					CredentialRevision:    stored.Revision,
					State:                 cloneCredential(stored.State),
				},
			)
		} else {
			result = append(result, RenewalTarget{Connection: connection, ConfigurationRevision: s.revision})
		}
	}
	return result
}

// UpdateCredential never changes enrollment or its revision. Both revision
// guards prevent a delayed request from undoing a replacement, pause, or removal.
// Like save, it holds s.commit across the write and takes s.mu only to publish.
func (s *Service) UpdateCredential(ctx context.Context, target RenewalTarget, state CredentialState) error {
	s.commit.Lock()
	defer s.commit.Unlock()
	previous, found := s.credentials[target.Connection.ID]
	if !found || s.revision != target.ConfigurationRevision || previous.Revision != target.CredentialRevision {
		return ErrConflict
	}
	var connection config.Connection
	for _, enrolled := range s.configuration.Connections {
		if enrolled.ID == target.Connection.ID {
			connection = enrolled
			break
		}
	}
	if connection.ID == "" || connection.Disabled || connection.Renewal == nil {
		return ErrConflict
	}
	if err := state.validate(); err != nil {
		return invalid("%s", err)
	}
	old := previous.State.Credential
	if state.Credential.Server != old.Server || state.Credential.CA != old.CA || state.Credential.Certificate != old.Certificate ||
		state.Credential.Key != old.Key {
		return invalid("renewal cannot change server or TLS identity")
	}
	connection.Access.Credential = &state.Credential
	if err := connection.ValidateRenewal(); err != nil {
		return invalid("%s", err)
	}
	stored := StoredCredential{Revision: previous.Revision + 1, State: cloneCredential(state)}
	row, err := s.encryptCredential(connection.ID, stored)
	if err != nil {
		return err
	}
	if err := s.repository.SaveCredential(ctx, s.revision, row); err != nil {
		// The write may have committed, or the stored revisions moved on. Reload so the
		// next renewal round starts from the stored credential.
		s.refresh(ctx)
		return fmt.Errorf("save reader credential: %w", err)
	}
	changed := !reflect.DeepEqual(old, state.Credential)
	s.mu.Lock()
	s.credentials[connection.ID] = stored
	if changed {
		for i := range s.configuration.Connections {
			if s.configuration.Connections[i].ID == connection.ID {
				credential := state.Credential
				s.configuration.Connections[i].Access.Credential = &credential
			}
		}
		s.readerGeneration++
	}
	s.mu.Unlock()
	if changed {
		s.notify()
	}
	return nil
}

func (s *Service) ReaderConfiguration() (config.Config, int64, uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.configuration), s.revision, s.readerGeneration
}

// ApplyCollection writes a collection result only while its configuration revision and
// reader generation are current. It shares s.commit with other collections, so commits
// wait for in-flight collection writes, but readers of the settings never do.
func (s *Service) ApplyCollection(ctx context.Context, revision int64, generation uint64, write func(context.Context) error) error {
	s.commit.RLock()
	defer s.commit.RUnlock()
	if revision != s.revision || generation != s.readerGeneration {
		return ErrConflict
	}
	return write(ctx)
}

func (s *Service) encryptCredential(id string, stored StoredCredential) (CredentialRecord, error) {
	payload, err := s.seal(stored.State, credentialData(id, stored.Revision))
	if err != nil {
		return CredentialRecord{}, err
	}
	return CredentialRecord{ConnectionID: id, Revision: stored.Revision, Payload: payload}, nil
}

func (s *Service) decryptCredential(row CredentialRecord) (CredentialState, error) {
	var state CredentialState
	if row.Revision < 1 {
		return state, errors.New("invalid reader ciphertext")
	}
	if err := s.unseal(row.Payload, credentialData(row.ConnectionID, row.Revision), &state); err != nil {
		return state, err
	}
	return state, state.validate()
}
