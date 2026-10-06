package store

import (
	"context"
	"errors"
	"github.com/dc-tec/openbao-operator-console/internal/settings"
	"github.com/dc-tec/openbao-operator-console/internal/store/database"
	"github.com/jackc/pgx/v5"
)

func (p *Postgres) LoadSettings(ctx context.Context) (settings.Record, error) {
	row, err := database.New(p.pool).GetSettings(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return settings.Record{}, settings.ErrMissing
	}
	return settings.Record{Revision: row.Revision, Payload: row.Ciphertext}, err
}
func (p *Postgres) SaveSettings(ctx context.Context, revision int64, payload []byte, actor, summary string, credentials []settings.CredentialRecord, keep []string) (settings.Record, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return settings.Record{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	var affected int64
	if revision == 0 {
		affected, err = q.CreateSettings(ctx, payload)
	} else {
		affected, err = q.UpdateSettings(ctx, database.UpdateSettingsParams{Ciphertext: payload, Revision: revision})
	}
	if err != nil {
		return settings.Record{}, err
	}
	if affected != 1 {
		return settings.Record{}, settings.ErrConflict
	}
	for _, credential := range credentials {
		if err := writeCredential(ctx, q, credential); err != nil {
			return settings.Record{}, err
		}
	}
	if err := q.DeleteRemovedCredentials(ctx, keep); err != nil {
		return settings.Record{}, err
	}
	if err := q.InsertSettingsChange(ctx, database.InsertSettingsChangeParams{Revision: revision + 1, Actor: actor, Summary: summary}); err != nil {
		return settings.Record{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return settings.Record{}, err
	}
	return settings.Record{Revision: revision + 1, Payload: payload}, nil
}
func (p *Postgres) SettingsHistory(ctx context.Context) ([]settings.Change, error) {
	rows, err := database.New(p.pool).ListSettingsChanges(ctx)
	if err != nil {
		return nil, err
	}
	result := []settings.Change{}
	for _, row := range rows {
		result = append(result, settings.Change{Revision: row.Revision, Actor: row.Actor, Summary: row.Summary, At: row.RecordedAt.Time})
	}
	return result, nil
}
