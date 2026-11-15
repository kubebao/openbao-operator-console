package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
	"github.com/kubebao/openbao-operator-console/internal/store/database"
	"github.com/kubebao/openbao-operator-console/internal/store/sqlitedb"
)

func chartPoint(at time.Time, projection []byte) (fleet.ChartObservation, error) {
	point := fleet.ChartObservation{RecordedAt: at}
	if err := json.Unmarshal(projection, &point.Deployment); err != nil {
		return point, fmt.Errorf("decode chart projection: %w", err)
	}
	return point, nil
}

// Charts reads a complete window and its baseline in one repeatable snapshot.
func (p *Postgres) Charts(ctx context.Context, id string, start, end time.Time) (fleet.ChartHistory, error) {
	tx, err := p.pool.BeginTx(ctx, repeatableRead())
	if err != nil {
		return fleet.ChartHistory{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := database.New(tx)
	rows, err := queries.GetCharts(ctx, database.GetChartsParams{DeploymentID: id, Since: timestamp(start), Until: timestamp(end)})
	if err != nil {
		return fleet.ChartHistory{}, fmt.Errorf("read chart window: %w", err)
	}
	points := make([]fleet.ChartObservation, 0, len(rows))
	for _, row := range rows {
		point, err := chartPoint(row.RecordedAt.Time, row.ChartProjection)
		if err != nil {
			return fleet.ChartHistory{}, err
		}
		points = append(points, point)
	}
	var baseline *fleet.ChartObservation
	row, err := queries.GetHistoryBaseline(ctx, database.GetHistoryBaselineParams{DeploymentID: id, Before: timestamp(start)})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return fleet.ChartHistory{}, fmt.Errorf("read chart baseline: %w", err)
	default:
		observation, err := decodeObservation(row.RecordedAt, row.Projection)
		if err != nil {
			return fleet.ChartHistory{}, err
		}
		point := observation.Chart()
		baseline = &point
	}
	return fleet.CompleteCharts(start, end, baseline, points), tx.Commit(ctx)
}

func (p *SQLite) Charts(ctx context.Context, id string, start, end time.Time) (fleet.ChartHistory, error) {
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fleet.ChartHistory{}, err
	}
	defer func() { _ = tx.Rollback() }()
	queries := sqlitedb.New(tx)
	rows, err := queries.GetCharts(ctx, sqlitedb.GetChartsParams{DeploymentID: id, Since: start.UnixMicro(), Until: end.UnixMicro()})
	if err != nil {
		return fleet.ChartHistory{}, fmt.Errorf("read chart window: %w", err)
	}
	points := make([]fleet.ChartObservation, 0, len(rows))
	for _, row := range rows {
		point, err := chartPoint(time.UnixMicro(row.RecordedAt).UTC(), []byte(row.ChartProjection))
		if err != nil {
			return fleet.ChartHistory{}, err
		}
		points = append(points, point)
	}
	var baseline *fleet.ChartObservation
	row, err := queries.GetHistoryBaseline(ctx, sqlitedb.GetHistoryBaselineParams{DeploymentID: id, Before: start.UnixMicro()})
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return fleet.ChartHistory{}, fmt.Errorf("read chart baseline: %w", err)
	default:
		observation, err := decodeSQLiteObservation(row.RecordedAt, row.Projection)
		if err != nil {
			return fleet.ChartHistory{}, err
		}
		point := observation.Chart()
		baseline = &point
	}
	return fleet.CompleteCharts(start, end, baseline, points), tx.Commit()
}
