package store

import (
	"context"

	"github.com/kubebao/openbao-operator-console/internal/settings"
	"github.com/kubebao/openbao-operator-console/internal/store/sqlitedb"
)

func (p *SQLite) LoadCredentials(ctx context.Context) ([]settings.CredentialRecord, error) {
	rows, err := sqlitedb.New(p.db).ListReaderCredentials(ctx)
	if err != nil {
		return nil, err
	}
	result := []settings.CredentialRecord{}
	for _, row := range rows {
		result = append(result, settings.CredentialRecord{ConnectionID: row.ConnectionID, Revision: row.Revision, Payload: row.Ciphertext})
	}
	return result, nil
}

func writeSQLiteCredential(ctx context.Context, q *sqlitedb.Queries, record settings.CredentialRecord) error {
	var count int64
	var err error
	if record.Revision == 1 {
		count, err = q.CreateReaderCredential(
			ctx,
			sqlitedb.CreateReaderCredentialParams{ConnectionID: record.ConnectionID, Ciphertext: record.Payload},
		)
	} else {
		count, err = q.UpdateReaderCredential(
			ctx,
			sqlitedb.UpdateReaderCredentialParams{
				ConnectionID: record.ConnectionID,
				Revision:     record.Revision - 1,
				Ciphertext:   record.Payload,
			},
		)
	}
	if err != nil {
		return err
	}
	if count != 1 {
		return settings.ErrConflict
	}
	return nil
}

func (p *SQLite) SaveCredential(ctx context.Context, configurationRevision int64, record settings.CredentialRecord) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	q := sqlitedb.New(tx)
	revision, err := q.LockSettingsRevision(ctx)
	if err != nil {
		return err
	}
	if revision != configurationRevision {
		return settings.ErrConflict
	}
	if err := writeSQLiteCredential(ctx, q, record); err != nil {
		return err
	}
	return tx.Commit()
}
