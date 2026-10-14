package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
	"github.com/kubebao/openbao-operator-console/internal/store/database"
)

// ErrInvalidCursor reports a cursor that names no position in the requested list.
var ErrInvalidCursor = errors.New("invalid page cursor")

func timestamp(at time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: at, Valid: true} }

// rowLimit reads one row beyond the page so that the last page needs no cursor.
func rowLimit(page fleet.Page) (int32, error) {
	if page.Limit < 1 || page.Limit > 500 {
		return 0, fmt.Errorf("page limit %d is out of range", page.Limit)
	}
	return int32(page.Limit) + 1, nil
}

// Activity returns one newest-first page of events in the given scopes. The scope applies
// before the limit, so events outside it never displace authorized events.
func (p *Postgres) Activity(ctx context.Context, scopes []config.Scope, page fleet.Page) (fleet.ActivityPage, error) {
	result := fleet.ActivityPage{Events: []fleet.Activity{}}
	limit, err := rowLimit(page)
	if err != nil {
		return result, err
	}
	params := database.ListActivityParams{ConnectionIds: []string{}, Namespaces: []string{}, RowLimit: limit}
	for _, scope := range scopes {
		for _, namespace := range scope.Namespaces {
			params.ConnectionIds = append(params.ConnectionIds, scope.ConnectionID)
			params.Namespaces = append(params.Namespaces, namespace)
		}
	}
	if len(params.ConnectionIds) == 0 {
		return result, nil
	}
	if page.After != nil {
		params.AfterAt = timestamp(page.After.RecordedAt)
		params.AfterID = pgtype.Text{String: page.After.ID, Valid: true}
	}
	rows, err := database.New(p.pool).ListActivity(ctx, params)
	if err != nil {
		return result, fmt.Errorf("list activity: %w", err)
	}
	for i, row := range rows {
		if i == page.Limit {
			last := rows[i-1]
			result.Next = &fleet.Cursor{RecordedAt: last.RecordedAt.Time, ID: last.ID}
			break
		}
		activity, err := decodeActivity(row.Projection, row.RecordedAt.Time)
		if err != nil {
			return result, err
		}
		result.Events = append(result.Events, activity)
	}
	return result, nil
}

// decodeActivity projects pre-structured events without interpreting their prose. Their
// identity and database timestamp remain available, while the unavailable change stays
// unknown. Free-form titles and details never enter the current API contract.
func decodeActivity(projection []byte, recordedAt time.Time) (fleet.Activity, error) {
	var stored struct {
		fleet.Activity
		Kind json.RawMessage `json:"kind"`
	}
	if err := json.Unmarshal(projection, &stored); err != nil {
		return fleet.Activity{}, fmt.Errorf("decode stored activity: %w", err)
	}
	if stored.Kind == nil {
		return fleet.Activity{
			ID: stored.ID, DeploymentID: stored.DeploymentID, ConnectionID: stored.ConnectionID,
			Namespace: stored.Namespace, RecordedAt: recordedAt, Kind: fleet.ActivityLegacy,
		}, nil
	}
	if err := json.Unmarshal(stored.Kind, &stored.Activity.Kind); err != nil {
		return fleet.Activity{}, fmt.Errorf("decode stored activity kind: %w", err)
	}
	switch stored.Activity.Kind {
	case fleet.ActivityHealthChanged, fleet.ActivityCollectionChanged, fleet.ActivityFindingRaised,
		fleet.ActivityFindingResolved, fleet.ActivityFindingSeverityChanged, fleet.ActivityLegacy:
		return stored.Activity, nil
	default:
		return fleet.Activity{}, errors.New("stored activity has an unknown kind")
	}
}

// History returns one newest-first page of a deployment's observations. The first page of
// a windowed query also carries the newest observation before the window. Both reads share
// one snapshot, so the baseline always precedes the page.
func (p *Postgres) History(ctx context.Context, id string, query fleet.HistoryQuery) (fleet.HistoryPage, error) {
	result := fleet.HistoryPage{Observations: []fleet.Observation{}}
	limit, err := rowLimit(query.Page)
	if err != nil {
		return result, err
	}
	params := database.GetHistoryParams{DeploymentID: id, RowLimit: limit}
	if query.Since != nil {
		params.Since = timestamp(*query.Since)
	}
	if after := query.Page.After; after != nil {
		position, err := strconv.ParseInt(after.ID, 10, 64)
		if err != nil {
			return result, ErrInvalidCursor
		}
		params.AfterAt = timestamp(after.RecordedAt)
		params.AfterID = pgtype.Int8{Int64: position, Valid: true}
	}
	tx, err := p.pool.BeginTx(ctx, repeatableRead())
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := database.New(tx)
	rows, err := queries.GetHistory(ctx, params)
	if err != nil {
		return result, fmt.Errorf("read history: %w", err)
	}
	for i, row := range rows {
		if i == query.Page.Limit {
			last := rows[i-1]
			result.Next = &fleet.Cursor{RecordedAt: last.RecordedAt.Time, ID: strconv.FormatInt(last.ID, 10)}
			break
		}
		observation, err := decodeObservation(row.RecordedAt, row.Projection)
		if err != nil {
			return result, err
		}
		result.Observations = append(result.Observations, observation)
	}
	if query.Since != nil && query.Page.After == nil {
		row, err := queries.GetHistoryBaseline(ctx, database.GetHistoryBaselineParams{DeploymentID: id, Before: params.Since})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return result, fmt.Errorf("read history baseline: %w", err)
		default:
			baseline, err := decodeObservation(row.RecordedAt, row.Projection)
			if err != nil {
				return result, err
			}
			result.Baseline = &baseline
		}
	}
	return result, tx.Commit(ctx)
}

func decodeObservation(recordedAt pgtype.Timestamptz, projection []byte) (fleet.Observation, error) {
	var deployment fleet.Deployment
	if err := json.Unmarshal(projection, &deployment); err != nil {
		return fleet.Observation{}, fmt.Errorf("decode stored observation: %w", err)
	}
	return fleet.Observation{Deployment: deployment, RecordedAt: recordedAt.Time}, nil
}
