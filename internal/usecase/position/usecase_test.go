package position

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	domain "github.com/bezbayana777/trip_go/internal/domain/position"
)

type testRepo struct {
	result domain.Position
	err    error
	saved  int
}

func (r *testRepo) Save(_ context.Context, id uuid.UUID, p domain.Position) (domain.Position, error) {
	r.saved++
	r.result = p
	r.result.TripID = id
	return r.result, r.err
}
func (r *testRepo) List(context.Context, uuid.UUID) ([]domain.Position, error) { return nil, r.err }

type testTx struct{}

func (testTx) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type testQueue struct{ canceled uuid.UUID }

func (q *testQueue) Cancel(id uuid.UUID) { q.canceled = id }

func TestSaveValidatesAndCancelsAfterSuccess(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	repo := &testRepo{}
	queue := &testQueue{}
	uc := New(repo, testTx{}, queue, func() time.Time { return now })
	id := uuid.New()
	p := domain.Position{Coordinates: domain.Coordinates{Latitude: 90, Longitude: 180}, RecordedAt: now}
	_, err := uc.Save(context.Background(), id, p)
	require.NoError(t, err)
	require.Equal(t, id, queue.canceled)
	for _, bad := range []domain.Position{
		{Coordinates: domain.Coordinates{Latitude: 90.1}, RecordedAt: now},
		{Coordinates: domain.Coordinates{Longitude: -180.1}, RecordedAt: now},
		{Coordinates: domain.Coordinates{}, RecordedAt: now.Add(time.Minute + time.Nanosecond)},
	} {
		_, err = uc.Save(context.Background(), id, bad)
		require.ErrorIs(t, err, domain.ErrInvalid)
	}
	repo.err = errors.New("insert failed")
	_, err = uc.Save(context.Background(), id, p)
	require.Error(t, err)
	require.Equal(t, id, queue.canceled)
}
