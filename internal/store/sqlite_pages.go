package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
	"github.com/kubebao/openbao-operator-console/internal/store/sqlitedb"
)

// Activity returns one newest-first page of events in the given scopes. The scope applies
// before the limit, so events outside it never displace authorized events.
func (p *SQLite) Activity(ctx context.Context, scopes []config.Scope, page fleet.Page) (fleet.ActivityPage, error) {
	result := fleet.ActivityPage{Events: []fleet.Activity{}}
	limit, err := rowLimit(page)
	if err != nil {
		return result, err
	}
	params := sqlitedb.ListActivityParams{RowLimit: int64(limit)}
	type namespaceScope struct {
		Connection string `json:"connection"`
		Namespace  string `json:"namespace"`
	}
	pairs := []namespaceScope{}
	for _, scope := range scopes {
		for _, namespace := range scope.Namespaces {
			pairs = append(pairs, namespaceScope{scope.ConnectionID, namespace})
		}
	}
	if len(pairs) == 0 {
		return result, nil
	}
	encoded, err := json.Marshal(pairs)
	if err != nil {
		return result, err
	}
	params.Scopes = string(encoded)
	if page.After != nil {
		params.AfterAt = sqliteTimestamp(page.After.RecordedAt)
		params.AfterID = sql.NullString{String: page.After.ID, Valid: true}
	}
	rows, err := sqlitedb.New(p.db).ListActivity(ctx, params)
	if err != nil {
		return result, fmt.Errorf("list activity: %w", err)
	}
	for i, row := range rows {
		if i == page.Limit {
			last := rows[i-1]
			result.Next = &fleet.Cursor{RecordedAt: time.UnixMicro(last.RecordedAt).UTC(), ID: last.ID}
			break
		}
		activity, err := decodeActivity(row.Projection, time.UnixMicro(row.RecordedAt).UTC())
		if err != nil {
			return result, err
		}
		result.Events = append(result.Events, activity)
	}
	return result, nil
}

// History returns one newest-first page of a deployment's observations. The first page of
// a windowed query also carries the newest observation before the window. Both reads share
// one snapshot, so the baseline always precedes the page.
func (p *SQLite) History(ctx context.Context, id string, query fleet.HistoryQuery) (fleet.HistoryPage, error) {
	result := fleet.HistoryPage{Observations: []fleet.Observation{}}
	limit, err := rowLimit(query.Page)
	if err != nil {
		return result, err
	}
	params := sqlitedb.GetHistoryParams{DeploymentID: id, RowLimit: int64(limit)}
	if query.Since != nil {
		params.Since = sqliteTimestamp(*query.Since)
	}
	if after := query.Page.After; after != nil {
		position, err := strconv.ParseInt(after.ID, 10, 64)
		if err != nil {
			return result, ErrInvalidCursor
		}
		params.AfterAt = sqliteTimestamp(after.RecordedAt)
		params.AfterID = sql.NullInt64{Int64: position, Valid: true}
	}
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	queries := sqlitedb.New(tx)
	rows, err := queries.GetHistory(ctx, params)
	if err != nil {
		return result, fmt.Errorf("read history: %w", err)
	}
	for i, row := range rows {
		if i == query.Page.Limit {
			last := rows[i-1]
			result.Next = &fleet.Cursor{RecordedAt: time.UnixMicro(last.RecordedAt).UTC(), ID: strconv.FormatInt(last.ID, 10)}
			break
		}
		observation, err := decodeSQLiteObservation(row.RecordedAt, row.Projection)
		if err != nil {
			return result, err
		}
		result.Observations = append(result.Observations, observation)
	}
	if query.Since != nil && query.Page.After == nil {
		row, err := queries.GetHistoryBaseline(ctx, sqlitedb.GetHistoryBaselineParams{DeploymentID: id, Before: params.Since.Int64})
		switch {
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return result, fmt.Errorf("read history baseline: %w", err)
		default:
			baseline, err := decodeSQLiteObservation(row.RecordedAt, row.Projection)
			if err != nil {
				return result, err
			}
			result.Baseline = &baseline
		}
	}
	return result, tx.Commit()
}

func decodeSQLiteObservation(recordedAt int64, projection []byte) (fleet.Observation, error) {
	var deployment fleet.Deployment
	if err := json.Unmarshal(projection, &deployment); err != nil {
		return fleet.Observation{}, fmt.Errorf("decode stored observation: %w", err)
	}
	return fleet.Observation{Deployment: deployment, RecordedAt: time.UnixMicro(recordedAt).UTC()}, nil
}

func sqliteTimestamp(at time.Time) sql.NullInt64 {
	return sql.NullInt64{Int64: at.UnixMicro(), Valid: true}
}
