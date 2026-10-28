package get

import (
	"encoding/json"
	"net/http"

	"github.com/bezbayana777/trip_go/internal/http/domainerror"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Handler struct {
	uc Usecase
}

func New(uc Usecase) *Handler {
	return &Handler{uc: uc}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idStr := chi.URLParam(r, "id")
	tripID, err := uuid.Parse(idStr)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid trip id format"})
		return
	}

	tripRes, err := h.uc.Execute(ctx, tripID)
	if err != nil {
		status, problem := domainerror.Problem(err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(problem)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(tripRes)
}