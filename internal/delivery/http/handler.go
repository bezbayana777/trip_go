package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/bezbayana777/trip_go/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	generated "github.com/bezbayana777/trip_go/internal/generated"
)

const maxBodyBytes = 1 << 20

type Server struct {
	repo      *repository.TripRepository
	txManager repository.TxManager
	dbPool    *pgxpool.Pool
}

func NewServer(repo *repository.TripRepository, txManager repository.TxManager, dbPool *pgxpool.Pool) *Server {
	return &Server{repo: repo, txManager: txManager, dbPool: dbPool}
}

func toGeneratedTrip(t *repository.Trip) generated.Trip {
	return generated.Trip{
		Id:       t.ID,
		UserId:   t.UserID,
		DriverId: t.DriverID,
		StartPoint: generated.Coordinates{
			Latitude:  t.StartLatitude,
			Longitude: t.StartLongitude,
		},
		EndPoint: generated.Coordinates{
			Latitude:  t.EndLatitude,
			Longitude: t.EndLongitude,
		},
		Price:      t.Price,
		Status:     generated.TripStatus(t.Status),
		StartedAt:  t.StartedAt,
		FinishedAt: t.FinishedAt,
	}
}

var problemTitles = map[string]string{
	"invalid_request":      "Invalid request",
	"trip_not_found":       "Trip not found",
	"trip_completed":       "Trip completed",
	"driver_busy":          "Driver busy",
	"idempotency_conflict": "Idempotency conflict",
	"internal_error":       "Internal Server Error",
}

func problemType(code string) string {
	return "https://tripgo.example/problems/" + strings.ReplaceAll(code, "_", "-")
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, code, detail string) {
	var detailPtr *string
	if detail != "" {
		detailPtr = &detail
	}
	instance := r.URL.Path

	title, ok := problemTitles[code]
	if !ok {
		title = http.StatusText(status)
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(generated.Problem{
		Type:     problemType(code),
		Title:    title,
		Status:   int32(status),
		Detail:   detailPtr,
		Instance: &instance,
		Code:     code,
	})
}

func internalError(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("internal error: %s %s: %v", r.Method, r.URL.Path, err)
	writeProblem(w, r, http.StatusInternalServerError, "internal_error", "Internal server error")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func requireNonNull(raw map[string]json.RawMessage, prefix string, keys ...string) error {
	for _, k := range keys {
		v, ok := raw[k]
		if !ok || bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return fmt.Errorf("%s%s is required and must not be null", prefix, k)
		}
	}
	return nil
}

func validateRequiredFields(body []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	if err := requireNonNull(raw, "", "user_id", "driver_id", "price", "start_point", "end_point"); err != nil {
		return err
	}
	for _, point := range []string{"start_point", "end_point"} {
		var p map[string]json.RawMessage
		if err := json.Unmarshal(raw[point], &p); err != nil {
			return fmt.Errorf("%s must be an object", point)
		}
		if err := requireNonNull(p, point+".", "latitude", "longitude"); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) CreateTrip(w http.ResponseWriter, r *http.Request, params generated.CreateTripParams) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	var req generated.CreateTripJSONRequestBody
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeProblem(w, r, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if err := validateRequiredFields(body); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if req.UserId == uuid.Nil || req.DriverId == uuid.Nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_request", "user_id and driver_id cannot be empty UUIDs")
		return
	}

	latStart, lonStart := req.StartPoint.Latitude, req.StartPoint.Longitude
	latEnd, lonEnd := req.EndPoint.Latitude, req.EndPoint.Longitude
	if latStart < -90 || latStart > 90 || lonStart < -180 || lonStart > 180 ||
		latEnd < -90 || latEnd > 90 || lonEnd < -180 || lonEnd > 180 {
		writeProblem(w, r, http.StatusBadRequest, "invalid_request", "coordinates out of valid range")
		return
	}

	if req.Price < 0 {
		writeProblem(w, r, http.StatusBadRequest, "invalid_request", "price must be >= 0")
		return
	}

	tripID := uuid.New()
	dbParams := repository.CreateTripParams{
		ID:             tripID,
		UserID:         req.UserId,
		DriverID:       req.DriverId,
		StartLatitude:  latStart,
		StartLongitude: lonStart,
		EndLatitude:    latEnd,
		EndLongitude:   lonEnd,
		Price:          req.Price,
		Status:         "active",
		StartedAt:      time.Now(),
	}

	err = s.txManager.Do(r.Context(), func(ctx context.Context) error {
		return s.repo.CreateTrip(ctx, dbParams)
	})
	if err != nil {
		if errors.Is(err, repository.ErrDriverBusy) {
			writeProblem(w, r, http.StatusConflict, "driver_busy", "Driver already has an active trip")
			return
		}
		internalError(w, r, err)
		return
	}

	created := &repository.Trip{
		ID:             tripID,
		UserID:         req.UserId,
		DriverID:       req.DriverId,
		StartLatitude:  latStart,
		StartLongitude: lonStart,
		EndLatitude:    latEnd,
		EndLongitude:   lonEnd,
		Price:          req.Price,
		Status:         "active",
		StartedAt:      dbParams.StartedAt,
	}

	w.Header().Set("Location", "/api/v1/trips/"+tripID.String())
	writeJSON(w, http.StatusCreated, toGeneratedTrip(created))
}

func (s *Server) GetTrip(w http.ResponseWriter, r *http.Request, tripId generated.TripId) {
	trip, err := s.repo.GetTrip(r.Context(), tripId)
	if err != nil {
		if errors.Is(err, repository.ErrTripNotFound) {
			writeProblem(w, r, http.StatusNotFound, "trip_not_found", "Trip not found")
			return
		}
		internalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toGeneratedTrip(trip))
}

func (s *Server) FinishTrip(w http.ResponseWriter, r *http.Request, tripId generated.TripId) {
	var updated *repository.Trip

	err := s.txManager.Do(r.Context(), func(ctx context.Context) error {
		var repoErr error
		updated, repoErr = s.repo.FinishTrip(ctx, tripId)
		return repoErr
	})
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrTripNotFound):
			writeProblem(w, r, http.StatusNotFound, "trip_not_found", "Trip not found")
		case errors.Is(err, repository.ErrTripCompleted):
			writeProblem(w, r, http.StatusConflict, "trip_completed", "Trip already completed")
		default:
			internalError(w, r, err)
		}
		return
	}

	writeJSON(w, http.StatusOK, toGeneratedTrip(updated))
}

func (s *Server) ListTripPositions(w http.ResponseWriter, r *http.Request, tripId generated.TripId) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (s *Server) CreateTripPosition(w http.ResponseWriter, r *http.Request, tripId generated.TripId) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, generated.HealthResponse{Status: generated.Ok})
}

func (s *Server) Ready(w http.ResponseWriter, r *http.Request) {
	pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.dbPool.Ping(pingCtx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, generated.HealthResponse{Status: generated.Unavailable})
		return
	}
	writeJSON(w, http.StatusOK, generated.HealthResponse{Status: generated.Ok})
}