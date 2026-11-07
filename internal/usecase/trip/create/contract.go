//go:generate go run go.uber.org/mock/mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

package create

import (
	"context"

	"github.com/google/uuid"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	"github.com/bezbayana777/trip_go/internal/model"
)

type (
	tripRepository interface {
		CreateTrip(ctx context.Context, params model.CreateTripParams) error
		GetActiveDriverTrip(ctx context.Context, driverID uuid.UUID) (*trip.Trip, error)
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}
)
