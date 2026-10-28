package get

import (
	"context"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	"github.com/google/uuid"
)

//go:generate mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
type Usecase interface {
	Execute(ctx context.Context, id uuid.UUID) (*trip.Trip, error)
}