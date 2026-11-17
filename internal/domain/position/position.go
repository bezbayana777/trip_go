package position

import (
	"time"

	"github.com/google/uuid"

	tripdomain "github.com/bezbayana777/trip_go/internal/domain/trip"
)

var (
	ErrTripNotFound  = tripdomain.ErrTripNotFound
	ErrTripCompleted = tripdomain.ErrTripCompleted
	ErrInvalid       = tripdomain.ErrInvalidPosition
)

type Coordinates struct {
	Latitude  float64
	Longitude float64
}

type Position struct {
	ID          int64
	TripID      uuid.UUID
	Coordinates Coordinates
	RecordedAt  time.Time
}

type StaleTrip struct {
	ID       uuid.UUID
	DriverID uuid.UUID
}
