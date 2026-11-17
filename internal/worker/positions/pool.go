package positions

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	domain "github.com/bezbayana777/trip_go/internal/domain/position"
	"github.com/bezbayana777/trip_go/internal/infra/telemetry"
)

type PushGateway interface {
	RequestPosition(context.Context, uuid.UUID, uuid.UUID) error
}

type PoolClock interface {
	Now() time.Time
	NewTimer(time.Duration) Timer
}

type Timer interface {
	C() <-chan time.Time
	Stop() bool
}

type (
	realPoolClock struct{}
	realTimer     struct{ timer *time.Timer }
)

func (realPoolClock) Now() time.Time                 { return time.Now() }
func (realPoolClock) NewTimer(d time.Duration) Timer { return realTimer{timer: time.NewTimer(d)} }
func (t realTimer) C() <-chan time.Time              { return t.timer.C }
func (t realTimer) Stop() bool                       { return t.timer.Stop() }

type Task struct {
	TripID, DriverID uuid.UUID
	TraceLink        trace.SpanContext
}

type Metrics struct {
	StaleChecks      metric.Int64Counter
	QueueSize        metric.Int64ObservableGauge
	ActiveWorkers    metric.Int64ObservableGauge
	PositionRequests metric.Int64Counter
	RequestDuration  metric.Float64Histogram
}

type Pool struct {
	queue                  chan Task
	mu                     sync.Mutex
	pending                map[uuid.UUID]struct{}
	inFlight               map[uuid.UUID]struct{}
	lastSent               map[uuid.UUID]time.Time
	push                   PushGateway
	workers                int
	timeout, checkInterval time.Duration
	logger                 *slog.Logger
	metrics                Metrics
	tracer                 trace.Tracer
	active                 int64
	activeMu               sync.Mutex
	ctx                    context.Context
	cancel                 context.CancelFunc
	wg                     sync.WaitGroup
	accepting              bool
	stopOnce               sync.Once
	stopped                chan struct{}
	clock                  PoolClock
	scanWG                 sync.WaitGroup
	scannerStopped         bool
}

func New(parent context.Context, workers, capacity int, timeout, checkInterval time.Duration, push PushGateway, logger *slog.Logger, metrics Metrics, tracer trace.Tracer, clocks ...PoolClock) (*Pool, error) {
	if workers < 1 || capacity < 1 || timeout <= 0 || checkInterval <= 0 || push == nil {
		return nil, fmt.Errorf("invalid position worker configuration")
	}
	clock := PoolClock(realPoolClock{})
	if len(clocks) > 0 && clocks[0] != nil {
		clock = clocks[0]
	}
	ctx, cancel := context.WithCancel(parent)
	p := &Pool{
		queue: make(chan Task, capacity), pending: make(map[uuid.UUID]struct{}),
		lastSent: make(map[uuid.UUID]time.Time), inFlight: make(map[uuid.UUID]struct{}), push: push, workers: workers,
		timeout: timeout, checkInterval: checkInterval, logger: logger, metrics: metrics,
		tracer: tracer, ctx: ctx, cancel: cancel, accepting: true,
		stopped: make(chan struct{}), clock: clock,
	}
	p.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go p.worker()
	}
	telemetry.SetPositionPoolProviders(p.QueueSize, p.ActiveWorkers)
	return p, nil
}

func (p *Pool) QueueSize() int64 { return int64(len(p.queue)) }

func (p *Pool) ActiveWorkers() int64 {
	p.activeMu.Lock()
	defer p.activeMu.Unlock()
	return p.active
}

func (p *Pool) Enqueue(t domain.StaleTrip) string {
	return p.EnqueueContext(p.ctx, t)
}

func (p *Pool) EnqueueContext(ctx context.Context, t domain.StaleTrip) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.scannerStopped {
		p.recordCheck("dropped")
		return "dropped"
	}
	if !p.accepting {
		p.recordCheck("dropped")
		return "dropped"
	}
	if _, ok := p.pending[t.ID]; ok {
		p.recordCheck("duplicate")
		return "duplicate"
	}
	if _, ok := p.inFlight[t.ID]; ok {
		p.recordCheck("duplicate")
		return "duplicate"
	}
	p.pending[t.ID] = struct{}{}
	select {
	case p.queue <- Task{TripID: t.ID, DriverID: t.DriverID, TraceLink: trace.SpanContextFromContext(ctx)}:
		p.recordCheck("queued")
		return "queued"
	default:
		delete(p.pending, t.ID)
		p.recordCheck("dropped")
		if p.logger != nil {
			p.logger.Warn("position request queue full", slog.String("trip_id", t.ID.String()))
		}
		return "dropped"
	}
}

