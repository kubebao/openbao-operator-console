// Package settings manages the versioned, encrypted console configuration.
package settings

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

var (
	ErrConflict = errors.New("configuration revision changed")
	ErrMissing  = errors.New("configuration is not initialized")
	ErrInvalid  = errors.New("configuration is invalid")
)

type Record struct {
	Revision int64
	Payload  []byte
}
type Change struct {
	Revision int64
	Actor    string
	Summary  string
	At       time.Time
}
type Repository interface {
	LoadSettings(context.Context) (Record, error)
	SaveSettings(context.Context, int64, []byte, string, string, []CredentialRecord, []string) (Record, error)
	LoadCredentials(context.Context) ([]CredentialRecord, error)
	SaveCredential(context.Context, int64, CredentialRecord) error
	SettingsHistory(context.Context) ([]Change, error)
}

type Service struct {
	mu               sync.RWMutex
	repository       Repository
	cipher           cipher.AEAD
	revision         int64
	configuration    config.Config
	changed          chan struct{}
	credentials      map[string]StoredCredential
	readerGeneration uint64
}

func New(ctx context.Context, repository Repository, key []byte, initial config.Config) (*Service, error) {
	return Open(ctx, repository, key, &initial)
}

// Open loads stored configuration. A nil seed leaves an empty database pending
// until a verified first administrator initializes it.
func Open(ctx context.Context, repository Repository, key []byte, initial *config.Config) (*Service, error) {
	if len(key) != 32 {
		return nil, errors.New("configuration key must contain 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	encryption, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	result := &Service{
		repository:  repository,
		cipher:      encryption,
		changed:     make(chan struct{}, 1),
		credentials: map[string]StoredCredential{},
	}
	stored, err := repository.LoadSettings(ctx)
	if errors.Is(err, ErrMissing) {
		if initial == nil {
			return result, nil
		}
		seed := clone(*initial)
		if err := seed.Validate(); err != nil {
			return nil, err
		}
		if !hasAdmin(seed) {
			return nil, errors.New("enroll at least one bootstrap console administrator")
		}
		if err := importReaders(&seed); err != nil {
			return nil, err
		}
		_, err = result.Save(ctx, 0, seed, "bootstrap", "Initial configuration imported")
		if err == nil {
			return result, nil
		}
		if errors.Is(err, ErrConflict) {
			stored, err = repository.LoadSettings(ctx)
		}
	}
	if err != nil {
		return nil, err
	}
	configuration, err := result.decrypt(stored.Payload)
	if err != nil {
		return nil, errors.New("configuration cannot be decrypted with the supplied key")
	}
	rows, err := repository.LoadCredentials(ctx)
	if err != nil {
		return nil, fmt.Errorf("load reader credentials: %w", err)
	}
	for _, row := range rows {
		state, err := result.decryptCredential(row)
		if err != nil {
			return nil, errors.New("reader credential cannot be decrypted with the supplied key")
		}
		result.credentials[row.ConnectionID] = StoredCredential{Revision: row.Revision, State: state}
	}
	for i := range configuration.Connections {
		connection := &configuration.Connections[i]
		if connection.Credential != nil {
			return nil, errors.New("stored configuration is invalid")
		}
		if connection.Remote {
			stored, found := result.credentials[connection.ID]
			if !found {
				return nil, errors.New("stored remote connection has no reader credential")
			}
			credential := stored.State.Credential
			connection.Credential = &credential
		}
	}
	if err := configuration.Validate(); err != nil {
		return nil, errors.New("stored configuration is invalid")
	}
	if !hasAdmin(configuration) {
		return nil, errors.New("stored configuration has no administrator")
	}
	result.configuration = configuration
	result.revision = stored.Revision
	return result, nil
}

func importReaders(configuration *config.Config) error {
	for i := range configuration.Connections {
		connection := &configuration.Connections[i]
		if connection.Kubeconfig == "" {
			continue
		}
		data, err := os.ReadFile(connection.Kubeconfig)
		if err != nil {
			return fmt.Errorf("read bootstrap reader credential for %q", connection.ID)
		}
		credential, err := config.ImportKubeconfig(data, connection.Context)
		if err != nil {
			return fmt.Errorf("bootstrap reader %q: %w", connection.ID, err)
		}
		connection.Credential = &credential
		connection.Kubeconfig = ""
		connection.Context = ""
	}
	return nil
}

// Initialize commits the first verified configuration with the same revision
// compare-and-swap used for later edits. A competing claim cannot overwrite it.
func (s *Service) Initialize(ctx context.Context, configuration config.Config, actor string) error {
	configuration = clone(configuration)
	if err := importReaders(&configuration); err != nil {
		return err
	}
	_, err := s.Save(ctx, 0, configuration, actor, "First administrator verified and configuration initialized")
	return err
}

func (s *Service) Initialized() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.revision > 0
}

