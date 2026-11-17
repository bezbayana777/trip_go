package positions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	domain "github.com/bezbayana777/trip_go/internal/domain/position"
)

type pushFunc func(context.Context, uuid.UUID, uuid.UUID) error

func (f pushFunc) RequestPosition(ctx context.Context, d, t uuid.UUID) error { return f(ctx, d, t) }

func TestQueueDeduplicatesAndDropsWithoutBlocking(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan uuid.UUID, 1)
	release := make(chan struct{})
	push := pushFunc(func(_ context.Context, _, tripID uuid.UUID) error {
		started <- tripID
		<-release
		return nil
	})
	p, err := New(ctx, 1, 1, time.Second, time.Millisecond, push, nil, Metrics{}, nil)
	require.NoError(t, err)
	first := domain.StaleTrip{ID: uuid.New(), DriverID: uuid.New()}
	second := domain.StaleTrip{ID: uuid.New(), DriverID: uuid.New()}
	require.Equal(t, "queued", p.Enqueue(first))
	require.Equal(t, "duplicate", p.Enqueue(first))
	select {
	case id := <-started:
		require.Equal(t, first.ID, id)
	case <-time.After(time.Second):
		t.Fatal("worker did not start first push")
	}
	require.Equal(t, "queued", p.Enqueue(second))
	third := domain.StaleTrip{ID: uuid.New(), DriverID: uuid.New()}
	require.Equal(t, "dropped", p.Enqueue(third))
	close(release)
	p.StopScanner()
	stopCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	require.NoError(t, p.Stop(stopCtx))
}

func TestCoordinateCancelsQueuedTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan uuid.UUID, 2)
	release := make(chan struct{})
	p, err := New(ctx, 1, 2, time.Second, time.Millisecond, pushFunc(func(_ context.Context, _, tripID uuid.UUID) error {
		started <- tripID
		<-release
		return nil
	}), nil, Metrics{}, nil)
	require.NoError(t, err)
	first := domain.StaleTrip{ID: uuid.New(), DriverID: uuid.New()}
	canceled := domain.StaleTrip{ID: uuid.New(), DriverID: uuid.New()}
	require.Equal(t, "queued", p.Enqueue(first))
	select {
	case id := <-started:
		require.Equal(t, first.ID, id)
	case <-time.After(time.Second):
		t.Fatal("worker did not start first push")
	}
	require.Equal(t, "queued", p.Enqueue(canceled))
	p.Cancel(canceled.ID)
	close(release)
	p.StopScanner()
	stopCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	require.NoError(t, p.Stop(stopCtx))
	select {
	case id := <-started:
		require.NotEqual(t, canceled.ID, id)
	default:
	}
}

func TestPoolRunsConfiguredNumberOfWorkers(t *testing.T) {
	const workers = 3
	started := make(chan uuid.UUID, workers)
	release := make(chan struct{}, workers)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := New(ctx, workers, workers, time.Second, time.Second, pushFunc(func(_ context.Context, _, tripID uuid.UUID) error {
		started <- tripID
		<-release
		return nil
	}), nil, Metrics{}, nil)
	require.NoError(t, err)
	for range workers {
		require.Equal(t, "queued", p.Enqueue(domain.StaleTrip{ID: uuid.New(), DriverID: uuid.New()}))
	}
	seen := make(map[uuid.UUID]struct{}, workers)
	for range workers {
		select {
		case id := <-started:
			seen[id] = struct{}{}
		case <-time.After(time.Second):
			t.Fatal("not all configured workers started a task")
		}
	}
	require.Len(t, seen, workers)
	for range workers {
		release <- struct{}{}
	}
	p.StopScanner()
	stopCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	require.NoError(t, p.Stop(stopCtx))
}

func TestPushErrorDoesNotStopNextTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := make(chan uuid.UUID, 2)
	p, err := New(ctx, 1, 2, time.Second, time.Second, pushFunc(func(_ context.Context, _, tripID uuid.UUID) error {
		called <- tripID
		if len(called) == 1 {
			return errors.New("temporary push failure")
		}
		return nil
	}), nil, Metrics{}, nil)
	require.NoError(t, err)
	first, second := uuid.New(), uuid.New()
	require.Equal(t, "queued", p.Enqueue(domain.StaleTrip{ID: first, DriverID: uuid.New()}))
	require.Equal(t, "queued", p.Enqueue(domain.StaleTrip{ID: second, DriverID: uuid.New()}))
	for _, expected := range []uuid.UUID{first, second} {
		select {
		case got := <-called:
			require.Equal(t, expected, got)
		case <-time.After(time.Second):
			t.Fatalf("worker did not continue to task %s", expected)
		}
	}
	p.StopScanner()
	stopCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	require.NoError(t, p.Stop(stopCtx))
}
