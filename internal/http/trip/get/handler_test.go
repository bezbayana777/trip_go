package get_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	"github.com/bezbayana777/trip_go/internal/http/trip/get"
)

func TestHandler_Get_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUC := NewMockUsecase(ctrl)
	handler := get.New(mockUC)

	tripID := uuid.New()
	expectedTrip := &trip.Trip{
		ID:        tripID,
		UserID:    uuid.New(),
		DriverID:  uuid.New(),
		Price:     1000,
		Status:    trip.StatusActive,
		StartedAt: time.Now().UTC(),
	}

	mockUC.EXPECT().
		Execute(gomock.Any(), tripID).
		Return(expectedTrip, nil)

	r := chi.NewRouter()
	r.Get("/trips/{tripId}", handler.ServeHTTP)

	req := httptest.NewRequest(http.MethodGet, "/trips/"+tripID.String(), nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var gotTrip map[string]json.RawMessage
	err := json.Unmarshal(rec.Body.Bytes(), &gotTrip)
	require.NoError(t, err)
	assert.Contains(t, gotTrip, "id")
	assert.Contains(t, gotTrip, "user_id")
	assert.Contains(t, gotTrip, "start_point")
	assert.NotContains(t, gotTrip, "ID")
}

func TestHandler_Get_Problems(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
	}{{"not found", trip.ErrTripNotFound, http.StatusNotFound}, {"internal", errors.New("secret db detail"), http.StatusInternalServerError}} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockUsecase(ctrl)
			h := get.New(uc)
			id := uuid.New()
			uc.EXPECT().Execute(gomock.Any(), id).Return(nil, tt.err)
			r := chi.NewRouter()
			r.Get("/trips/{tripId}", h.ServeHTTP)
			req := httptest.NewRequest(http.MethodGet, "/trips/"+id.String(), nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			assert.Equal(t, tt.status, rec.Code)
			assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
			assert.NotContains(t, rec.Body.String(), "secret db detail")
		})
	}
}

func TestHandler_Get_InvalidIDUsesProblemDetails(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/trips/{tripId}", get.New(nil).ServeHTTP)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/trips/not-a-uuid", nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Body.String(), `"instance":"/trips/not-a-uuid"`)
}
