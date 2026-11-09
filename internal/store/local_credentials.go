package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/kubebao/openbao-operator-console/internal/localauth"
	"github.com/kubebao/openbao-operator-console/internal/store/database"
)

func (p *Postgres) GetLocalCredential(ctx context.Context, accountID string) (localauth.Record, error) {
	row, err := database.New(p.pool).GetLocalCredential(ctx, accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return localauth.Record{}, localauth.ErrNotFound
	}
	if err != nil {
		return localauth.Record{}, err
	}
	return localauth.Record{AccountID: row.AccountID, Revision: row.Revision, Payload: row.Ciphertext}, nil
}

func (p *Postgres) CreateLocalCredential(ctx context.Context, record localauth.Record) error {
	if record.Revision != 1 {
		return localauth.ErrConflict
	}
	count, err := database.New(p.pool).CreateLocalCredential(
		ctx,
		database.CreateLocalCredentialParams{AccountID: record.AccountID, Ciphertext: record.Payload},
	)
	if err != nil {
		return err
	}
	if count != 1 {
		return localauth.ErrConflict
	}
	return nil
}

func (p *Postgres) UpdateLocalCredential(ctx context.Context, record localauth.Record) error {
	count, err := database.New(p.pool).UpdateLocalCredential(ctx, database.UpdateLocalCredentialParams{
		Ciphertext: record.Payload,
		AccountID:  record.AccountID,
		Revision:   record.Revision - 1,
	})
	if err != nil {
		return err
	}
	if count != 1 {
		return localauth.ErrConflict
	}
	return nil
}

func (p *Postgres) DeleteLocalCredentials(ctx context.Context, keep []string) error {
	return database.New(p.pool).DeleteRemovedLocalCredentials(ctx, keep)
}
