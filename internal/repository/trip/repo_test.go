//go:build integration

package trip_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	"github.com/bezbayana777/trip_go/internal/infra/postgres"
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
	defer pool.Close()

	repo := repoTrip.New(pool)
	ctx := context.Background()

	tripID := uuid.New()
	userID := uuid.New()
	driverID := uuid.New()

	params := repoTrip.CreateTripParams{
		ID:             tripID,
		UserID:         userID,
		DriverID:       driverID,
		StartLatitude:  55.75,
		StartLongitude: 37.61,
		EndLatitude:    55.76,
		EndLongitude:   37.62,
		Price:          1000,
		Status:         string(trip.StatusActive),
		StartedAt:      time.Now().UTC(),
	}

	err := repo.CreateTrip(ctx, params)
	require.NoError(t, err)

	tr, err := repo.GetTrip(ctx, tripID)
	require.NoError(t, err)
	require.NotNil(t, tr)

	assert.Equal(t, tripID, tr.ID)
	assert.Equal(t, userID, tr.UserID)
	assert.Equal(t, driverID, tr.DriverID)
	assert.Equal(t, trip.StatusActive, tr.Status)
	assert.Equal(t, int64(1000), tr.Price)
}

func TestRepository_FinishTrip(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()

	repo := repoTrip.New(pool)
	ctx := context.Background()

	tripID := uuid.New()
	params := repoTrip.CreateTripParams{
		ID:             tripID,
		UserID:         uuid.New(),
		DriverID:       uuid.New(),
		StartLatitude:  55.0,
		StartLongitude: 37.0,
		EndLatitude:    55.1,
		EndLongitude:   37.1,
		Price:          500,
		Status:         string(trip.StatusActive),
		StartedAt:      time.Now().UTC(),
	}

	err := repo.CreateTrip(ctx, params)
	require.NoError(t, err)

	err = repo.FinishTrip(ctx, tripID)
	assert.NoError(t, err)

	tr, err := repo.GetTrip(ctx, tripID)
	require.NoError(t, err)
	assert.Equal(t, trip.StatusCompleted, tr.Status)
	assert.NotNil(t, tr.FinishedAt)
}

func TestTxManager_RollbackOnesError(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()

	txManager := postgres.NewTxManager(pool)
	repo := repoTrip.New(pool)
	ctx := context.Background()

	tripID := uuid.New()

	err := txManager.Do(ctx, func(ctxTx context.Context) error {
		params := repoTrip.CreateTripParams{
			ID:             tripID,
			UserID:         uuid.New(),
			DriverID:       uuid.New(),
			StartLatitude:  55.0,
			StartLongitude: 37.0,
			EndLatitude:    55.1,
			EndLongitude:   37.1,
			Price:          700,
			Status:         string(trip.StatusActive),
			StartedAt:      time.Now().UTC(),
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