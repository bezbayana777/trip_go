package finish

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/bezbayana777/trip_go/internal/http/domainerror"
	"github.com/bezbayana777/trip_go/internal/http/transport"
)

type Handler struct {
	uc Usecase
}

func New(uc Usecase) *Handler {
	return &Handler{uc: uc}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	idStr := chi.URLParam(r, "tripId")
	tripID, err := uuid.Parse(idStr)
	if err != nil {
		writeProblem(w, r, &domainerror.ValidationError{Field: "trip_id"})
		return
	}

	tripRes, err := h.uc.Execute(ctx, tripID)
	if err != nil {
		status, problem := domainerror.Problem(err)
		problem.Instance = stringPtr(r.URL.Path)
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(problem)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(transport.Trip(tripRes))
}

func writeProblem(w http.ResponseWriter, r *http.Request, err error) {
	status, p := domainerror.Problem(err)
	p.Instance = stringPtr(r.URL.Path)
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}
func stringPtr(s string) *string { return &s }
