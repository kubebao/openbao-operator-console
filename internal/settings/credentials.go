package settings

import (
	"context"
	"crypto/rand"
	"encoding/json"
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
func (s *Service) UpdateCredential(ctx context.Context, target RenewalTarget, state CredentialState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	old := previous.State.Credential
	if state.Credential.Server != old.Server || state.Credential.CA != old.CA || state.Credential.Certificate != old.Certificate ||
		state.Credential.Key != old.Key {
		return fmt.Errorf("%w: renewal cannot change server or TLS identity", ErrInvalid)
	}
	connection.Access.Credential = &state.Credential
	if err := connection.ValidateRenewal(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	stored := StoredCredential{Revision: previous.Revision + 1, State: cloneCredential(state)}
	row, err := s.encryptCredential(connection.ID, stored)
	if err != nil {
		return err
	}
	if err := s.repository.SaveCredential(ctx, s.revision, row); err != nil {
		return err
	}
	s.credentials[connection.ID] = stored
	if !reflect.DeepEqual(old, state.Credential) {
		for i := range s.configuration.Connections {
			if s.configuration.Connections[i].ID == connection.ID {
				credential := state.Credential
				s.configuration.Connections[i].Access.Credential = &credential
			}
		}
		s.readerGeneration++
		s.notify()
	}
	return nil
}

func (s *Service) ReaderConfiguration() (config.Config, int64, uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.configuration), s.revision, s.readerGeneration
}

func (s *Service) ApplyCollection(ctx context.Context, revision int64, generation uint64, write func(context.Context) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if revision != s.revision || generation != s.readerGeneration {
		return ErrConflict
	}
	return write(ctx)
}

func (s *Service) encryptCredential(id string, stored StoredCredential) (CredentialRecord, error) {
	plaintext, err := json.Marshal(stored.State)
	if err != nil {
		return CredentialRecord{}, err
	}
	defer clear(plaintext)
	nonce := make([]byte, s.cipher.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return CredentialRecord{}, err
	}
	return CredentialRecord{
		ConnectionID: id,
		Revision:     stored.Revision,
		Payload:      s.cipher.Seal(nonce, nonce, plaintext, []byte("console-reader-credential-v1:"+id)),
	}, nil
}

func (s *Service) decryptCredential(row CredentialRecord) (CredentialState, error) {
	var state CredentialState
	if row.Revision < 1 || len(row.Payload) < s.cipher.NonceSize() {
		return state, errors.New("invalid reader ciphertext")
	}
	plaintext, err := s.cipher.Open(
		nil,
		row.Payload[:s.cipher.NonceSize()],
		row.Payload[s.cipher.NonceSize():],
		[]byte("console-reader-credential-v1:"+row.ConnectionID),
	)
	if err != nil {
		return state, err
	}
	defer clear(plaintext)
	if err := json.Unmarshal(plaintext, &state); err != nil {
		return state, err
	}
	return state, state.validate()
}
