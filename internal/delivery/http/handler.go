package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/bezbayana777/trip_go/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	generated "github.com/bezbayana777/trip_go/internal/generated"
)

type Server struct {
	repo      *repository.TripRepository
	txManager repository.TxManager
	dbPool		*pgxpool.Pool
}

func NewServer(repo *repository.TripRepository, txManager repository.TxManager, dbPool *pgxpool.Pool) *Server {
	return &Server{
		repo:      repo,
		txManager: txManager,
		dbPool:    dbPool,
	}
}

func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"title":  title,
		"status": status,
		"detail": detail,
	})
}

func (s *Server) CreateTrip(w http.ResponseWriter, r *http.Request, params generated.CreateTripParams) {
	var req struct {
		UserID   uuid.UUID `json:"user_id"`
		DriverID uuid.UUID `json:"driver_id"`
		StartPoint struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"start_point"`
		EndPoint struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"end_point"`
		Price int64 `json:"price"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid Request Body", err.Error())
		return
	}

	if req.UserID == uuid.Nil || req.DriverID == uuid.Nil {
		writeProblem(w, http.StatusBadRequest, "Validation Error", "user_id and driver_id cannot be empty UUIDs")
		return
	}
	if req.Price < 0 {
		writeProblem(w, http.StatusBadRequest, "Validation Error", "price must be >= 0")
		return
	}

	tripID := uuid.New()
	dbParams := repository.CreateTripParams{
		ID:             tripID,
		UserID:         req.UserID,
		DriverID:       req.DriverID,
		StartLatitude:  req.StartPoint.Latitude,
		StartLongitude: req.StartPoint.Longitude,
		EndLatitude:    req.EndPoint.Latitude,
		EndLongitude:   req.EndPoint.Longitude,
		Price:          req.Price,
		Status:         "active",
		StartedAt:      time.Now(),
	}

	err := s.txManager.Do(r.Context(), func(ctx context.Context) error {
		return s.repo.CreateTrip(ctx, dbParams)
	})

	if err != nil {
		writeProblem(w, http.StatusConflict, "Driver Busy", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":        tripID,
		"user_id":   dbParams.UserID,
		"driver_id": dbParams.DriverID,
		"start_point": map[string]float64{
			"latitude":  dbParams.StartLatitude,
			"longitude": dbParams.StartLongitude,
		},
		"end_point": map[string]float64{
			"latitude":  dbParams.EndLatitude,
			"longitude": dbParams.EndLongitude,
		},
		"status":     dbParams.Status,
		"started_at": dbParams.StartedAt,
		"price":      dbParams.Price,
	})
}
func (s *Server) GetTrip(w http.ResponseWriter, r *http.Request, tripId generated.TripId) {
	trip, err := s.repo.GetTrip(r.Context(), tripId)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeProblem(w, http.StatusNotFound, "Not Found", "Trip not found")
			return
		}
		writeProblem(w, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(trip)
}

func (s *Server) FinishTrip(w http.ResponseWriter, r *http.Request, tripId generated.TripId) {
	err := s.txManager.Do(r.Context(), func(ctx context.Context) error {
		return s.repo.FinishTrip(ctx, tripId)
	})

	if err != nil {
		writeProblem(w, http.StatusConflict, "Conflict", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "completed",
	})
}

func (s *Server) ListTripPositions(w http.ResponseWriter, r *http.Request, tripId generated.TripId) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (s *Server) CreateTripPosition(w http.ResponseWriter, r *http.Request, tripId generated.TripId) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) Ready(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")

    pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
    defer cancel()

    if err := s.dbPool.Ping(pingCtx); err != nil {
        w.WriteHeader(http.StatusServiceUnavailable)
        _ = json.NewEncoder(w).Encode(map[string]string{"status": "database unavailable"})
        return
    }

    w.WriteHeader(http.StatusOK)
    _ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}