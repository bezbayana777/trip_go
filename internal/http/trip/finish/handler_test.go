package finish_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	"github.com/bezbayana777/trip_go/internal/http/trip/finish"
)

func TestHandler_Finish_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUC := NewMockUsecase(ctrl)
	handler := finish.New(mockUC)

	tripID := uuid.New()

	finished := time.Now().UTC()
	mockUC.EXPECT().
		Execute(gomock.Any(), tripID).
		Return(&trip.Trip{ID: tripID, Status: trip.StatusCompleted, FinishedAt: &finished}, nil)

	r := chi.NewRouter()
	r.Post("/trips/{tripId}/finish", handler.ServeHTTP)

	req := httptest.NewRequest(http.MethodPost, "/trips/"+tripID.String()+"/finish", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"status":"completed"`)
}

func TestHandler_Finish_Problems(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
	}{{"completed", trip.ErrTripCompleted, http.StatusConflict}, {"internal", errors.New("secret db detail"), http.StatusInternalServerError}} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockUsecase(ctrl)
			h := finish.New(uc)
			id := uuid.New()
			uc.EXPECT().Execute(gomock.Any(), id).Return(nil, tt.err)
			r := chi.NewRouter()
			r.Post("/trips/{tripId}/finish", h.ServeHTTP)
			req := httptest.NewRequest(http.MethodPost, "/trips/"+id.String()+"/finish", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			assert.Equal(t, tt.status, rec.Code)
			assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
			assert.NotContains(t, rec.Body.String(), "secret db detail")
		})
	}
}

func TestHandler_Finish_InvalidIDUsesProblemDetails(t *testing.T) {
	r := chi.NewRouter()
	r.Post("/trips/{tripId}/finish", finish.New(nil).ServeHTTP)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/trips/not-a-uuid/finish", nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Body.String(), `"instance":"/trips/not-a-uuid/finish"`)
}
