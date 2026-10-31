package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/bezbayana777/trip_go/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrTripNotFound  = errors.New("trip not found")
	ErrTripCompleted = errors.New("trip already completed")
	ErrDriverBusy    = errors.New("driver already has an active trip")
)

type DBops interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type TripRepository struct {
	pool *pgxpool.Pool
}

func NewTripRepository(pool *pgxpool.Pool) *TripRepository {
	return &TripRepository{pool: pool}
}

type CreateTripParams struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	DriverID       uuid.UUID
	StartLatitude  float64
	StartLongitude float64
	EndLatitude    float64
	EndLongitude   float64
	Price          int64
	Status         string
	StartedAt      time.Time
}

func isDriverBusy(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "trips_driver_active_idx"
}

func (r *TripRepository) CreateTrip(ctx context.Context, arg CreateTripParams) error {
	ctx, cancel := db.WithQueryTimeout(ctx)
	defer cancel()

	conn := getDB(ctx, r.pool)
	builder := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	now := time.Now()

	queryTrip, argsTrip, err := builder.
		Insert("trips").
		Columns(
			"id", "user_id", "driver_id",
			"start_latitude", "start_longitude",
			"end_latitude", "end_longitude",
			"price", "status", "started_at",
			"created_at", "updated_at",
		).
		Values(
			arg.ID, arg.UserID, arg.DriverID,
			arg.StartLatitude, arg.StartLongitude,
			arg.EndLatitude, arg.EndLongitude,
			arg.Price, arg.Status, arg.StartedAt,
			now, now,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build trip insert query: %w", err)
	}

	if _, err = conn.Exec(ctx, queryTrip, argsTrip...); err != nil {
		if isDriverBusy(err) {
			return fmt.Errorf("%w: %v", ErrDriverBusy, err)
		}
		return fmt.Errorf("failed to insert trip: %w", err)
	}

	queryHistory, argsHistory, err := builder.
		Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason", "changed_at").
		Values(arg.ID, nil, arg.Status, "Trip created", now).
		ToSql()
	if err != nil {
		return fmt.Errorf("failed to build history insert query: %w", err)
	}

	if _, err = conn.Exec(ctx, queryHistory, argsHistory...); err != nil {
		return fmt.Errorf("failed to insert trip status history: %w", err)
	}

	return nil
}

type Trip struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	DriverID       uuid.UUID
	StartLatitude  float64
	StartLongitude float64
	EndLatitude    float64
	EndLongitude   float64
	Price          int64
	Status         string
	StartedAt      time.Time
	FinishedAt     *time.Time
}

func (r *TripRepository) GetTrip(ctx context.Context, tripID uuid.UUID) (*Trip, error) {
	ctx, cancel := db.WithQueryTimeout(ctx)
	defer cancel()

	conn := getDB(ctx, r.pool)
	builder := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

	query, args, err := builder.
		Select(
			"id", "user_id", "driver_id",
			"start_latitude", "start_longitude",
			"end_latitude", "end_longitude",
			"price", "status", "started_at", "finished_at",
		).
		From("trips").
		Where(squirrel.Eq{"id": tripID}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build get trip query: %w", err)
	}

	var t Trip
	err = conn.QueryRow(ctx, query, args...).Scan(
		&t.ID, &t.UserID, &t.DriverID,
		&t.StartLatitude, &t.StartLongitude,
		&t.EndLatitude, &t.EndLongitude,
		&t.Price, &t.Status, &t.StartedAt, &t.FinishedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTripNotFound
		}
		return nil, fmt.Errorf("failed to get trip: %w", err)
	}

	return &t, nil
}

func (r *TripRepository) FinishTrip(ctx context.Context, tripID uuid.UUID) (*Trip, error) {
	ctx, cancel := db.WithQueryTimeout(ctx)
	defer cancel()

	conn := getDB(ctx, r.pool)
	builder := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	now := time.Now()

	queryUpdate, argsUpdate, err := builder.
		Update("trips").
		Set("status", "completed").
		Set("finished_at", now).
		Set("updated_at", now).
		Where(squirrel.Eq{"id": tripID, "status": "active"}).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build finish trip query: %w", err)
	}

	tag, err := conn.Exec(ctx, queryUpdate, argsUpdate...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute finish trip: %w", err)
	}

	if tag.RowsAffected() == 0 {
		queryCheck, argsCheck, err := builder.
			Select("status").
			From("trips").
			Where(squirrel.Eq{"id": tripID}).
			ToSql()
		if err != nil {
			return nil, fmt.Errorf("failed to build check query: %w", err)
		}

		var currentStatus string
		if err = conn.QueryRow(ctx, queryCheck, argsCheck...).Scan(&currentStatus); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrTripNotFound
			}
			return nil, fmt.Errorf("failed to check trip existence: %w", err)
		}

		if currentStatus == "completed" {
			return nil, ErrTripCompleted
		}
		return nil, fmt.Errorf("trip %s is in unexpected status %q", tripID, currentStatus)
	}

	queryHistory, argsHistory, err := builder.
		Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason", "changed_at").
		Values(tripID, "active", "completed", "Trip finished", now).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build finish history query: %w", err)
	}

	if _, err = conn.Exec(ctx, queryHistory, argsHistory...); err != nil {
		return nil, fmt.Errorf("failed to insert trip status history for finish: %w", err)
	}

	return r.GetTrip(ctx, tripID)
}