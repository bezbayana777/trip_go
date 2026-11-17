package positions

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	domain "github.com/bezbayana777/trip_go/internal/domain/position"
)

type staleRepo struct {
	trips  []domain.StaleTrip
	calls  int
	called chan struct{}
}

func (r *staleRepo) FindStale(context.Context, time.Duration) ([]domain.StaleTrip, error) {
	r.calls++
	if r.called != nil {
		r.called <- struct{}{}
	}
	return r.trips, nil
}

type manualTicker struct{ ticks chan time.Time }

func (t *manualTicker) Chan() <-chan time.Time { return t.ticks }
func (t *manualTicker) Stop()                  {}

type manualClock struct{ ticker *manualTicker }

func (c manualClock) NewTicker(d time.Duration) Ticker { return c.ticker }

func TestScanOnceQueriesAndQueuesStaleTrips(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan uuid.UUID, 2)
	release := make(chan struct{}, 2)
	push := pushFunc(func(_ context.Context, _ uuid.UUID, tripID uuid.UUID) error {
		started <- tripID
		<-release
		return nil
	})
	pool, err := New(ctx, 1, 4, time.Second, time.Hour, push, nil, Metrics{}, nil)
	require.NoError(t, err)
	first := domain.StaleTrip{ID: uuid.New(), DriverID: uuid.New()}
	second := domain.StaleTrip{ID: uuid.New(), DriverID: uuid.New()}
	repo := &staleRepo{trips: []domain.StaleTrip{first, second}}
	scanner, err := NewScanner(repo, pool, time.Minute, time.Second, nil, nil)
	require.NoError(t, err)
	require.NoError(t, scanner.ScanOnce(ctx))
	require.Equal(t, 1, repo.calls)
	select {
	case id := <-started:
		require.Equal(t, first.ID, id)
	case <-time.After(time.Second):
		t.Fatal("worker did not start the first stale trip")
	}
	require.EqualValues(t, 1, pool.QueueSize())
	release <- struct{}{}
	select {
	case id := <-started:
		require.Equal(t, second.ID, id)
	case <-time.After(time.Second):
		t.Fatal("worker did not start the second stale trip")
	}
	release <- struct{}{}
	pool.StopScanner()
	stopCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	require.NoError(t, pool.Stop(stopCtx))
}

func TestRunScansOnlyAfterTickerTickAndStopsOnContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ticker := &manualTicker{ticks: make(chan time.Time)}
	pool, err := New(ctx, 1, 1, time.Second, time.Second, pushFunc(func(context.Context, uuid.UUID, uuid.UUID) error { return nil }), nil, Metrics{}, nil)
	require.NoError(t, err)
	repo := &staleRepo{called: make(chan struct{}, 1)}
	interval := 17 * time.Second
	scanner, err := NewScanner(repo, pool, time.Minute, interval, nil, manualClock{ticker: ticker})
	require.NoError(t, err)
	done := make(chan struct{})
	go func() { defer close(done); scanner.Run(ctx) }()
	select {
	case <-repo.called:
		t.Fatal("scanner ran before receiving a tick")
	default:
	}
	ticker.ticks <- time.Now()
	select {
	case <-repo.called:
	case <-time.After(time.Second):
		t.Fatal("scanner did not run after a tick")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scanner did not stop after context cancellation")
	}
	stopCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	require.NoError(t, pool.Stop(stopCtx))
}
