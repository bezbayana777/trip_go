//go:generate go run go.uber.org/mock/mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package get

import (
	"context"
	"github.com/bezbayana777/trip_go/internal/domain/trip"
	"github.com/google/uuid"
)

type tripRepository interface {
	GetTrip(ctx context.Context, id uuid.UUID) (*trip.Trip, error)
}