func hasAdmin(c config.Config) bool {
	for _, user := range c.Users {
		if user.Admin {
			return true
		}
	}
	for _, group := range c.Groups {
		if group.Admin {
			return true
		}
	}
	return false
}

func clone(c config.Config) config.Config {
	result := c
	result.Connections = slices.Clone(c.Connections)
	for i := range result.Connections {
		connection := &result.Connections[i]
		connection.Namespaces = slices.Clone(connection.Namespaces)
		connection.Operators = slices.Clone(connection.Operators)
		if connection.Renewal != nil {
			renewal := *connection.Renewal
			connection.Renewal = &renewal
		}
		for j := range connection.Operators {
			connection.Operators[j].Namespaces = slices.Clone(connection.Operators[j].Namespaces)
		}
		if connection.Credential != nil {
			credential := *connection.Credential
			connection.Credential = &credential
		}
	}
	result.Users = slices.Clone(c.Users)
	for i := range result.Users {
		result.Users[i].Scopes = cloneScopes(result.Users[i].Scopes)
	}
	result.Groups = slices.Clone(c.Groups)
	for i := range result.Groups {
		result.Groups[i].Scopes = cloneScopes(result.Groups[i].Scopes)
	}
	result.Relationships = slices.Clone(c.Relationships)
	if c.Identity != nil {
		identity := *c.Identity
		result.Identity = &identity
	}
	return result
}

func cloneScopes(scopes []config.Scope) []config.Scope {
	result := slices.Clone(scopes)
	for i := range result {
		result[i].Namespaces = slices.Clone(result[i].Namespaces)
	}
	return result
}

func (s *Service) Current() (config.Config, int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.configuration), s.revision
}
func (s *Service) Changed() <-chan struct{} { return s.changed }
func (s *Service) Resolve(_ context.Context, subject string, groups []string) (config.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.configuration.Resolve(subject, groups)
}

func (s *Service) History(ctx context.Context) ([]Change, error) {
	return s.repository.SettingsHistory(ctx)
}

func (s *Service) Save(ctx context.Context, revision int64, configuration config.Config, actor, summary string) (int64, error) {
	return s.SaveReplacing(ctx, revision, configuration, actor, summary, nil)
}

