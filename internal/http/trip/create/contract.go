package create

import (
	"context"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	"github.com/bezbayana777/trip_go/internal/model"
)

//go:generate mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
type Usecase interface {
	Execute(ctx context.Context, dto model.CreateTripDTO) (*trip.Trip, error)
}