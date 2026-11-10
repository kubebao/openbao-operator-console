package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kubebao/openbao-operator-console/internal/localauth"
	"github.com/kubebao/openbao-operator-console/internal/store/sqlitedb"
)

func (p *SQLite) GetLocalCredential(ctx context.Context, accountID string) (localauth.Record, error) {
	row, err := sqlitedb.New(p.db).GetLocalCredential(ctx, accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return localauth.Record{}, localauth.ErrNotFound
	}
	if err != nil {
		return localauth.Record{}, err
	}
	return localauth.Record{AccountID: row.AccountID, Revision: row.Revision, Payload: row.Ciphertext}, nil
}

func (p *SQLite) CreateLocalCredential(ctx context.Context, record localauth.Record) error {
	if record.Revision != 1 {
		return localauth.ErrConflict
	}
	count, err := sqlitedb.New(p.db).CreateLocalCredential(
		ctx,
		sqlitedb.CreateLocalCredentialParams{AccountID: record.AccountID, Ciphertext: record.Payload},
	)
	if err != nil {
		return err
	}
	if count != 1 {
		return localauth.ErrConflict
	}
	return nil
}

func (p *SQLite) UpdateLocalCredential(ctx context.Context, record localauth.Record) error {
	count, err := sqlitedb.New(p.db).UpdateLocalCredential(ctx, sqlitedb.UpdateLocalCredentialParams{
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

func (p *SQLite) DeleteLocalCredentials(ctx context.Context, keep []string) error {
	return sqlitedb.New(p.db).DeleteRemovedLocalCredentials(ctx, stringList(keep))
}
