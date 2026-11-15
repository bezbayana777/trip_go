package transport

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	api "github.com/bezbayana777/trip_go/internal/generated"
)

func Trip(t *trip.Trip) api.Trip {
	result := api.Trip{
		Id: openapi_types.UUID(t.ID), UserId: openapi_types.UUID(t.UserID), DriverId: openapi_types.UUID(t.DriverID),
		StartPoint: api.Coordinates{Latitude: t.StartPoint.Latitude, Longitude: t.StartPoint.Longitude},
		EndPoint:   api.Coordinates{Latitude: t.EndPoint.Latitude, Longitude: t.EndPoint.Longitude},
		Price:      t.Price, Status: api.TripStatus(t.Status), StartedAt: t.StartedAt,
	}
	result.FinishedAt = t.FinishedAt
	result.LastPositionAt = t.LastPositionAt
	return result
}
