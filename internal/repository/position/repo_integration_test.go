//go:build integration

package position

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	domain "github.com/bezbayana777/trip_go/internal/domain/position"
	"github.com/bezbayana777/trip_go/internal/infra/postgres"
)

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if os.Getenv("DATABASE_URL") == "" {
		t.Fatal("DATABASE_URL is required; run tripgoctl connect first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	require.NoError(t, err)
	require.NoError(t, pool.Ping(ctx))
	t.Cleanup(pool.Close)
	return pool
}

func insertTrip(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id, driverID uuid.UUID, status string, startedAt time.Time, lastPositionAt *time.Time) {
	t.Helper()
	var finishedAt *time.Time
	if status == "completed" {
		finished := startedAt.Add(time.Minute)
		finishedAt = &finished
	}
	_, err := pool.Exec(ctx, `INSERT INTO trips
		(id,user_id,driver_id,start_latitude,start_longitude,end_latitude,end_longitude,price,status,started_at,finished_at,last_position_at)
		VALUES ($1,$2,$3,0,0,1,1,100,$4,$5,$6,$7)`, id, uuid.New(), driverID, status, startedAt, finishedAt, lastPositionAt)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM trips WHERE id=$1`, id) })
}

func TestFindStaleSelectsOnlyActiveTripsWithoutFreshPosition(t *testing.T) {
	pool := integrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	startedOld := time.Now().UTC().Add(-2 * time.Hour)
	startedFresh := time.Now().UTC()
	lastOld := time.Now().UTC().Add(-2 * time.Hour)
	lastFresh := time.Now().UTC()
	withoutPosition, withStalePosition := uuid.New(), uuid.New()
	fresh, completed := uuid.New(), uuid.New()
	insertTrip(t, ctx, pool, withoutPosition, uuid.New(), "active", startedOld, nil)
	insertTrip(t, ctx, pool, withStalePosition, uuid.New(), "active", startedFresh, &lastOld)
	insertTrip(t, ctx, pool, fresh, uuid.New(), "active", startedFresh, &lastFresh)
	insertTrip(t, ctx, pool, completed, uuid.New(), "completed", startedOld, nil)

	trips, err := New(pool).FindStale(ctx, time.Hour)
	require.NoError(t, err)
	got := make(map[uuid.UUID]struct{}, len(trips))
	for _, trip := range trips {
		got[trip.ID] = struct{}{}
	}
	require.Contains(t, got, withoutPosition)
	require.Contains(t, got, withStalePosition)
	require.NotContains(t, got, fresh)
	require.NotContains(t, got, completed)
}

func TestSavePositionUpdatesLastPositionAtomically(t *testing.T) {
	pool := integrationPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tripID := uuid.New()
	insertTrip(t, ctx, pool, tripID, uuid.New(), "active", time.Now().UTC(), nil)
	repo := New(pool)
	txManager := postgres.NewTxManager(pool)
	newer := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	position := domain.Position{Coordinates: domain.Coordinates{Latitude: 59.93, Longitude: 30.33}, RecordedAt: newer}
	rollbackErr := errors.New("force rollback")
	err := txManager.Do(ctx, func(txCtx context.Context) error {
		_, saveErr := repo.Save(txCtx, tripID, position)
		if saveErr != nil {
			return saveErr
		}
		return rollbackErr
	})
	require.ErrorIs(t, err, rollbackErr)
	var count int
	var lastPositionAt *time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM trip_positions WHERE trip_id=$1`, tripID).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, pool.QueryRow(ctx, `SELECT last_position_at FROM trips WHERE id=$1`, tripID).Scan(&lastPositionAt))
	require.Nil(t, lastPositionAt)

	err = txManager.Do(ctx, func(txCtx context.Context) error {
		_, saveErr := repo.Save(txCtx, tripID, position)
		return saveErr
	})
	require.NoError(t, err)
	older := position
	older.RecordedAt = newer.Add(-time.Hour)
	err = txManager.Do(ctx, func(txCtx context.Context) error {
		_, saveErr := repo.Save(txCtx, tripID, older)
		return saveErr
	})
	require.NoError(t, err)
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM trip_positions WHERE trip_id=$1`, tripID).Scan(&count))
	require.Equal(t, 2, count)
	require.NoError(t, pool.QueryRow(ctx, `SELECT last_position_at FROM trips WHERE id=$1`, tripID).Scan(&lastPositionAt))
	require.NotNil(t, lastPositionAt)
	require.WithinDuration(t, newer, *lastPositionAt, time.Microsecond)
}
