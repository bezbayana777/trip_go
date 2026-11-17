package position

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bezbayana777/trip_go/internal/db"
	domain "github.com/bezbayana777/trip_go/internal/domain/position"
	tripdomain "github.com/bezbayana777/trip_go/internal/domain/trip"
	"github.com/bezbayana777/trip_go/internal/infra/postgres"
)

type DB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }
func (r *Repository) db(ctx context.Context) DB {
	if tx, ok := postgres.TxFromContext(ctx); ok {
		return tx
	}
	return r.pool
}

func (r *Repository) Save(ctx context.Context, tripID uuid.UUID, p domain.Position) (domain.Position, error) {
	ctx, cancel := db.WithQueryTimeout(ctx)
	defer cancel()
	check, checkArgs, err := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).Select("status").From("trips").Where(squirrel.Eq{"id": tripID}).Suffix("FOR UPDATE").ToSql()
	if err != nil {
		return domain.Position{}, fmt.Errorf("build trip status query: %w", err)
	}
	var status string
	err = r.db(ctx).QueryRow(ctx, check, checkArgs...).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Position{}, tripdomain.ErrTripNotFound
	}
	if err != nil {
		return domain.Position{}, fmt.Errorf("load trip for position: %w", err)
	}
	if status != "active" {
		return domain.Position{}, tripdomain.ErrTripCompleted
	}
	insert, insertArgs, err := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).Insert("trip_positions").Columns("trip_id", "latitude", "longitude", "recorded_at").Values(tripID, p.Coordinates.Latitude, p.Coordinates.Longitude, p.RecordedAt).Suffix("RETURNING id").ToSql()
	if err != nil {
		return domain.Position{}, fmt.Errorf("build insert position query: %w", err)
	}
	err = r.db(ctx).QueryRow(ctx, insert, insertArgs...).Scan(&p.ID)
	if err != nil {
		return domain.Position{}, fmt.Errorf("insert trip position: %w", err)
	}
	update, updateArgs, err := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).Update("trips").Set("last_position_at", squirrel.Expr("GREATEST(COALESCE(last_position_at, ?), ?)", p.RecordedAt, p.RecordedAt)).Where(squirrel.Eq{"id": tripID}).ToSql()
	if err != nil {
		return domain.Position{}, fmt.Errorf("build update last position query: %w", err)
	}
	if _, err = r.db(ctx).Exec(ctx, update, updateArgs...); err != nil {
		return domain.Position{}, fmt.Errorf("update last trip position time: %w", err)
	}
	p.TripID = tripID
	return p, nil
}

func (r *Repository) List(ctx context.Context, tripID uuid.UUID) ([]domain.Position, error) {
	ctx, cancel := db.WithQueryTimeout(ctx)
	defer cancel()
	var exists bool
	if err := r.db(ctx).QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM trips WHERE id = $1)", tripID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check trip before listing positions: %w", err)
	}
	if !exists {
		return nil, domain.ErrTripNotFound
	}
	query, args, err := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).Select("id", "trip_id", "latitude", "longitude", "recorded_at").From("trip_positions").Where(squirrel.Eq{"trip_id": tripID}).OrderBy("recorded_at", "id").ToSql()
	if err != nil {
		return nil, fmt.Errorf("build list positions query: %w", err)
	}
	rows, err := r.db(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list trip positions: %w", err)
	}
	defer rows.Close()
	positions := make([]domain.Position, 0)
	for rows.Next() {
		var p domain.Position
		if err := rows.Scan(&p.ID, &p.TripID, &p.Coordinates.Latitude, &p.Coordinates.Longitude, &p.RecordedAt); err != nil {
			return nil, fmt.Errorf("scan trip position: %w", err)
		}
		positions = append(positions, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trip positions: %w", err)
	}
	return positions, nil
}

func (r *Repository) FindStale(ctx context.Context, staleAfter time.Duration) ([]domain.StaleTrip, error) {
	ctx, cancel := db.WithQueryTimeout(ctx)
	defer cancel()
	staleAfterMillis := staleAfter.Milliseconds()
	if staleAfterMillis < 1 {
		staleAfterMillis = 1
	}
	query, args, err := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).
		Select("id", "driver_id").From("trips").
		Where("status = ?", "active").
		Where("COALESCE(last_position_at, started_at) < now() - (? * interval '1 millisecond')", staleAfterMillis).
		OrderBy("COALESCE(last_position_at, started_at)", "id").ToSql()
	if err != nil {
		return nil, fmt.Errorf("build stale trips query: %w", err)
	}
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("find stale trips: %w", err)
	}
	defer rows.Close()
	trips := make([]domain.StaleTrip, 0)
	for rows.Next() {
		var t domain.StaleTrip
		if err := rows.Scan(&t.ID, &t.DriverID); err != nil {
			return nil, fmt.Errorf("scan stale trip: %w", err)
		}
		trips = append(trips, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stale trips: %w", err)
	}
	return trips, nil
}

// StaleQueryPlan is used by integration checks to verify the partial scan index.
func (r *Repository) StaleQueryPlan(ctx context.Context, staleAfter time.Duration) ([]string, error) {
	ctx, cancel := db.WithQueryTimeout(ctx)
	defer cancel()
	staleAfterMillis := staleAfter.Milliseconds()
	if staleAfterMillis < 1 {
		staleAfterMillis = 1
	}
	rows, err := r.pool.Query(ctx, "EXPLAIN SELECT id,driver_id FROM trips WHERE status=$1 AND COALESCE(last_position_at,started_at) < now()-($2 * interval '1 millisecond') ORDER BY COALESCE(last_position_at,started_at),id", "active", staleAfterMillis)
	if err != nil {
		return nil, fmt.Errorf("explain stale trip query: %w", err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return nil, fmt.Errorf("scan stale trip query plan: %w", err)
		}
		plan = append(plan, line)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read stale trip query plan: %w", err)
	}
	return plan, nil
}
