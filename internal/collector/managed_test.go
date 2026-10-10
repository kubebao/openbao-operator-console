package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/settings"
	"github.com/kubebao/openbao-operator-console/internal/store"
)

type managedConfiguration struct {
	credentials map[string]settings.CredentialRecord
	mu          sync.Mutex
	record      settings.Record
}

func (r *managedConfiguration) LoadSettings(context.Context) (settings.Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.record.Revision == 0 {
		return settings.Record{}, settings.ErrMissing
	}
	return r.record, nil
}

func (r *managedConfiguration) SaveSettings(
	_ context.Context,
	revision int64,
	payload []byte,
	_, _ string,
	writes []settings.CredentialRecord,
	keep []string,
) (settings.Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if revision != r.record.Revision {
		return settings.Record{}, settings.ErrConflict
	}
	if r.credentials == nil {
		r.credentials = map[string]settings.CredentialRecord{}
	}
	for _, row := range writes {
		if row.Revision != r.credentials[row.ConnectionID].Revision+1 {
			return settings.Record{}, settings.ErrConflict
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

	r.record = settings.Record{Revision: revision + 1, Payload: payload}
	return r.record, nil
}

func (*managedConfiguration) SettingsHistory(context.Context) ([]settings.Change, error) {
	return nil, nil
}

type batchRecorder struct{ batches chan store.Batch }

func (r *batchRecorder) Apply(_ context.Context, batch store.Batch) error {
	r.batches <- batch
	return nil
}

func TestManagedCollectionAddsPausesAndReconfiguresReaders(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer reader" {
			t.Error("reader made an unexpected request")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"apiVersion": "openbao.org/v1alpha1",
			"kind":       "List",
			"items":      []any{},
			"metadata":   map[string]any{},
		})
	}))
	defer server.Close()
	credential := &config.Credential{
		Server: server.URL,
		Token:  "reader",
		CA:     string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})),
	}
	one := config.Connection{
		ID:         "one",
		Name:       "One",
		Namespaces: []config.Namespace{{Name: "before", Environment: "development"}},
		Access:     config.Access{Mode: config.AccessRemote, Credential: credential},
	}
	service, err := settings.New(
		context.Background(),
		&managedConfiguration{},
		bytes.Repeat([]byte{1}, 32),
		config.Config{MaxBackupAgeHours: 24, Users: []config.User{{Subject: "admin", Admin: true}}, Connections: []config.Connection{one}},
	)
	if err != nil {
		t.Fatal(err)
	}
	repository := &batchRecorder{batches: make(chan store.Batch, 20)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunManaged(ctx, service, repository, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("collector did not stop")
		}
	}()
	await := func() store.Batch {
		t.Helper()
		select {
		case batch := <-repository.batches:
			return batch
		case <-time.After(3 * time.Second):
			t.Fatal("configuration change did not trigger collection")
			return store.Batch{}
		}
	}
	if batch := await(); batch.Connection.ID != "one" || batch.Connection.Collection != "current" {
		t.Fatal("initial reader failed")
	}
	current, revision := service.Current()
	current.Connections[0].Disabled = true
	two := one
	two.ID = "two"
	two.Name = "Two"
	current.Connections = append(current.Connections, two)
	if _, err := service.Save(ctx, revision, current, "admin", "Add reader"); err != nil {
		t.Fatal(err)
	}
	if batch := await(); batch.Connection.ID != "two" {
		t.Fatal("paused connection was collected")
	}
	current, revision = service.Current()
	current.Connections[1].Namespaces[0].Name = "after"
	if _, err := service.Save(ctx, revision, current, "admin", "Change namespace"); err != nil {
		t.Fatal(err)
	}
	if batch := await(); batch.Connection.ID != "two" || len(batch.Connection.Namespaces) != 1 ||
		batch.Connection.Namespaces[0] != "after" {
		t.Fatal("reader reused its old namespace")
	}
}

func (r *managedConfiguration) LoadCredentials(context.Context) ([]settings.CredentialRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := []settings.CredentialRecord{}
	for _, row := range r.credentials {
		rows = append(rows, row)
	}
	return rows, nil
}

func (r *managedConfiguration) SaveCredential(_ context.Context, revision int64, row settings.CredentialRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if revision != r.record.Revision || row.Revision != r.credentials[row.ConnectionID].Revision+1 {
		return settings.ErrConflict
	}
	r.credentials[row.ConnectionID] = row
	return nil
}
