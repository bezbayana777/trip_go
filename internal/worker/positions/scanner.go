package positions

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	domain "github.com/bezbayana777/trip_go/internal/domain/position"
)

type Repository interface {
	FindStale(context.Context, time.Duration) ([]domain.StaleTrip, error)
}
type Ticker interface {
	Chan() <-chan time.Time
	Stop()
}
type (
	Clock      interface{ NewTicker(time.Duration) Ticker }
	realClock  struct{}
	realTicker struct{ ticker *time.Ticker }
)

func (realClock) NewTicker(d time.Duration) Ticker { return realTicker{time.NewTicker(d)} }
func (t realTicker) Chan() <-chan time.Time        { return t.ticker.C }
func (t realTicker) Stop()                         { t.ticker.Stop() }

type Scanner struct {
	repo                 Repository
	pool                 *Pool
	staleAfter, interval time.Duration
	clock                Clock
	logger               *slog.Logger
}

func NewScanner(repo Repository, pool *Pool, staleAfter, interval time.Duration, logger *slog.Logger, clock Clock) (*Scanner, error) {
	if staleAfter <= 0 || interval <= 0 {
		return nil, fmt.Errorf("scanner durations must be positive")
	}
	if clock == nil {
		clock = realClock{}
	}
	return &Scanner{repo: repo, pool: pool, staleAfter: staleAfter, interval: interval, clock: clock, logger: logger}, nil
}

func (s *Scanner) Run(ctx context.Context) {
	ticker := s.clock.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.pool.StopScanner()
			return
		case <-ticker.Chan():
			if !s.pool.BeginScan() {
				return
			}
			err := s.scanOnce(ctx)
			s.pool.EndScan()
			if err != nil {
				if s.logger != nil {
					s.logger.ErrorContext(ctx, "stale trip scan failed", slog.String("error", err.Error()))
				}
				continue
			}
		}
	}
}

// ScanOnce performs one sequential scan; the ticker loop never starts a second
// scan until this function has returned.
func (s *Scanner) ScanOnce(ctx context.Context) error {
	if !s.pool.BeginScan() {
		return nil
	}
	defer s.pool.EndScan()
	return s.scanOnce(ctx)
}

func (s *Scanner) scanOnce(ctx context.Context) error {
	scanCtx, span := otel.Tracer("trip-service/positions").Start(ctx, "scan stale trips", trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()
	queryCtx, cancel := context.WithTimeout(scanCtx, s.interval)
	defer cancel()
	trips, err := s.repo.FindStale(queryCtx, s.staleAfter)
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("scan stale trips: %w", err)
	}
	for _, trip := range trips {
		if err := queryCtx.Err(); err != nil {
			return err
		}
		s.pool.EnqueueContext(scanCtx, trip)
	}
	return nil
}
