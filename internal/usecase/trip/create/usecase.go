package create

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	"github.com/bezbayana777/trip_go/internal/infra/telemetry"
	"github.com/bezbayana777/trip_go/internal/model"
	repoTrip "github.com/bezbayana777/trip_go/internal/repository/trip"
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
	
	dbParams := repoTrip.CreateTripParams{
		ID:             newTrip.ID,
		UserID:         newTrip.UserID,
		DriverID:       newTrip.DriverID,
		StartLatitude:  newTrip.StartPoint.Latitude,
		StartLongitude: newTrip.StartPoint.Longitude,
		EndLatitude:    newTrip.EndPoint.Latitude,
		EndLongitude:   newTrip.EndPoint.Longitude,
		Price:          newTrip.Price,
		Status:         string(newTrip.Status),
		StartedAt:      newTrip.StartedAt,
	}

	err := uc.txManager.Do(ctx, func(ctx context.Context) error {
		return uc.repo.CreateTrip(ctx, dbParams)
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create trip: %w", err)
	}

	telemetry.GlobalMetrics.TripsCreated.Add(ctx, 1)
	slog.Info("trip created", append(telemetry.LogWithContext(ctx), slog.String("trip_id", tripID.String()))...)

	return &newTrip, nil
}