package create_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	"github.com/bezbayana777/trip_go/internal/http/trip/create"
	"github.com/bezbayana777/trip_go/internal/model"
)

func TestHandler_Create_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUC := NewMockUsecase(ctrl)
	handler := create.New(mockUC)

	tripID := uuid.New()
	userID := uuid.New()
	driverID := uuid.New()

	reqDTO := model.CreateTripDTO{
		UserID: userID,
		DriverID: driverID,
		StartPoint: model.CoordinatesDTO{Latitude: 55.0, Longitude: 37.0},
		EndPoint: model.CoordinatesDTO{Latitude: 55.1, Longitude: 37.1},
		Price: 500,
	}

	expectedTrip := &trip.Trip{
		ID: tripID,
		UserID: userID,
		DriverID: driverID,
		Price: 500,
		Status: trip.StatusActive,
		StartedAt: time.Now().UTC(),
	}

	mockUC.EXPECT().
		Execute(gomock.Any(), reqDTO).
		Return(expectedTrip, nil)

	body, err := json.Marshal(reqDTO)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/trips", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var gotTrip trip.Trip
	err = json.Unmarshal(rec.Body.Bytes(), &gotTrip)
	require.NoError(t, err)
	assert.Equal(t, tripID, gotTrip.ID)
	assert.Equal(t, trip.StatusActive, gotTrip.Status)
}