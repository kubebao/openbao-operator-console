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

func TestCiphertextsAreBoundToTheirRevision(t *testing.T) {
	ctx := context.Background()
	repository := &memoryRepository{}
	key := bytes.Repeat([]byte{6}, 32)
	service, err := Open(ctx, repository, key, new(renewableSeed()))
	if err != nil {
		t.Fatal(err)
	}
	first := repository.record
	if _, err := service.decrypt(first.Payload, first.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := service.decrypt(first.Payload, first.Revision+1); err == nil {
		t.Fatal("configuration ciphertext opened as a later revision")
	}
	row := repository.credentials["cluster"]
	row.Revision++
	if _, err := service.decryptCredential(row); err == nil {
		t.Fatal("credential ciphertext opened as a later revision")
	}
	current, revision := service.Current()
	current.MaxBackupAgeHours = 12
	if _, err := service.Save(ctx, revision, current, "admin", "Policies updated"); err != nil {
		t.Fatal(err)
	}
	repository.record.Payload = first.Payload
	if _, err := Open(ctx, repository, key, nil); err == nil {
		t.Fatal("an older configuration ciphertext was replayed as the current revision")
	}
}

// unreliableRepository commits writes but can report a failure afterward, as a lost reply
// from the database would. It can also fail reads.
type unreliableRepository struct {
	*memoryRepository
	lostReplies int
	failedReads int
}

var errLostReply = errors.New("connection reset")

func (r *unreliableRepository) SaveSettings(
	ctx context.Context,
	revision int64,
	payload []byte,
	actor, summary string,
	writes []CredentialRecord,
	keep []string,
) (Record, error) {
	record, err := r.memoryRepository.SaveSettings(ctx, revision, payload, actor, summary, writes, keep)
	if err == nil && r.lostReplies > 0 {
		r.lostReplies--
		return Record{}, errLostReply
	}
	return record, err
}

func (r *unreliableRepository) SaveCredential(ctx context.Context, revision int64, row CredentialRecord) error {
	err := r.memoryRepository.SaveCredential(ctx, revision, row)
	if err == nil && r.lostReplies > 0 {
		r.lostReplies--
		return errLostReply
	}
	return err
}

func (r *unreliableRepository) LoadSettings(ctx context.Context) (Record, error) {
	if r.failedReads > 0 {
		r.failedReads--
		return Record{}, errors.New("database unavailable")
	}
	return r.memoryRepository.LoadSettings(ctx)
}

func TestAmbiguousCommitReloadsStoredState(t *testing.T) {
	ctx := context.Background()
	repository := &unreliableRepository{memoryRepository: &memoryRepository{}}
	service, err := Open(ctx, repository, bytes.Repeat([]byte{2}, 32), new(renewableSeed()))
	if err != nil {
		t.Fatal(err)
	}
	t.Run("lost reply after a commit reports the commit", func(t *testing.T) {
		current, revision := service.Current()
		current.MaxBackupAgeHours = 12
		repository.lostReplies = 1
		saved, err := service.Save(ctx, revision, current, "admin", "Policies updated")
		if err != nil || saved != revision+1 {
			t.Fatalf("committed save = %d, %v", saved, err)
		}
		if got, latest := service.Current(); latest != revision+1 || got.MaxBackupAgeHours != 12 {
			t.Fatal("committed save was not published")
		}
	})
	t.Run("unreadable outcome recovers on the next save", func(t *testing.T) {
		current, revision := service.Current()
		current.MaxBackupAgeHours = 6
		repository.lostReplies, repository.failedReads = 1, 1
		if _, err := service.Save(ctx, revision, current, "admin", "Policies updated"); !errors.Is(err, errLostReply) {
			t.Fatalf("unknown commit outcome = %v", err)
		}
		if _, err := service.Save(ctx, revision, current, "admin", "Retry"); !errors.Is(err, ErrConflict) {
			t.Fatalf("stale retry = %v, want a conflict", err)
		}
		reloaded, latest := service.Current()
		if latest != revision+1 || reloaded.MaxBackupAgeHours != 6 {
			t.Fatal("conflict did not reload the stored configuration")
		}
		reloaded.MaxBackupAgeHours = 24
		if _, err := service.Save(ctx, latest, reloaded, "admin", "Policies updated"); err != nil {
			t.Fatal("settings stayed in conflict after reload")
		}
	})
	t.Run("lost credential reply reloads the renewed reader", func(t *testing.T) {
		_, _, generation := service.ReaderConfiguration()
		target := service.CredentialTargets()[0]
		state := renewedState(target)
		repository.lostReplies = 1
		if err := service.UpdateCredential(ctx, target, state); !errors.Is(err, errLostReply) {
			t.Fatalf("unknown credential outcome = %v", err)
		}
		got := service.CredentialTargets()[0]
		if got.CredentialRevision != target.CredentialRevision+1 || got.State.Credential.Token != state.Credential.Token {
			t.Fatal("committed renewal was not reloaded")
		}
		if _, _, current := service.ReaderConfiguration(); current == generation {
			t.Fatal("reloaded reader did not start a new collection generation")
		}
		if err := service.UpdateCredential(ctx, got, renewedState(got)); err != nil {
			t.Fatal("renewal stayed in conflict after reload")
		}
	})
}

// blockingRepository holds settings commits until released.
type blockingRepository struct {
	*memoryRepository
	entered chan struct{}
	release chan struct{}
}

func (r *blockingRepository) SaveSettings(
	ctx context.Context,
	revision int64,
	payload []byte,
	actor, summary string,
	writes []CredentialRecord,
	keep []string,
) (Record, error) {
	if revision > 0 {
		r.entered <- struct{}{}
		<-r.release
	}
	return r.memoryRepository.SaveSettings(ctx, revision, payload, actor, summary, writes, keep)
}

// readsComplete fails the test when settings reads wait for a commit or collection write.
func readsComplete(t *testing.T, service *Service) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.Current()
		service.Resolve(context.Background(), config.Principal{Issuer: testIssuer, Subject: "admin"})
		service.Initialized()
		service.CredentialTargets()
		service.ReaderConfiguration()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("settings reads waited for database I/O")
	}
}

