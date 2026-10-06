package collector

import (
	"context"
	"log/slog"
	"reflect"
	"time"

	"github.com/dc-tec/openbao-operator-console/internal/evidence"
	"github.com/dc-tec/openbao-operator-console/internal/settings"
	"github.com/dc-tec/openbao-operator-console/internal/store"
)

type revisionRepository struct {
	store.Repository
	settings   *settings.Service
	revision   int64
	generation uint64
}

func (r revisionRepository) Apply(ctx context.Context, batch store.Batch) error {
	return r.settings.ApplyCollection(ctx, r.revision, r.generation, func(ctx context.Context) error { return r.Repository.Apply(ctx, batch) })
}

func RunManaged(ctx context.Context, configuration *settings.Service, repository store.Repository, logger *slog.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	previous := map[string]*Reader{}
	for {
		current, revision, generation := configuration.ReaderConfiguration()
		policy := evidence.Policy{MaxBackupAge: time.Duration(current.MaxBackupAgeHours) * time.Hour, StartupGrace: time.Duration(current.StartupGraceMinutes) * time.Minute}
		readers := []*Reader{}
		next := map[string]*Reader{}
		for _, connection := range current.Connections {
			if connection.Disabled {
				continue
			}
			reader := previous[connection.ID]
			if reader == nil || !reflect.DeepEqual(reader.connection, connection) || reader.policy != policy {
				reader = Unconfigured(connection, policy)
			}
			readers = append(readers, reader)
			next[connection.ID] = reader
		}
		previous = next
		collectRound(ctx, readers, revisionRepository{Repository: repository, settings: configuration, revision: revision, generation: generation}, logger)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-configuration.Changed():
		}
	}
}
