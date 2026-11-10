package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/settings"
	"github.com/kubebao/openbao-operator-console/internal/store/sqlitedb"
)

func (p *SQLite) LoadSettings(ctx context.Context) (settings.Record, error) {
	row, err := sqlitedb.New(p.db).GetSettings(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return settings.Record{}, settings.ErrMissing
	}
	return settings.Record{Revision: row.Revision, Payload: row.Ciphertext}, err
}

func (p *SQLite) SaveSettings(
	ctx context.Context,
	revision int64,
	payload []byte,
	actor, summary string,
	credentials []settings.CredentialRecord,
	keep []string,
) (settings.Record, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return settings.Record{}, err
	}
	defer func() { _ = tx.Rollback() }()
	q := sqlitedb.New(tx)
	var affected int64
	if revision == 0 {
		affected, err = q.CreateSettings(ctx, payload)
	} else {
		affected, err = q.UpdateSettings(ctx, sqlitedb.UpdateSettingsParams{Ciphertext: payload, Revision: revision})
	}
	if err != nil {
		return settings.Record{}, err
	}
	if affected != 1 {
		return settings.Record{}, settings.ErrConflict
	}
	for _, credential := range credentials {
		if err := writeSQLiteCredential(ctx, q, credential); err != nil {
			return settings.Record{}, err
		}
	}
	if err := q.DeleteRemovedCredentials(ctx, stringList(keep)); err != nil {
		return settings.Record{}, err
	}
	if err := q.InsertSettingsChange(
		ctx,
		sqlitedb.InsertSettingsChangeParams{Revision: revision + 1, Actor: actor, Summary: summary},
	); err != nil {
		return settings.Record{}, err
	}
	if err := tx.Commit(); err != nil {
		return settings.Record{}, err
	}
	return settings.Record{Revision: revision + 1, Payload: payload}, nil
}

func (p *SQLite) SettingsHistory(ctx context.Context) ([]settings.Change, error) {
	rows, err := sqlitedb.New(p.db).ListSettingsChanges(ctx)
	if err != nil {
		return nil, err
	}
	result := []settings.Change{}
	for _, row := range rows {
		result = append(
			result,
			settings.Change{Revision: row.Revision, Actor: row.Actor, Summary: row.Summary, At: time.UnixMicro(row.RecordedAt).UTC()},
		)
	}
	return result, nil
}
