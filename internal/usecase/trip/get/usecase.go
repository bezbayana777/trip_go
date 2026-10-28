package get

import (
	"context"
	"fmt"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	"github.com/google/uuid"
)

type (

	Usecase struct {
		repo tripRepository
	}
)

func New(repo tripRepository) *Usecase {
	return &Usecase{repo: repo}
}

func (uc *Usecase) Execute(ctx context.Context, tripID uuid.UUID) (*trip.Trip, error) {
	t, err := uc.repo.GetTrip(ctx, tripID)
	if err != nil {
		return nil, fmt.Errorf("failed to get trip: %w", err)
	}
	return t, nil
}