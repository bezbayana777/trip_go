package finish

import (
	"context"
	"fmt"
	"log/slog"
	"github.com/bezbayana777/trip_go/internal/infra/telemetry"
	"github.com/google/uuid"
)

type (
	Usecase struct {
		repo      tripRepository
		txManager txManager
	}
)

func New(repo tripRepository, txManager txManager) *Usecase {
	return &Usecase{
		repo:      repo,
		txManager: txManager,
	}
}

func (uc *Usecase) Execute(ctx context.Context, tripID uuid.UUID) error {
	err := uc.txManager.Do(ctx, func(ctx context.Context) error {
		return uc.repo.FinishTrip(ctx, tripID)
	})

	if err != nil {
		return fmt.Errorf("failed to finish trip: %w", err)
	}

	telemetry.GlobalMetrics.TripsCompleted.Add(ctx, 1)
	slog.Info("trip completed", append(telemetry.LogWithContext(ctx), slog.String("trip_id", tripID.String()))...)

	return nil
}