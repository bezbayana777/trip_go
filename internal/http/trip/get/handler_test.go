package get_test

import (
	"encoding/json"
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
	r.Get("/trips/{id}", handler.ServeHTTP)

	req := httptest.NewRequest(http.MethodGet, "/trips/"+tripID.String(), nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var gotTrip trip.Trip
	err := json.Unmarshal(rec.Body.Bytes(), &gotTrip)
	require.NoError(t, err)
	assert.Equal(t, tripID, gotTrip.ID)
}