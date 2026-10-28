package trip

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	"github.com/bezbayana777/trip_go/internal/infra/postgres"
	telemetry "github.com/bezbayana777/trip_go/internal/infra/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type DB interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) getDB(ctx context.Context) DB {
	if tx, ok := postgres.TxFromContext(ctx); ok {
		return tx
	}
	return r.pool
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

func (r *Repository) CreateTrip(ctx context.Context, arg CreateTripParams) error {
	start := time.Now()
	operationName := "INSERT"
	collectionName := "trips"

	ctx, span := telemetry.Tracer().Start(ctx, "db "+operationName+" "+collectionName,
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()

	span.SetAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation.name", operationName),
		attribute.String("db.collection.name", collectionName),
	)

	query := `
		INSERT INTO trips (
			id, user_id, driver_id, 
			start_latitude, start_longitude, end_latitude, end_longitude, 
			price, status, started_at
		) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := r.getDB(ctx).Exec(ctx, query,
		arg.ID,
		arg.UserID,
		arg.DriverID,
		arg.StartLatitude,
		arg.StartLongitude,
		arg.EndLatitude,
		arg.EndLongitude,
		arg.Price,
		arg.Status,
		arg.StartedAt,
	)

	duration := time.Since(start).Seconds()
	errType := ""
	if err != nil {
		errType = fmt.Sprintf("%T", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	if telemetry.GlobalMetrics.DbDuration != nil {
		telemetry.GlobalMetrics.DbDuration.Record(ctx, duration,
			metric.WithAttributes(
				attribute.String("db.operation.name", operationName),
				attribute.String("db.collection.name", collectionName),
				attribute.String("error.type", errType),
			),
		)
	}

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return trip.ErrDriverBusy
		}
		return fmt.Errorf("failed to create trip: %w", err)
	}

	return nil
}

func (r *Repository) GetTrip(ctx context.Context, id uuid.UUID) (*trip.Trip, error) {
	start := time.Now()
	operationName := "SELECT"
	collectionName := "trips"

	ctx, span := telemetry.Tracer().Start(ctx, "db "+operationName+" "+collectionName,
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()

	span.SetAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation.name", operationName),
		attribute.String("db.collection.name", collectionName),
	)

	query := `
		SELECT 
			id, user_id, driver_id, 
			start_latitude, start_longitude, end_latitude, end_longitude, 
			price, status, started_at, finished_at
		FROM trips 
		WHERE id = $1
	`

	var t trip.Trip
	err := r.getDB(ctx).QueryRow(ctx, query, id).Scan(
		&t.ID,
		&t.UserID,
		&t.DriverID,
		&t.StartPoint.Latitude,
		&t.StartPoint.Longitude,
		&t.EndPoint.Latitude,
		&t.EndPoint.Longitude,
		&t.Price,
		&t.Status,
		&t.StartedAt,
		&t.FinishedAt,
	)

	duration := time.Since(start).Seconds()
	errType := ""
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		errType = fmt.Sprintf("%T", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	if telemetry.GlobalMetrics.DbDuration != nil {
		telemetry.GlobalMetrics.DbDuration.Record(ctx, duration,
			metric.WithAttributes(
				attribute.String("db.operation.name", operationName),
				attribute.String("db.collection.name", collectionName),
				attribute.String("error.type", errType),
			),
		)
	}

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, trip.ErrTripNotFound
		}
		return nil, fmt.Errorf("failed to get trip by id: %w", err)
	}

	return &t, nil
}

func (r *Repository) FinishTrip(ctx context.Context, id uuid.UUID) error {
	start := time.Now()
	operationName := "UPDATE"
	collectionName := "trips"

	ctx, span := telemetry.Tracer().Start(ctx, "db "+operationName+" "+collectionName,
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()

	span.SetAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation.name", operationName),
		attribute.String("db.collection.name", collectionName),
	)

	var currentStatus string
	checkQuery := `SELECT status FROM trips WHERE id = $1`
	err := r.getDB(ctx).QueryRow(ctx, checkQuery, id).Scan(&currentStatus)
	if err != nil {
		duration := time.Since(start).Seconds()
		errType := ""
		if !errors.Is(err, pgx.ErrNoRows) {
			errType = fmt.Sprintf("%T", err)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		if telemetry.GlobalMetrics.DbDuration != nil {
			telemetry.GlobalMetrics.DbDuration.Record(ctx, duration,
				metric.WithAttributes(
					attribute.String("db.operation.name", operationName),
					attribute.String("db.collection.name", collectionName),
					attribute.String("error.type", errType),
				),
			)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return trip.ErrTripNotFound
		}
		return fmt.Errorf("failed to check trip status: %w", err)
	}

	if currentStatus == "completed" {
		return trip.ErrTripCompleted
	}

	query := `
		UPDATE trips 
		SET status = 'completed', finished_at = $2 
		WHERE id = $1 AND status = 'active'
	`
	cmdTag, err := r.getDB(ctx).Exec(ctx, query, id, time.Now())

	duration := time.Since(start).Seconds()
	errType := ""
	if err != nil {
		errType = fmt.Sprintf("%T", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	if telemetry.GlobalMetrics.DbDuration != nil {
		telemetry.GlobalMetrics.DbDuration.Record(ctx, duration,
			metric.WithAttributes(
				attribute.String("db.operation.name", operationName),
				attribute.String("db.collection.name", collectionName),
				attribute.String("error.type", errType),
			),
		)
	}

	if err != nil {
		return fmt.Errorf("failed to finish trip: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return trip.ErrTripCompleted
	}

	return nil
}

func (r *Repository) GetActiveDriverTrip(ctx context.Context, driverID uuid.UUID) (*trip.Trip, error) {
	start := time.Now()
	operationName := "SELECT"
	collectionName := "trips"

	ctx, span := telemetry.Tracer().Start(ctx, "db "+operationName+" "+collectionName,
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()

	span.SetAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation.name", operationName),
		attribute.String("db.collection.name", collectionName),
	)

	query := `
		SELECT id, user_id, driver_id, start_latitude, start_longitude, 
			   end_latitude, end_longitude, price, status, started_at, finished_at
		FROM trips
		WHERE driver_id = $1 AND status = $2
		LIMIT 1
	`

	var tr trip.Trip
	var statusStr string

	err := r.getDB(ctx).QueryRow(ctx, query, driverID, string(trip.StatusActive)).Scan(
		&tr.ID,
		&tr.UserID,
		&tr.DriverID,
		&tr.StartPoint.Latitude,
		&tr.StartPoint.Longitude,
		&tr.EndPoint.Latitude,
		&tr.EndPoint.Longitude,
		&tr.Price,
		&statusStr,
		&tr.StartedAt,
		&tr.FinishedAt,
	)

	duration := time.Since(start).Seconds()
	errType := ""
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		errType = fmt.Sprintf("%T", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	if telemetry.GlobalMetrics.DbDuration != nil {
		telemetry.GlobalMetrics.DbDuration.Record(ctx, duration,
			metric.WithAttributes(
				attribute.String("db.operation.name", operationName),
				attribute.String("db.collection.name", collectionName),
				attribute.String("error.type", errType),
			),
		)
	}

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, trip.ErrTripNotFound
		}
		return nil, err
	}

	tr.Status = trip.Status(statusStr)
	return &tr, nil
}