// Package settings manages the versioned, encrypted console configuration.
package settings

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
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
		access := &configuration.Connections[i].Access
		if access.Mode != config.AccessRemote {
			continue
		}
		stored, found := result.credentials[configuration.Connections[i].ID]
		if !found {
			return nil, errors.New("stored remote connection has no reader credential")
		}
		credential := stored.State.Credential
		access.Credential = &credential
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

// importReaders replaces kubeconfig file references from the enrollment file with
// embedded remote credentials. The console never reads those files again.
func importReaders(configuration *config.Config) error {
	for i := range configuration.Connections {
		connection := &configuration.Connections[i]
		if connection.Access.Mode != config.AccessKubeconfig {
			continue
		}
		if connection.Access.Kubeconfig == "" {
			return fmt.Errorf("bootstrap reader %q requires a kubeconfig file", connection.ID)
		}
		data, err := os.ReadFile(connection.Access.Kubeconfig)
		if err != nil {
			return fmt.Errorf("read bootstrap reader credential for %q", connection.ID)
		}
		credential, err := config.ImportKubeconfig(data, connection.Access.Context)
		if err != nil {
			return fmt.Errorf("bootstrap reader %q: %w", connection.ID, err)
		}
		connection.Access = config.Access{Mode: config.AccessRemote, Credential: &credential}
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
		if connection.Access.Credential != nil {
			credential := *connection.Access.Credential
			connection.Access.Credential = &credential
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

// Save replaces the configuration. Remote connections keep their stored reader credentials.
func (s *Service) Save(ctx context.Context, revision int64, configuration config.Config, actor, summary string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save(ctx, revision, configuration, actor, summary, nil)
}

// save replaces only the credentials named by the caller. Omitted credentials are
// resolved under the lock, so a stale edit cannot undo renewal. The caller holds s.mu.
func (s *Service) save(
	ctx context.Context,
	revision int64,
	configuration config.Config,
	actor, summary string,
	replacements []string,
) (int64, error) {
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
		switch {
		case connection.Access.Mode == config.AccessKubeconfig:
			return 0, invalid("import kubeconfig access before saving")
		case connection.Access.Mode != config.AccessRemote:
			continue
		case connection.Access.Credential == nil:
			return 0, invalid("remote connection requires a reader credential")
		}
		previous, exists := s.credentials[connection.ID]
		if exists && !replace[connection.ID] {
			credential := previous.State.Credential
			connection.Access.Credential = &credential
		}
		policyChanged := false
		for _, old := range s.configuration.Connections {
			if old.ID == connection.ID {
				policyChanged = !reflect.DeepEqual(old.Renewal, connection.Renewal)
			}
		}
		state := previous
		if !exists || replace[connection.ID] || policyChanged {
			state = StoredCredential{Revision: previous.Revision + 1, State: initialCredential(*connection.Access.Credential)}
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
		return 0, invalid("credential replacement references an unknown remote connection")
	}
	if err := configuration.Validate(); err != nil {
		return 0, invalid("%s", err)
	}
	if !hasAdmin(configuration) {
		return 0, invalid("keep at least one console administrator")
	}
	// Reader credentials are excluded from the configuration payload and stored separately.
	payload, err := s.encrypt(configuration)
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

func (s *Service) encrypt(configuration config.Config) ([]byte, error) {
	return s.seal(configuration, configurationPurpose)
}

func (s *Service) decrypt(payload []byte) (config.Config, error) {
	var result config.Config
	err := s.unseal(payload, configurationPurpose, &result)
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
