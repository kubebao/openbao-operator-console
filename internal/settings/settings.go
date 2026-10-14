// Package settings manages the versioned, encrypted console configuration.
package settings

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
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

// refreshTimeout bounds the reload after a commit with an unknown outcome.
const refreshTimeout = 5 * time.Second

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
	// commit serializes configuration and credential commits with collection writes, so
	// a collection result never lands after the configuration that superseded it. It is
	// held across database I/O. Request handlers that only read never take it.
	commit sync.RWMutex
	// mu guards the published state below and is never held across database I/O. The
	// state changes only while both locks are held, so either lock is enough to read it.
	mu               sync.RWMutex
	repository       Repository
	cipher           cipher.AEAD
	revision         int64
	configuration    config.Config
	changed          chan struct{}
	credentials      map[string]StoredCredential
	readerGeneration uint64
}

// stored is the configuration and the reader credentials read from the repository.
type stored struct {
	revision      int64
	payload       []byte
	configuration config.Config
	credentials   map[string]StoredCredential
}

// Open loads stored configuration. An empty database is initialized from the seed; a
// nil seed leaves it pending until a verified first administrator initializes it.
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
	state, err := result.load(ctx)
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
			state, err = result.load(ctx)
		}
	}
	if err != nil {
		return nil, err
	}
	result.publish(state)
	return result, nil
}

// load reads, decrypts, and validates the stored configuration and reader credentials.
func (s *Service) load(ctx context.Context) (stored, error) {
	record, err := s.repository.LoadSettings(ctx)
	if err != nil {
		return stored{}, err
	}
	configuration, err := s.decrypt(record.Payload, record.Revision)
	if err != nil {
		return stored{}, errors.New("configuration cannot be decrypted with the supplied key")
	}
	rows, err := s.repository.LoadCredentials(ctx)
	if err != nil {
		return stored{}, fmt.Errorf("load reader credentials: %w", err)
	}
	credentials := map[string]StoredCredential{}
	for _, row := range rows {
		state, err := s.decryptCredential(row)
		if err != nil {
			return stored{}, errors.New("reader credential cannot be decrypted with the supplied key")
		}
		credentials[row.ConnectionID] = StoredCredential{Revision: row.Revision, State: state}
	}
	for i := range configuration.Connections {
		access := &configuration.Connections[i].Access
		if access.Mode != config.AccessRemote {
			continue
		}
		reader, found := credentials[configuration.Connections[i].ID]
		if !found {
			return stored{}, errors.New("stored remote connection has no reader credential")
		}
		credential := reader.State.Credential
		access.Credential = &credential
	}
	if err := configuration.Validate(); err != nil {
		return stored{}, errors.New("stored configuration is invalid")
	}
	if !hasAdmin(configuration) {
		return stored{}, errors.New("stored configuration has no administrator")
	}
	return stored{revision: record.Revision, payload: record.Payload, configuration: configuration, credentials: credentials}, nil
}

// publish replaces the in-memory state. A changed revision or reader credential starts a
// new reader generation, so collectors stop writing results from the previous one.
func (s *Service) publish(state stored) {
	s.mu.Lock()
	changed := state.revision != s.revision || len(state.credentials) != len(s.credentials)
	for id, credential := range state.credentials {
		previous, found := s.credentials[id]
		changed = changed || !found || previous.Revision != credential.Revision
	}
	s.revision = state.revision
	s.configuration = state.configuration
	s.credentials = state.credentials
	if changed {
		s.readerGeneration++
	}
	s.mu.Unlock()
	if changed {
		s.notify()
	}
}

// refresh publishes the stored state after a commit whose outcome is unknown or that lost
// a revision race. Later edits and renewals then start from the stored revisions instead
// of conflicting until restart. It returns the stored state when the read succeeded. The
// caller holds s.commit.
func (s *Service) refresh(ctx context.Context) (stored, bool) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), refreshTimeout)
	defer cancel()
	state, err := s.load(ctx)
	if err != nil {
		return stored{}, false
	}
	s.publish(state)
	return state, true
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
func (s *Service) Resolve(_ context.Context, principal config.Principal) (config.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.configuration.Resolve(principal)
}

func (s *Service) History(ctx context.Context) ([]Change, error) {
	return s.repository.SettingsHistory(ctx)
}

// Save replaces the configuration. Remote connections keep their stored reader credentials.
func (s *Service) Save(ctx context.Context, revision int64, configuration config.Config, actor, summary string) (int64, error) {
	s.commit.Lock()
	defer s.commit.Unlock()
	return s.save(ctx, revision, configuration, actor, summary, nil)
}

// save replaces only the credentials named by the caller. Omitted credentials are
// resolved under the commit lock, so a stale edit cannot undo renewal. The caller holds
// s.commit, which excludes every other writer, so save reads the published state without
// s.mu and takes s.mu only to publish.
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
	next := revision + 1
	payload, err := s.encrypt(configuration, next)
	if err != nil {
		return 0, err
	}
	if _, err := s.repository.SaveSettings(ctx, revision, payload, actor, summary, writes, keep); err != nil {
		// The commit may have succeeded even though the reply failed. The stored payload
		// carries a random nonce, so an exact match proves that this save committed.
		if state, read := s.refresh(ctx); read && state.revision == next && bytes.Equal(state.payload, payload) {
			return next, nil
		}
		return 0, fmt.Errorf("save configuration: %w", err)
	}
	s.mu.Lock()
	s.configuration = configuration
	s.credentials = states
	s.revision = next
	s.mu.Unlock()
	s.notify()
	return next, nil
}

func (s *Service) notify() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

func (s *Service) encrypt(configuration config.Config, revision int64) ([]byte, error) {
	return s.seal(configuration, configurationData(revision))
}

func (s *Service) decrypt(payload []byte, revision int64) (config.Config, error) {
	var stored struct {
		config.Config
		AccessIssuer json.RawMessage `json:"accessIssuer"`
	}
	if err := s.unseal(payload, configurationData(revision), &stored); err != nil {
		return config.Config{}, err
	}
	// Before issuer binding, stored access belonged to the stored provider. Interpret
	// only an absent field as that legacy format. An empty or mismatched declared
	// binding still fails validation. The next settings save persists the binding.
	if stored.AccessIssuer == nil {
		if stored.Identity != nil {
			stored.Config.AccessIssuer = stored.Identity.Issuer
		}
	} else if err := json.Unmarshal(stored.AccessIssuer, &stored.Config.AccessIssuer); err != nil {
		return config.Config{}, err
	}
	return stored.Config, nil
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
