package position

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	domain "github.com/bezbayana777/trip_go/internal/domain/position"
)

type Usecase struct {
	repo  Repository
	tx    TxManager
	queue Queue
	now   func() time.Time
}

func New(repo Repository, tx TxManager, queue Queue, now func() time.Time) *Usecase {
	if now == nil {
		now = time.Now
	}
	return &Usecase{repo: repo, tx: tx, queue: queue, now: now}
}

func (u *Usecase) Save(ctx context.Context, tripID uuid.UUID, p domain.Position) (domain.Position, error) {
	if math.IsNaN(p.Coordinates.Latitude) || math.IsInf(p.Coordinates.Latitude, 0) || p.Coordinates.Latitude < -90 || p.Coordinates.Latitude > 90 {
		return domain.Position{}, fmt.Errorf("latitude: %w", domain.ErrInvalid)
	}
	if math.IsNaN(p.Coordinates.Longitude) || math.IsInf(p.Coordinates.Longitude, 0) || p.Coordinates.Longitude < -180 || p.Coordinates.Longitude > 180 {
		return domain.Position{}, fmt.Errorf("longitude: %w", domain.ErrInvalid)
	}
	if p.RecordedAt.IsZero() || p.RecordedAt.After(u.now().Add(time.Minute)) {
		return domain.Position{}, fmt.Errorf("recorded_at: %w", domain.ErrInvalid)
	}
	var saved domain.Position
	err := u.tx.Do(ctx, func(txctx context.Context) error { var e error; saved, e = u.repo.Save(txctx, tripID, p); return e })
	if err != nil {
		return domain.Position{}, fmt.Errorf("save position: %w", err)
	}
	u.queue.Cancel(tripID)
	return saved, nil
}

func (u *Usecase) List(ctx context.Context, tripID uuid.UUID) ([]domain.Position, error) {
	positions, err := u.repo.List(ctx, tripID)
	if err != nil {
		return nil, fmt.Errorf("get trip positions: %w", err)
	}
	return positions, nil
}
