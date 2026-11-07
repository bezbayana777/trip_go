//go:build integration

package trip_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	"github.com/bezbayana777/trip_go/internal/infra/postgres"
	"github.com/bezbayana777/trip_go/internal/model"
	repoTrip "github.com/bezbayana777/trip_go/internal/repository/trip"
)

func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL environment variable is not set. Did you run `tripgoctl connect` or source `.env`?")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := postgres.NewPool(ctx, databaseURL)
	require.NoError(t, err, "failed to connect to test database")

	return pool
}

func TestRepository_CreateAndGetTrip(t *testing.T) {
	pool := setupTestDB(t)
	t.Cleanup(pool.Close)

	repo := repoTrip.New(pool)
	txManager := postgres.NewTxManager(pool)
	ctx := context.Background()

	tripID := uuid.New()
	t.Cleanup(func() { _, err := pool.Exec(ctx, `DELETE FROM trips WHERE id=$1`, tripID); assert.NoError(t, err) })
	userID := uuid.New()
	driverID := uuid.New()

	params := model.CreateTripParams{
		ID: tripID, UserID: userID, DriverID: driverID,
		StartPoint: model.CoordinatesDTO{Latitude: 55.75, Longitude: 37.61},
		EndPoint:   model.CoordinatesDTO{Latitude: 55.76, Longitude: 37.62},
		Price:      1000,
		Status:     string(trip.StatusActive),
		StartedAt:  time.Now().UTC(),
	}

	err := txManager.Do(ctx, func(ctx context.Context) error { return repo.CreateTrip(ctx, params) })
	require.NoError(t, err)

	tr, err := repo.GetTrip(ctx, tripID)
	require.NoError(t, err)
	require.NotNil(t, tr)

	assert.Equal(t, tripID, tr.ID)
	assert.Equal(t, userID, tr.UserID)
	assert.Equal(t, driverID, tr.DriverID)
	assert.Equal(t, trip.StatusActive, tr.Status)
	assert.Equal(t, int64(1000), tr.Price)
	var history int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM trip_status_history WHERE trip_id=$1`, tripID).Scan(&history))
	assert.Equal(t, 1, history)
}

func TestRepository_FinishTrip(t *testing.T) {
	pool := setupTestDB(t)
	t.Cleanup(pool.Close)

	repo := repoTrip.New(pool)
	txManager := postgres.NewTxManager(pool)
	ctx := context.Background()

	tripID := uuid.New()
	t.Cleanup(func() { _, err := pool.Exec(ctx, `DELETE FROM trips WHERE id=$1`, tripID); assert.NoError(t, err) })
	params := model.CreateTripParams{
		ID: tripID, UserID: uuid.New(), DriverID: uuid.New(),
		StartPoint: model.CoordinatesDTO{Latitude: 55, Longitude: 37},
		EndPoint:   model.CoordinatesDTO{Latitude: 55.1, Longitude: 37.1},
		Price:      500,
		Status:     string(trip.StatusActive),
		StartedAt:  time.Now().UTC(),
	}

	err := txManager.Do(ctx, func(ctx context.Context) error { return repo.CreateTrip(ctx, params) })
	require.NoError(t, err)

	err = txManager.Do(ctx, func(ctx context.Context) error {
		finished, err := repo.GetTripForUpdate(ctx, tripID)
		if err != nil {
			return err
		}
		if err = finished.Finish(time.Now().UTC()); err != nil {
			return err
		}
		return repo.SaveFinishedTrip(ctx, finished)
	})
	require.NoError(t, err)

	tr, err := repo.GetTrip(ctx, tripID)
	require.NoError(t, err)
	assert.Equal(t, trip.StatusCompleted, tr.Status)
	assert.NotNil(t, tr.FinishedAt)
	var history int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM trip_status_history WHERE trip_id=$1`, tripID).Scan(&history))
	assert.Equal(t, 2, history)
}

func TestRepository_CreateRollsBackWhenHistoryInsertFails(t *testing.T) {
	pool := setupTestDB(t)
	t.Cleanup(pool.Close)
	ctx := context.Background()
	repo := repoTrip.New(pool)
	txManager := postgres.NewTxManager(pool)
	id := uuid.New()
	suffix := strings.ReplaceAll(id.String(), "-", "")
	functionName := "fail_history_" + suffix
	triggerName := "fail_history_trigger_" + suffix
	_, err := pool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.trip_id = '%s' THEN RAISE EXCEPTION 'forced history insert failure'; END IF; RETURN NEW; END $$`, functionName, id))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, fmt.Sprintf(`CREATE TRIGGER %s BEFORE INSERT ON trip_status_history FOR EACH ROW EXECUTE FUNCTION %s()`, triggerName, functionName))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON trip_status_history`, triggerName))
		_, _ = pool.Exec(ctx, fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, functionName))
	})
	params := model.CreateTripParams{ID: id, UserID: uuid.New(), DriverID: uuid.New(), StartPoint: model.CoordinatesDTO{}, EndPoint: model.CoordinatesDTO{}, Price: 0, Status: string(trip.StatusActive), StartedAt: time.Now().UTC()}
	err = txManager.Do(ctx, func(ctx context.Context) error { return repo.CreateTrip(ctx, params) })
	require.Error(t, err)
	_, err = repo.GetTrip(ctx, id)
	assert.ErrorIs(t, err, trip.ErrTripNotFound)
}

func TestTxManager_RollbackOnError(t *testing.T) {
	pool := setupTestDB(t)
	t.Cleanup(pool.Close)

	txManager := postgres.NewTxManager(pool)
	repo := repoTrip.New(pool)
	ctx := context.Background()

	tripID := uuid.New()

	err := txManager.Do(ctx, func(ctxTx context.Context) error {
		params := model.CreateTripParams{
			ID: tripID, UserID: uuid.New(), DriverID: uuid.New(),
			StartPoint: model.CoordinatesDTO{Latitude: 55, Longitude: 37},
			EndPoint:   model.CoordinatesDTO{Latitude: 55.1, Longitude: 37.1},
			Price:      700,
			Status:     string(trip.StatusActive),
			StartedAt:  time.Now().UTC(),
		}

		if err := repo.CreateTrip(ctxTx, params); err != nil {
			return err
		}

		return assert.AnError
	})

	assert.ErrorIs(t, err, assert.AnError)

	_, err = repo.GetTrip(ctx, tripID)
	assert.ErrorIs(t, err, trip.ErrTripNotFound)
}
