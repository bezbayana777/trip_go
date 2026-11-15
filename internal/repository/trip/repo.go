package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	domaintrip "github.com/bezbayana777/trip_go/internal/domain/trip"
	"github.com/bezbayana777/trip_go/internal/infra/postgres"
	"github.com/bezbayana777/trip_go/internal/model"
)

type DB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}
type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }
func (r *Repository) getDB(ctx context.Context) DB {
	if tx, ok := postgres.TxFromContext(ctx); ok {
		return tx
	}
	return r.pool
}

func (r *Repository) CreateTrip(ctx context.Context, arg model.CreateTripParams) error {
	_, err := r.getDB(ctx).Exec(ctx, `INSERT INTO trips (id,user_id,driver_id,start_latitude,start_longitude,end_latitude,end_longitude,price,status,started_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, arg.ID, arg.UserID, arg.DriverID, arg.StartPoint.Latitude, arg.StartPoint.Longitude, arg.EndPoint.Latitude, arg.EndPoint.Longitude, arg.Price, arg.Status, arg.StartedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domaintrip.ErrDriverBusy
		}
		return fmt.Errorf("failed to create trip: %w", err)
	}
	if _, err = r.getDB(ctx).Exec(ctx, `INSERT INTO trip_status_history (trip_id,from_status,to_status) VALUES ($1,NULL,$2)`, arg.ID, arg.Status); err != nil {
		return fmt.Errorf("failed to create trip status history: %w", err)
	}
	return nil
}

func (r *Repository) GetTrip(ctx context.Context, id uuid.UUID) (*domaintrip.Trip, error) {
	var t domaintrip.Trip
	err := r.getDB(ctx).QueryRow(ctx, `SELECT id,user_id,driver_id,start_latitude,start_longitude,end_latitude,end_longitude,price,status,started_at,finished_at FROM trips WHERE id=$1`, id).Scan(&t.ID, &t.UserID, &t.DriverID, &t.StartPoint.Latitude, &t.StartPoint.Longitude, &t.EndPoint.Latitude, &t.EndPoint.Longitude, &t.Price, &t.Status, &t.StartedAt, &t.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domaintrip.ErrTripNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get trip: %w", err)
	}
	return &t, nil
}

// GetTripForUpdate locks one trip until the surrounding transaction completes.
func (r *Repository) GetTripForUpdate(ctx context.Context, id uuid.UUID) (*domaintrip.Trip, error) {
	var t domaintrip.Trip
	err := r.getDB(ctx).QueryRow(ctx, `SELECT id,user_id,driver_id,start_latitude,start_longitude,end_latitude,end_longitude,price,status,started_at,finished_at FROM trips WHERE id=$1 FOR UPDATE`, id).Scan(&t.ID, &t.UserID, &t.DriverID, &t.StartPoint.Latitude, &t.StartPoint.Longitude, &t.EndPoint.Latitude, &t.EndPoint.Longitude, &t.Price, &t.Status, &t.StartedAt, &t.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domaintrip.ErrTripNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load trip for finish: %w", err)
	}
	if t.Status == domaintrip.StatusCompleted {
		return nil, domaintrip.ErrTripCompleted
	}
	return &t, nil
}

func (r *Repository) SaveFinishedTrip(ctx context.Context, t *domaintrip.Trip) error {
	if t.Status != domaintrip.StatusCompleted || t.FinishedAt == nil {
		return fmt.Errorf("invalid completed trip state")
	}
	result, err := r.getDB(ctx).Exec(ctx, `UPDATE trips SET status='completed',finished_at=$2 WHERE id=$1 AND status='active'`, t.ID, *t.FinishedAt)
	if err != nil {
		return fmt.Errorf("failed to finish trip: %w", err)
	}
	if result.RowsAffected() != 1 {
		return domaintrip.ErrTripCompleted
	}
	if _, err = r.getDB(ctx).Exec(ctx, `INSERT INTO trip_status_history (trip_id,from_status,to_status) VALUES ($1,'active','completed')`, t.ID); err != nil {
		return fmt.Errorf("failed to create trip status history: %w", err)
	}
	return nil
}

func (r *Repository) GetActiveDriverTrip(ctx context.Context, driverID uuid.UUID) (*domaintrip.Trip, error) {
	var t domaintrip.Trip
	err := r.getDB(ctx).QueryRow(ctx, `SELECT id,user_id,driver_id,start_latitude,start_longitude,end_latitude,end_longitude,price,status,started_at,finished_at FROM trips WHERE driver_id=$1 AND status=$2 LIMIT 1`, driverID, string(domaintrip.StatusActive)).Scan(&t.ID, &t.UserID, &t.DriverID, &t.StartPoint.Latitude, &t.StartPoint.Longitude, &t.EndPoint.Latitude, &t.EndPoint.Longitude, &t.Price, &t.Status, &t.StartedAt, &t.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domaintrip.ErrTripNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get active trip: %w", err)
	}
	return &t, nil
}

func (r *Repository) CountActiveTrips(ctx context.Context) (int64, error) {
	var count int64
	if err := r.getDB(ctx).QueryRow(ctx, `SELECT count(*) FROM trips WHERE status=$1`, string(domaintrip.StatusActive)).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count active trips: %w", err)
	}
	return count, nil
}
