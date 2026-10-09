package settings

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

type memoryRepository struct {
	credentials map[string]CredentialRecord
	mu          sync.Mutex
	record      Record
	changes     []Change
}

func TestPendingInitializationIsEncryptedSingleUseAndNeverReopens(t *testing.T) {
	ctx := context.Background()
	repository := &memoryRepository{}
	key := bytes.Repeat([]byte{3}, 32)
	if _, err := Open(ctx, repository, key[:31], nil); err == nil {
		t.Fatal("pending configuration accepted an invalid encryption key")
	}
	first, err := Open(ctx, repository, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Open(ctx, repository, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Initialized() || len(repository.changes) != 0 {
		t.Fatal("opening pending configuration wrote a seed")
	}
	results := make(chan error, 2)
	var callers sync.WaitGroup
	for _, service := range []*Service{first, second} {
		callers.Add(1)
		go func() {
			defer callers.Done()
			results <- service.Initialize(ctx, seed(), "verified-admin")
		}()
	}
	callers.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 || repository.record.Revision != 1 || len(repository.changes) != 1 || bytes.Contains(repository.record.Payload, []byte("credential-never-plaintext")) {
		t.Fatal("competing initialization overwrote or exposed configuration")
	}
	restarted, err := Open(ctx, repository, key, nil)
	if err != nil || !restarted.Initialized() {
		t.Fatal("stored configuration reopened setup")
	}
	if err := restarted.Initialize(ctx, seed(), "second-admin"); !errors.Is(err, ErrConflict) {
		t.Fatal("initialized database accepted another first claim")
	}
}

func (r *memoryRepository) LoadSettings(context.Context) (Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.record.Revision == 0 {
		return Record{}, ErrMissing
	}
	return r.record, nil
}

func (r *memoryRepository) SaveSettings(_ context.Context, revision int64, payload []byte, actor, summary string, writes []CredentialRecord, keep []string) (Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if revision != r.record.Revision {
		return Record{}, ErrConflict
	}
	if r.credentials == nil {
		r.credentials = map[string]CredentialRecord{}
	}
	for _, row := range writes {
		if row.Revision != r.credentials[row.ConnectionID].Revision+1 {
			return Record{}, ErrConflict
		}
	}
	for _, row := range writes {
		r.credentials[row.ConnectionID] = row
	}
	retained := map[string]bool{}
	for _, id := range keep {
		retained[id] = true
	}
	for id := range r.credentials {
		if !retained[id] {
			delete(r.credentials, id)
		}
	}

	r.record = Record{Revision: revision + 1, Payload: append([]byte{}, payload...)}
	r.changes = append(r.changes, Change{Revision: revision + 1, Actor: actor, Summary: summary, At: time.Now()})
	return r.record, nil
}

func (r *memoryRepository) SettingsHistory(context.Context) ([]Change, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Change{}, r.changes...), nil
}

func seed() config.Config {
	return config.Config{MaxBackupAgeHours: 24, Users: []config.User{{Subject: "admin", Admin: true}, {Subject: "reader", Scopes: []config.Scope{{ConnectionID: "cluster", Namespaces: []string{"team"}}}}}, Connections: []config.Connection{{ID: "cluster", Name: "Cluster", Namespaces: []config.Namespace{{Name: "team", Environment: "production"}}, Credential: &config.Credential{Server: "https://api.example", Token: "credential-never-plaintext"}}}}
}

func TestEncryptionRestartConflictAndRevocation(t *testing.T) {
	ctx := context.Background()
	repository := &memoryRepository{}
	key := bytes.Repeat([]byte{7}, 32)
	service, err := New(ctx, repository, key, seed())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(repository.record.Payload, []byte("credential-never-plaintext")) {
		t.Fatal("credential persisted in plaintext")
	}
	restarted, err := New(ctx, repository, key, config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if user, ok := restarted.Resolve(ctx, "reader", nil); !ok || len(user.Scopes) != 1 {
		t.Fatal("configuration lost on restart")
	}
	if _, err := New(ctx, repository, bytes.Repeat([]byte{8}, 32), seed()); err == nil {
		t.Fatal("wrong encryption key accepted")
	}
	current, revision := service.Current()
	current.Users = current.Users[:1]
	if _, err := service.Save(ctx, revision, current, "admin", "Access updated"); err != nil {
		t.Fatal(err)
	}
	if _, ok := service.Resolve(ctx, "reader", nil); ok {
		t.Fatal("removed user still enrolled")
	}
	if _, err := service.Save(ctx, revision, current, "admin", "stale"); err != ErrConflict {
		t.Fatal("stale revision overwrote settings")
	}
	writes := 0
	if err := service.Apply(ctx, revision, func(context.Context) error { writes++; return nil }); err != ErrConflict || writes != 0 {
		t.Fatal("superseded reader persisted a result")
	}
	current, revision = service.Current()
	current.Users = nil
	if _, err := service.Save(ctx, revision, current, "admin", "Remove admin"); err == nil {
		t.Fatal("last administrator removed")
	}
}

func TestCurrentReturnsAnIndependentConfiguration(t *testing.T) {
	service, err := New(context.Background(), &memoryRepository{}, bytes.Repeat([]byte{1}, 32), seed())
	if err != nil {
		t.Fatal(err)
	}
	current, _ := service.Current()
	current.Connections[0].Credential.Token = "changed"
	current.Connections[0].Namespaces[0].Name = "other"
	actual, _ := service.Current()
	if actual.Connections[0].Credential.Token == "changed" || actual.Connections[0].Namespaces[0].Name == "other" {
		t.Fatal("caller changed live configuration")
	}
}

func (r *memoryRepository) LoadCredentials(context.Context) ([]CredentialRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := []CredentialRecord{}
	for _, row := range r.credentials {
		rows = append(rows, row)
	}
	return rows, nil
}

func (r *memoryRepository) SaveCredential(_ context.Context, revision int64, row CredentialRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if revision != r.record.Revision || row.Revision != r.credentials[row.ConnectionID].Revision+1 {
		return ErrConflict
	}
	r.credentials[row.ConnectionID] = row
	return nil
}
