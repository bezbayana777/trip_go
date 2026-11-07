package create_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

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
		UserID:     userID,
		DriverID:   driverID,
		StartPoint: model.CoordinatesDTO{Latitude: 55.0, Longitude: 37.0},
		EndPoint:   model.CoordinatesDTO{Latitude: 55.1, Longitude: 37.1},
		Price:      500,
	}

	expectedTrip := &trip.Trip{
		ID:        tripID,
		UserID:    userID,
		DriverID:  driverID,
		Price:     500,
		Status:    trip.StatusActive,
		StartedAt: time.Now().UTC(),
	}

	mockUC.EXPECT().
		Execute(gomock.Any(), reqDTO).
		Return(expectedTrip, nil)

	body, err := json.Marshal(reqDTO)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, "/api/v1/trips/"+tripID.String(), rec.Header().Get("Location"))
	var got map[string]json.RawMessage
	err = json.Unmarshal(rec.Body.Bytes(), &got)
	require.NoError(t, err)
	assert.Contains(t, got, "id")
	assert.Contains(t, got, "user_id")
	assert.Contains(t, got, "start_point")
	assert.NotContains(t, got, "ID")
	assert.NotContains(t, got, "StartPoint")
	assert.JSONEq(t, `"active"`, string(got["status"]))
}

func TestHandler_Create_Problems(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
	}{{"busy", trip.ErrDriverBusy, http.StatusConflict}, {"internal", errors.New("database password leaked"), http.StatusInternalServerError}} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMockUsecase(ctrl)
			handler := create.New(uc)
			dto := model.CreateTripDTO{UserID: uuid.MustParse("11111111-1111-4111-8111-111111111111"), DriverID: uuid.MustParse("22222222-2222-4222-8222-222222222222")}
			uc.EXPECT().Execute(gomock.Any(), dto).Return(nil, tt.err)
			body := `{"user_id":"11111111-1111-4111-8111-111111111111","driver_id":"22222222-2222-4222-8222-222222222222","start_point":{"latitude":0,"longitude":0},"end_point":{"latitude":0,"longitude":0},"price":0}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/", bytes.NewBufferString(body))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			assert.Equal(t, tt.status, rec.Code)
			assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
			assert.NotContains(t, rec.Body.String(), "database password leaked")
		})
	}
}

func TestHandler_Create_InvalidRequests(t *testing.T) {
	handler := create.New(nil)
	valid := `{"user_id":"11111111-1111-4111-8111-111111111111","driver_id":"22222222-2222-4222-8222-222222222222","start_point":{"latitude":0,"longitude":0},"end_point":{"latitude":0,"longitude":0},"price":0}`
	tests := []struct{ name, body string }{
		{"missing price", `{"user_id":"11111111-1111-4111-8111-111111111111","driver_id":"22222222-2222-4222-8222-222222222222","start_point":{"latitude":0,"longitude":0},"end_point":{"latitude":0,"longitude":0}}`},
		{"null price", `{"user_id":"11111111-1111-4111-8111-111111111111","driver_id":"22222222-2222-4222-8222-222222222222","start_point":{"latitude":0,"longitude":0},"end_point":{"latitude":0,"longitude":0},"price":null}`},
		{"empty uuid", `{"user_id":"00000000-0000-0000-0000-000000000000","driver_id":"22222222-2222-4222-8222-222222222222","start_point":{"latitude":0,"longitude":0},"end_point":{"latitude":0,"longitude":0},"price":0}`},
		{"out of range", `{"user_id":"11111111-1111-4111-8111-111111111111","driver_id":"22222222-2222-4222-8222-222222222222","start_point":{"latitude":91,"longitude":0},"end_point":{"latitude":0,"longitude":0},"price":0}`},
		{"negative price", `{"user_id":"11111111-1111-4111-8111-111111111111","driver_id":"22222222-2222-4222-8222-222222222222","start_point":{"latitude":0,"longitude":0},"end_point":{"latitude":0,"longitude":0},"price":-1}`},
		{"unknown field", valid[:len(valid)-1] + `,"unexpected":1}`},
		{"trailing json", valid + ` {}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/", bytes.NewBufferString(tt.body))
			handler.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
			assert.Contains(t, rec.Body.String(), `"code":"invalid_request"`)
		})
	}
}
