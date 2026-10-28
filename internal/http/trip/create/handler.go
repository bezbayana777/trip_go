package create

import (
	"encoding/json"
	"net/http"

	"github.com/bezbayana777/trip_go/internal/http/domainerror"
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

	var req model.CreateTripDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	tripRes, err := h.uc.Execute(ctx, req)
	if err != nil {
		status, problem := domainerror.Problem(err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(problem)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(tripRes)
}