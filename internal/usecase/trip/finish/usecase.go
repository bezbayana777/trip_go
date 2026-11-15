package finish

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
)

type (
	Usecase struct {
		repo      tripRepository
		txManager txManager
		logger    *slog.Logger
		metrics   metric.Int64Counter
	}
)

func New(repo tripRepository, txManager txManager, logger *slog.Logger, metrics metric.Int64Counter) *Usecase {
	return &Usecase{
		repo:      repo,
		txManager: txManager,
		logger:    logger,
		metrics:   metrics,
	}
}

func (uc *Usecase) Execute(ctx context.Context, tripID uuid.UUID) (*trip.Trip, error) {
	var completed *trip.Trip
	err := uc.txManager.Do(ctx, func(ctx context.Context) error {
		var err error
		completed, err = uc.repo.GetTripForUpdate(ctx, tripID)
		if err != nil {
			return err
		}
		if err := completed.Finish(time.Now().UTC()); err != nil {
			return err
		}
		return uc.repo.SaveFinishedTrip(ctx, completed)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to finish trip: %w", err)
	}

	if uc.metrics != nil {
		uc.metrics.Add(ctx, 1)
	}
	if uc.logger != nil {
		uc.logger.InfoContext(ctx, "trip completed", slog.String("trip_id", tripID.String()))
	}

	return completed, nil
}
