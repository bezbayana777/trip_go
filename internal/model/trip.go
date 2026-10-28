package model

import "github.com/google/uuid"

type CoordinatesDTO struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type CreateTripDTO struct {
	UserID     uuid.UUID      `json:"user_id"`
	DriverID   uuid.UUID      `json:"driver_id"`
	StartPoint CoordinatesDTO `json:"start_point"`
	EndPoint   CoordinatesDTO `json:"end_point"`
	Price      int64          `json:"price"`
}