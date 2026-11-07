package create

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"

	"github.com/google/uuid"

	api "github.com/bezbayana777/trip_go/internal/generated"
	"github.com/bezbayana777/trip_go/internal/http/domainerror"
	"github.com/bezbayana777/trip_go/internal/http/transport"
	"github.com/bezbayana777/trip_go/internal/model"
)

type Handler struct {
	uc Usecase
}

func New(uc Usecase) *Handler {
	return &Handler{uc: uc}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var raw map[string]json.RawMessage
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&raw); err != nil || raw == nil {
		writeProblem(w, r, &domainerror.ValidationError{Field: "body"})
		return
	}
	for key := range raw {
		switch key {
		case "user_id", "driver_id", "start_point", "end_point", "price":
		default:
			writeProblem(w, r, &domainerror.ValidationError{Field: key})
			return
		}
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		writeProblem(w, r, &domainerror.ValidationError{Field: "body"})
		return
	}
	for _, key := range []string{"user_id", "driver_id", "start_point", "end_point", "price"} {
		if value, ok := raw[key]; !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			writeProblem(w, r, &domainerror.ValidationError{Field: key})
			return
		}
	}
	for _, key := range []string{"start_point", "end_point"} {
		var point map[string]json.RawMessage
		if json.Unmarshal(raw[key], &point) != nil {
			writeProblem(w, r, &domainerror.ValidationError{Field: key})
			return
		}
		for _, field := range []string{"latitude", "longitude"} {
			value, ok := point[field]
			if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				writeProblem(w, r, &domainerror.ValidationError{Field: key + "." + field})
				return
			}
		}
		for field := range point {
			if field != "latitude" && field != "longitude" {
				writeProblem(w, r, &domainerror.ValidationError{Field: key + "." + field})
				return
			}
		}
	}
	body, _ := json.Marshal(raw)
	var contract api.TripData
	if err := json.Unmarshal(body, &contract); err != nil {
		writeProblem(w, r, &domainerror.ValidationError{Field: "body"})
		return
	}
	req := model.CreateTripDTO{UserID: uuid.UUID(contract.UserId), DriverID: uuid.UUID(contract.DriverId), Price: contract.Price, StartPoint: model.CoordinatesDTO{Latitude: contract.StartPoint.Latitude, Longitude: contract.StartPoint.Longitude}, EndPoint: model.CoordinatesDTO{Latitude: contract.EndPoint.Latitude, Longitude: contract.EndPoint.Longitude}}
	if req.UserID == uuid.Nil {
		writeProblem(w, r, &domainerror.ValidationError{Field: "user_id"})
		return
	}
	if req.DriverID == uuid.Nil {
		writeProblem(w, r, &domainerror.ValidationError{Field: "driver_id"})
		return
	}
	if req.Price < 0 {
		writeProblem(w, r, &domainerror.ValidationError{Field: "price"})
		return
	}
	for name, point := range map[string]model.CoordinatesDTO{"start_point": req.StartPoint, "end_point": req.EndPoint} {
		if math.IsNaN(point.Latitude) || math.IsInf(point.Latitude, 0) || point.Latitude < -90 || point.Latitude > 90 {
			writeProblem(w, r, &domainerror.ValidationError{Field: name + ".latitude"})
			return
		}
		if math.IsNaN(point.Longitude) || math.IsInf(point.Longitude, 0) || point.Longitude < -180 || point.Longitude > 180 {
			writeProblem(w, r, &domainerror.ValidationError{Field: name + ".longitude"})
			return
		}
	}

	tripRes, err := h.uc.Execute(ctx, req)
	if err != nil {
		status, problem := domainerror.Problem(err)
		problem.Instance = stringPtr(r.URL.Path)
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(problem)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/api/v1/trips/"+tripRes.ID.String())
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(transport.Trip(tripRes))
}

func writeProblem(w http.ResponseWriter, r *http.Request, err error) {
	status, problem := domainerror.Problem(err)
	problem.Instance = stringPtr(r.URL.Path)
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem)
}
func stringPtr(s string) *string { return &s }
