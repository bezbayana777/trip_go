//go:generate mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package finish

import (
	"context"

	"github.com/google/uuid"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
)

type (
	tripRepository interface {
		GetTripForUpdate(ctx context.Context, id uuid.UUID) (*trip.Trip, error)
		SaveFinishedTrip(ctx context.Context, trip *trip.Trip) error
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}
)
