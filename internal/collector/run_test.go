package collector

import (
	"bytes"
	"context"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/settings"
	"github.com/kubebao/openbao-operator-console/internal/store"
)

// TestSlowConnectionDoesNotDelayAnotherConnection blocks every read of one connection
// until shutdown. The other connection keeps its own cadence, and shutdown waits for both.
func TestSlowConnectionDoesNotDelayAnotherConnection(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/namespaces/slow/") {
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		resource := "openbaoclusters"
		if strings.HasSuffix(r.URL.Path, "openbaorestores") {
			resource = "openbaorestores"
		}
		writeList(w, resource, []any{}, "")
	}))
	defer server.Close()
	credential := &config.Credential{
		Server: server.URL,
		Token:  "reader",
		CA:     string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})),
	}
	connection := func(id string) config.Connection {
		return config.Connection{
			ID:         id,
			Name:       id,
			Namespaces: []config.Namespace{{Name: id, Environment: "development"}},
			Access:     config.Access{Mode: config.AccessRemote, Credential: credential},
		}
	}
	service, err := settings.Open(
		context.Background(),
		&managedConfiguration{},
		bytes.Repeat([]byte{1}, 32),
		&config.Config{
			MaxBackupAgeHours: 24,
			Users:             []config.User{{Subject: "admin", Admin: true}},
			Connections:       []config.Connection{connection("slow"), connection("fast")},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	repository := &batchRecorder{batches: make(chan store.Batch, 20)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runManaged(ctx, service, repository, &schedule{
			interval: 20 * time.Millisecond,
			slots:    make(chan struct{}, 4),
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		})
	}()
	timeout := time.After(5 * time.Second)
	for collected := 0; collected < 5; {
		select {
		case batch := <-repository.batches:
			if batch.Connection.ID != "fast" {
				t.Fatalf("blocked connection %s persisted a batch", batch.Connection.ID)
			}
			collected++
		case <-timeout:
			t.Fatalf("the fast connection completed %d collections while another connection was blocked", collected)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("collection did not stop on shutdown")
	}
}

// TestConcurrentCollectionsAreBounded holds every collection open and counts how many
// connections read at the same time.
func TestConcurrentCollectionsAreBounded(t *testing.T) {
	started := make(chan string, 20)
	server := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		started <- r.URL.Path
		<-r.Context().Done()
	}))
	defer server.Close()
	credential := &config.Credential{
		Server: server.URL,
		Token:  "reader",
		CA:     string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})),
	}
	connections := []config.Connection{}
	for _, id := range []string{"a", "b", "c", "d"} {
		connections = append(connections, config.Connection{
			ID:         id,
			Name:       id,
			Namespaces: []config.Namespace{{Name: id, Environment: "development"}},
			Access:     config.Access{Mode: config.AccessRemote, Credential: credential},
		})
	}
	service, err := settings.Open(
		context.Background(),
		&managedConfiguration{},
		bytes.Repeat([]byte{1}, 32),
		&config.Config{MaxBackupAgeHours: 24, Users: []config.User{{Subject: "admin", Admin: true}}, Connections: connections},
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runManaged(ctx, service, &batchRecorder{batches: make(chan store.Batch, 20)}, &schedule{
			interval: time.Hour,
			slots:    make(chan struct{}, 2),
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		})
	}()
	for range 2 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("collections did not start")
		}
	}
	select {
	case path := <-started:
		t.Fatalf("a third collection started while two were in flight: %s", path)
	case <-time.After(200 * time.Millisecond):
	}
	cancel()
	<-done
}
