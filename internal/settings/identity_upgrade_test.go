package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

func TestStoredAccessBeforeIssuerBindingSurvivesUpgrade(t *testing.T) {
	ctx := context.Background()
	repository := &memoryRepository{}
	key := bytes.Repeat([]byte{3}, 32)
	service, err := Open(ctx, repository, key, new(seed()))
	if err != nil {
		t.Fatal(err)
	}
	legacy, revision := service.Current()
	legacy.AccessIssuer = ""
	payload, err := service.encrypt(legacy, revision)
	if err != nil {
		t.Fatal(err)
	}
	repository.record.Payload = payload
	before := len(repository.changes)
	restarted, err := Open(ctx, repository, key, nil)
	if err != nil {
		t.Fatalf("legacy configuration cannot restart: %v", err)
	}
	current, gotRevision := restarted.Current()
	if current.AccessIssuer != testIssuer || gotRevision != revision || len(repository.changes) != before ||
		!bytes.Equal(repository.record.Payload, payload) {
		t.Fatal("loading legacy access changed the stored revision, ciphertext, or audit")
	}
	if _, allowed := restarted.Resolve(ctx, reader()); !allowed {
		t.Fatal("upgrade lost the stored provider's access")
	}
	if _, allowed := restarted.Resolve(ctx, config.Principal{Issuer: "https://other.example", Subject: "admin"}); allowed {
		t.Fatal("upgrade granted access to another issuer")
	}
	if current.Connections[0].Access.Credential.Token != "credential-never-plaintext" {
		t.Fatal("upgrade lost the reader credential")
	}
	if _, err := restarted.Save(ctx, revision, current, "admin", "Save upgraded access"); err != nil {
		t.Fatal(err)
	}
	var persisted struct {
		AccessIssuer string `json:"accessIssuer"`
	}
	if err := restarted.unseal(repository.record.Payload, configurationData(revision+1), &persisted); err != nil ||
		persisted.AccessIssuer != testIssuer {
		t.Fatal("the next settings save did not persist the issuer binding")
	}
	if _, err := Open(ctx, repository, key, nil); err != nil {
		t.Fatalf("upgraded configuration cannot restart: %v", err)
	}
}

func TestStoredDeclaredIssuerBindingIsNeverInferred(t *testing.T) {
	for _, binding := range []json.RawMessage{json.RawMessage(`""`), json.RawMessage(`null`), json.RawMessage(`"https://other.example"`)} {
		t.Run(string(binding), func(t *testing.T) {
			ctx := context.Background()
			repository := &memoryRepository{}
			key := bytes.Repeat([]byte{4}, 32)
			service, err := Open(ctx, repository, key, new(seed()))
			if err != nil {
				t.Fatal(err)
			}
			current, revision := service.Current()
			payload, err := service.seal(struct {
				config.Config
				AccessIssuer json.RawMessage `json:"accessIssuer"`
			}{current, binding}, configurationData(revision))
			if err != nil {
				t.Fatal(err)
			}
			repository.record.Payload = payload
			if _, err := Open(ctx, repository, key, nil); err == nil {
				t.Fatal("an invalid declared issuer binding was inferred from the provider")
			}
		})
	}
}
