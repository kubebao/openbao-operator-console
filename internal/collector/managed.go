package collector

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
	"github.com/kubebao/openbao-operator-console/internal/settings"
	"github.com/kubebao/openbao-operator-console/internal/store"
)

const (
	// collectInterval is the cadence of each connection. view.StaleAfter allows several
	// missed collections before evidence becomes stale.
	collectInterval = 30 * time.Second
	collectTimeout  = 30 * time.Second
	writeTimeout    = 10 * time.Second
	// maxCollections bounds concurrent collections across all connections. Slow
	// connections delay others only when this many are slow at the same time.
	maxCollections = 16
)

// revisionSink drops a batch when the configuration that produced it was superseded.
type revisionSink struct {
	sink       Sink
	settings   *settings.Service
	revision   int64
	generation uint64
}

func (r revisionSink) Apply(ctx context.Context, batch store.Batch) error {
	return r.settings.ApplyCollection(
		ctx,
		r.revision,
		r.generation,
		func(ctx context.Context) error { return r.sink.Apply(ctx, batch) },
	)
}

func (r revisionSink) RetainConnections(ctx context.Context, enrolled []string) error {
	return r.settings.ApplyCollection(
		ctx,
		r.revision,
		r.generation,
		func(ctx context.Context) error { return r.sink.RetainConnections(ctx, enrolled) },
	)
}

// retainEnrolled deletes stored observations of connections that are no longer enrolled.
// Paused connections stay enrolled and keep their last-known inventory. A superseded
// configuration removes nothing; the next configuration retries.
func retainEnrolled(ctx context.Context, current config.Config, sink Sink, logger *slog.Logger) {
	enrolled := make([]string, 0, len(current.Connections))
	for _, connection := range current.Connections {
		enrolled = append(enrolled, connection.ID)
	}
	writeCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	if err := sink.RetainConnections(writeCtx, enrolled); err != nil && !errors.Is(err, settings.ErrConflict) && ctx.Err() == nil {
		logger.Error("Remove observations of removed connections failed")
	}
}

// schedule holds the bounds shared by every connection's collection loop.
type schedule struct {
	interval time.Duration
	slots    chan struct{}
	logger   *slog.Logger
	running  sync.WaitGroup
}

// worker collects one connection on its own cadence, so a slow connection never delays
// the next collection of another one. It has at most one collection in flight.
type worker struct {
	connection config.Connection
	policy     fleet.Policy
	// sink is the revision guard of the newest configuration that still matches this
	// worker's connection and policy.
	sink   atomic.Pointer[Sink]
	cancel context.CancelFunc
}

func (s *schedule) start(ctx context.Context, connection config.Connection, policy fleet.Policy, sink Sink) *worker {
	ctx, cancel := context.WithCancel(ctx)
	w := &worker{connection: connection, policy: policy, cancel: cancel}
	w.sink.Store(&sink)
	reader := Unconfigured(connection, policy)
	s.running.Go(func() { s.run(ctx, w, reader) })
	return w
}

// run collects immediately and then once per interval. A collection that overruns the
// interval is followed directly by the next one.
func (s *schedule) run(ctx context.Context, w *worker, reader *Reader) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case s.slots <- struct{}{}:
		case <-ctx.Done():
			return
		}
		// The guard is taken before reading, so a batch read under a superseded
		// configuration is never persisted.
		s.collect(ctx, reader, *w.sink.Load())
		<-s.slots
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *schedule) collect(ctx context.Context, reader *Reader, sink Sink) {
	collectCtx, cancel := context.WithTimeout(ctx, collectTimeout)
	batch := reader.Collect(collectCtx, time.Now().UTC())
	cancel()
	if ctx.Err() != nil {
		return
	}
	writeCtx, cancelWrite := context.WithTimeout(ctx, writeTimeout)
	defer cancelWrite()
	if err := sink.Apply(writeCtx, batch); err != nil && !errors.Is(err, settings.ErrConflict) && ctx.Err() == nil {
		s.logger.Error("Persist collection failed", "connection", reader.connection.ID, "cause", store.FailureCause(err))
	}
}

// reconcile keeps workers whose connection and policy are unchanged and moves them to the
// new revision guard. It replaces changed workers and stops removed or paused ones.
func (s *schedule) reconcile(ctx context.Context, workers map[string]*worker, current config.Config, sink Sink) map[string]*worker {
	policy := current.Policy()
	next := map[string]*worker{}
	for _, connection := range current.Connections {
		if connection.Disabled {
			continue
		}
		existing := workers[connection.ID]
		if existing != nil && reflect.DeepEqual(existing.connection, connection) && existing.policy == policy {
			existing.sink.Store(&sink)
			next[connection.ID] = existing
			delete(workers, connection.ID)
			continue
		}
		next[connection.ID] = s.start(ctx, connection, policy, sink)
	}
	for _, stale := range workers {
		stale.cancel()
	}
	return next
}

// RunManaged collects every enabled connection on its own schedule and follows
// configuration changes without a restart. It returns after every collection stopped.
func RunManaged(ctx context.Context, configuration *settings.Service, sink Sink, logger *slog.Logger) {
	runManaged(ctx, configuration, sink, &schedule{
		interval: collectInterval,
		slots:    make(chan struct{}, maxCollections),
		logger:   logger,
	})
}

func runManaged(ctx context.Context, configuration *settings.Service, sink Sink, s *schedule) {
	workers := map[string]*worker{}
	defer s.running.Wait()
	defer func() {
		for _, w := range workers {
			w.cancel()
		}
	}()
	// The ticker only retries removal after a failed write; collection has its own cadence.
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		current, revision, generation := configuration.ReaderConfiguration()
		guard := revisionSink{sink: sink, settings: configuration, revision: revision, generation: generation}
		retainEnrolled(ctx, current, guard, s.logger)
		workers = s.reconcile(ctx, workers, current, guard)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-configuration.Changed():
		}
	}
}
