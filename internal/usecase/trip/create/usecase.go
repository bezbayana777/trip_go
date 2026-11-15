package create

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	"github.com/bezbayana777/trip_go/internal/model"
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

func (uc *Usecase) Execute(ctx context.Context, dto model.CreateTripDTO) (*trip.Trip, error) {
	tripID := uuid.New()
	now := time.Now()

	newTrip := trip.Trip{
		ID:       tripID,
		UserID:   dto.UserID,
		DriverID: dto.DriverID,
		StartPoint: trip.Coordinates{
			Latitude:  dto.StartPoint.Latitude,
			Longitude: dto.StartPoint.Longitude,
		},
		EndPoint: trip.Coordinates{
			Latitude:  dto.EndPoint.Latitude,
			Longitude: dto.EndPoint.Longitude,
		},
		Price:     dto.Price,
		Status:    trip.StatusActive,
		StartedAt: now,
	}

	dbParams := model.CreateTripParams{
		ID:         newTrip.ID,
		UserID:     newTrip.UserID,
		DriverID:   newTrip.DriverID,
		StartPoint: model.CoordinatesDTO{Latitude: newTrip.StartPoint.Latitude, Longitude: newTrip.StartPoint.Longitude},
		EndPoint:   model.CoordinatesDTO{Latitude: newTrip.EndPoint.Latitude, Longitude: newTrip.EndPoint.Longitude},
		Price:      newTrip.Price,
		Status:     string(newTrip.Status),
		StartedAt:  newTrip.StartedAt,
	}

	err := uc.txManager.Do(ctx, func(ctx context.Context) error {
		return uc.repo.CreateTrip(ctx, dbParams)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create trip: %w", err)
	}

	if uc.metrics != nil {
		uc.metrics.Add(ctx, 1)
	}
	if uc.logger != nil {
		uc.logger.InfoContext(ctx, "trip created", slog.String("trip_id", tripID.String()))
	}

	return &newTrip, nil
}