// SaveReplacing replaces only the credentials named by the caller. Omitted
// credentials are resolved under the lock, so a stale edit cannot undo renewal.
func (s *Service) SaveReplacing(
	ctx context.Context,
	revision int64,
	configuration config.Config,
	actor, summary string,
	replacements []string,
) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if revision != s.revision {
		return 0, ErrConflict
	}
	configuration = clone(configuration)
	replace := map[string]bool{}
	for _, id := range replacements {
		replace[id] = true
	}
	writes := []CredentialRecord{}
	keep := []string{}
	states := map[string]StoredCredential{}
	for i := range configuration.Connections {
		connection := &configuration.Connections[i]
		if connection.Credential == nil {
			if connection.Remote {
				return 0, fmt.Errorf("%w: remote connection requires a reader credential", ErrInvalid)
			}
			continue
		}
		connection.Remote = true
		previous, exists := s.credentials[connection.ID]
		if exists && !replace[connection.ID] {
			credential := previous.State.Credential
			connection.Credential = &credential
		}
		policyChanged := false
		for _, old := range s.configuration.Connections {
			if old.ID == connection.ID {
				policyChanged = !reflect.DeepEqual(old.Renewal, connection.Renewal)
			}
		}
		state := previous
		if !exists || replace[connection.ID] || policyChanged {
			state = StoredCredential{Revision: previous.Revision + 1, State: initialCredential(*connection.Credential)}
			row, err := s.encryptCredential(connection.ID, state)
			if err != nil {
				return 0, err
			}
			writes = append(writes, row)
		}
		states[connection.ID] = state
		keep = append(keep, connection.ID)
		delete(replace, connection.ID)
	}
	if len(replace) != 0 {
		return 0, fmt.Errorf("%w: credential replacement references an unknown remote connection", ErrInvalid)
	}
	if err := configuration.Validate(); err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if !hasAdmin(configuration) {
		return 0, fmt.Errorf("%w: keep at least one console administrator", ErrInvalid)
	}
	stored := clone(configuration)
	for i := range stored.Connections {
		stored.Connections[i].Credential = nil
	}
	payload, err := s.encrypt(stored)
	if err != nil {
		return 0, err
	}
	record, err := s.repository.SaveSettings(ctx, revision, payload, actor, summary, writes, keep)
	if err != nil {
		return 0, err
	}
	s.configuration = configuration
	s.credentials = states
	s.revision = record.Revision
	s.notify()
	return s.revision, nil
}

func (s *Service) notify() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

// Apply serializes configuration commits and collection writes. A superseded
// reader cannot publish a batch after its connection or policy is replaced.
func (s *Service) Apply(ctx context.Context, revision int64, write func(context.Context) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if revision != s.revision {
		return ErrConflict
	}
	return write(ctx)
}

func (s *Service) encrypt(configuration config.Config) ([]byte, error) {
	plaintext, err := json.Marshal(configuration)
	if err != nil {
		return nil, err
	}
	defer clear(plaintext)
	nonce := make([]byte, s.cipher.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return s.cipher.Seal(nonce, nonce, plaintext, []byte("console-configuration-v1")), nil
}

func (s *Service) decrypt(payload []byte) (config.Config, error) {
	var result config.Config
	if len(payload) < s.cipher.NonceSize() {
		return result, errors.New("invalid ciphertext")
	}
	plaintext, err := s.cipher.Open(nil, payload[:s.cipher.NonceSize()], payload[s.cipher.NonceSize():], []byte("console-configuration-v1"))
	if err != nil {
		return result, err
	}
	defer clear(plaintext)
	err = json.Unmarshal(plaintext, &result)
	return result, err
}

func Summary(previous, next config.Config) string {
	areas := []string{}
	if !reflect.DeepEqual(previous.Connections, next.Connections) {
		areas = append(areas, "connections")
	}
	if !reflect.DeepEqual(previous.Users, next.Users) || !reflect.DeepEqual(previous.Groups, next.Groups) {
		areas = append(areas, "access")
	}
	if !reflect.DeepEqual(previous.Relationships, next.Relationships) {
		areas = append(areas, "relationships")
	}
	if previous.MaxBackupAgeHours != next.MaxBackupAgeHours || previous.StartupGraceMinutes != next.StartupGraceMinutes {
		areas = append(areas, "policies")
	}
	if !reflect.DeepEqual(previous.Identity, next.Identity) {
		areas = append(areas, "identity provider")
	}
	if len(areas) == 0 {
		return "Configuration saved"
	}
	return "Updated " + strings.Join(areas, ", ")
}