func (p *Pool) Cancel(tripID uuid.UUID) {
	p.mu.Lock()
	delete(p.pending, tripID)
	delete(p.lastSent, tripID)
	if _, active := p.inFlight[tripID]; !active {
		delete(p.inFlight, tripID)
	}
	queued := len(p.queue)
	for i := 0; i < queued; i++ {
		task := <-p.queue
		if task.TripID != tripID {
			p.queue <- task
		}
	}
	p.mu.Unlock()
}

func (p *Pool) recordCheck(result string) {
	if p.metrics.StaleChecks != nil {
		p.metrics.StaleChecks.Add(p.ctx, 1, metric.WithAttributes(attribute.String("result", result)))
	}
}

func (p *Pool) worker() {
	defer p.wg.Done()
	for task := range p.queue {
		p.mu.Lock()
		_, pending := p.pending[task.TripID]
		lastSent := p.lastSent[task.TripID]
		if pending {
			p.inFlight[task.TripID] = struct{}{}
		}
		p.mu.Unlock()
		if !pending {
			continue
		}
		p.mu.Lock()
		_, pending = p.pending[task.TripID]
		if pending {
			p.inFlight[task.TripID] = struct{}{}
		}
		p.mu.Unlock()
		if !pending {
			continue
		}

		if !lastSent.IsZero() {
			wait := lastSent.Add(p.checkInterval).Sub(p.clock.Now())
			if wait > 0 {
				timer := p.clock.NewTimer(wait)
				select {
				case <-timer.C():
				case <-p.ctx.Done():
					timer.Stop()
					continue
				}
			}
		}

		// Recheck after the wait so a coordinate can cancel a delayed task.
		p.mu.Lock()
		_, pending = p.pending[task.TripID]
		_, canceled := p.inFlight[task.TripID]
		pending = pending && canceled
		p.mu.Unlock()
		if !pending {
			continue
		}

		p.activeMu.Lock()
		p.active++
		p.activeMu.Unlock()
		p.send(task)
		p.activeMu.Lock()
		p.active--
		p.activeMu.Unlock()
	}
}

func (p *Pool) send(task Task) {
	p.mu.Lock()
	_, stillPending := p.pending[task.TripID]
	_, queued := p.inFlight[task.TripID]
	p.mu.Unlock()
	if !stillPending || !queued {
		return
	}
	ctx, cancel := context.WithTimeout(p.ctx, p.timeout)
	defer cancel()
	started := p.clock.Now()
	var span trace.Span
	if p.tracer != nil {
		ctx, span = p.tracer.Start(ctx, "push request position")
		if task.TraceLink.IsValid() {
			span.AddLink(trace.Link{SpanContext: task.TraceLink})
		}
	}
	err := p.push.RequestPosition(ctx, task.DriverID, task.TripID)
	if span != nil {
		if err != nil {
			span.RecordError(err)
		}
		span.End()
	}
	result := "sent"
	if err != nil {
		result = "error"
	}
	if p.metrics.PositionRequests != nil {
		p.metrics.PositionRequests.Add(p.ctx, 1, metric.WithAttributes(attribute.String("result", result)))
	}
	if p.metrics.RequestDuration != nil {
		p.metrics.RequestDuration.Record(p.ctx, p.clock.Now().Sub(started).Seconds(), metric.WithAttributes(attribute.String("result", result)))
	}
	if err != nil && p.logger != nil {
		p.logger.Error("push position request failed", slog.String("trip_id", task.TripID.String()), slog.String("error", err.Error()))
	}
	p.mu.Lock()
	delete(p.pending, task.TripID)
	delete(p.inFlight, task.TripID)
	if err == nil && stillPending {
		p.lastSent[task.TripID] = p.clock.Now()
	}
	p.mu.Unlock()
}

func (p *Pool) StopScanner() {
	p.mu.Lock()
	p.accepting = false
	p.scannerStopped = true
	p.mu.Unlock()
}

func (p *Pool) BeginScan() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.accepting {
		return false
	}
	p.scanWG.Add(1)
	return true
}
func (p *Pool) EndScan() { p.scanWG.Done() }

func (p *Pool) Stop(ctx context.Context) error {
	p.StopScanner()
	p.stopOnce.Do(func() {
		go func() {
			p.scanWG.Wait()
			p.mu.Lock()
			close(p.queue)
			p.mu.Unlock()
			p.wg.Wait()
			close(p.stopped)
		}()
	})
	select {
	case <-p.stopped:
		p.cancel()
		return nil
	case <-ctx.Done():
		p.cancel()
		<-p.stopped
		return fmt.Errorf("position workers did not stop before shutdown deadline: %w", ctx.Err())
	}
}

func (p *Pool) Close() { p.cancel() }
