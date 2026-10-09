package store

import (
	"context"

	"github.com/kubebao/openbao-operator-console/internal/settings"
	"github.com/kubebao/openbao-operator-console/internal/store/database"
)

func (p *Postgres) LoadCredentials(ctx context.Context) ([]settings.CredentialRecord, error) {
	rows, err := database.New(p.pool).ListReaderCredentials(ctx)
	if err != nil {
		return nil, err
	}
	result := []settings.CredentialRecord{}
	for _, row := range rows {
		result = append(result, settings.CredentialRecord{ConnectionID: row.ConnectionID, Revision: row.Revision, Payload: row.Ciphertext})
	}
	return result, nil
}

func writeCredential(ctx context.Context, q *database.Queries, record settings.CredentialRecord) error {
	var count int64
	var err error
	if record.Revision == 1 {
		count, err = q.CreateReaderCredential(ctx, database.CreateReaderCredentialParams{ConnectionID: record.ConnectionID, Ciphertext: record.Payload})
	} else {
		count, err = q.UpdateReaderCredential(ctx, database.UpdateReaderCredentialParams{ConnectionID: record.ConnectionID, Revision: record.Revision - 1, Ciphertext: record.Payload})
	}
	if err != nil {
		return err
	}
	if count != 1 {
		return settings.ErrConflict
	}
	return nil
}

func (p *Postgres) SaveCredential(ctx context.Context, configurationRevision int64, record settings.CredentialRecord) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := database.New(tx)
	revision, err := q.LockSettingsRevision(ctx)
	if err != nil {
		return err
	}
	if revision != configurationRevision {
		return settings.ErrConflict
	}
	if err := writeCredential(ctx, q, record); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