func TestReadsDoNotWaitForDatabaseWrites(t *testing.T) {
	ctx := context.Background()
	repository := &blockingRepository{
		memoryRepository: &memoryRepository{},
		entered:          make(chan struct{}),
		release:          make(chan struct{}),
	}
	service, err := Open(ctx, repository, bytes.Repeat([]byte{3}, 32), new(seed()))
	if err != nil {
		t.Fatal(err)
	}
	var writers sync.WaitGroup
	current, revision := service.Current()
	writers.Go(func() {
		if _, err := service.Save(ctx, revision, current, "admin", "Saved"); err != nil {
			t.Error(err)
		}
	})
	<-repository.entered
	readsComplete(t, service)
	close(repository.release)
	writers.Wait()

	// A collection write holds the commit lock shared. A waiting commit must not hold
	// back readers behind it.
	current, revision = service.Current()
	_, _, generation := service.ReaderConfiguration()
	writing, finish := make(chan struct{}), make(chan struct{})
	writers.Go(func() {
		_ = service.ApplyCollection(ctx, revision, generation, func(context.Context) error {
			close(writing)
			<-finish
			return nil
		})
	})
	<-writing
	repository.release = make(chan struct{})
	writers.Go(func() {
		if _, err := service.Save(ctx, revision, current, "admin", "Saved"); err != nil {
			t.Error(err)
		}
	})
	time.Sleep(50 * time.Millisecond) // let the commit queue behind the collection write
	readsComplete(t, service)
	close(finish)
	<-repository.entered
	close(repository.release)
	writers.Wait()
}
