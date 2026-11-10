package store

import (
	"context"
	"errors"
	"testing"

	"github.com/kubebao/openbao-operator-console/internal/localauth"
)

func TestLocalCredentialsUseRevisionCompareAndSwap(t *testing.T) {
	storageBackends(t, localCredentialsUseRevisionCompareAndSwap)
}

func localCredentialsUseRevisionCompareAndSwap(t *testing.T) {
	repository := isolatedDatabase(t)()
	defer repository.Close()
	ctx := context.Background()
	credentials, err := localauth.NewStore(repository, testKey)
	if err != nil {
		t.Fatal(err)
	}
	hasher, err := localauth.NewHasher()
	if err != nil {
		t.Fatal(err)
	}
	hash, err := hasher.Hash(ctx, "maple river lantern 42")
	if err != nil {
		t.Fatal(err)
	}
	secret, err := localauth.NewTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	credential := localauth.Credential{PasswordHash: hash, TOTPSecret: secret, RecoveryCodes: []string{}}
	if _, err := credentials.Load(ctx, "account-a"); !errors.Is(err, localauth.ErrNotFound) {
		t.Fatalf("missing credential returned %v", err)
	}
	if err := credentials.Create(ctx, "account-a", credential); err != nil {
		t.Fatal(err)
	}
	if err := credentials.Create(ctx, "account-a", credential); !errors.Is(err, localauth.ErrConflict) {
		t.Fatal("second create did not conflict")
	}
	credential.TOTPLastStep = 42
	if err := credentials.Update(ctx, "account-a", 1, credential); err != nil {
		t.Fatal(err)
	}
	if err := credentials.Update(ctx, "account-a", 1, credential); !errors.Is(err, localauth.ErrConflict) {
		t.Fatal("stale update did not conflict")
	}
	stored, err := credentials.Load(ctx, "account-a")
	if err != nil || stored.Revision != 2 || stored.Credential.TOTPLastStep != 42 {
		t.Fatalf("stored credential: %+v, %v", stored, err)
	}
	if err := credentials.Create(ctx, "account-b", credential); err != nil {
		t.Fatal(err)
	}
	if err := credentials.Prune(ctx, []string{"account-b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := credentials.Load(ctx, "account-a"); !errors.Is(err, localauth.ErrNotFound) {
		t.Fatal("removed account kept its credential")
	}
	if err := credentials.Prune(ctx, []string{}); err != nil {
		t.Fatal(err)
	}
	if _, err := credentials.Load(ctx, "account-b"); !errors.Is(err, localauth.ErrNotFound) {
		t.Fatal("an empty keep list did not remove every credential")
	}
}
