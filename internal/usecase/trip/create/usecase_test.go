package create_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.uber.org/mock/gomock"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	"github.com/bezbayana777/trip_go/internal/infra/telemetry"
	"github.com/bezbayana777/trip_go/internal/model"
	"github.com/bezbayana777/trip_go/internal/usecase/trip/create"
)

type dummyTxManager struct{}

func (dummyTxManager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func TestUsecase_Create_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repoMock := NewMocktripRepository(ctrl)
	txMgr := dummyTxManager{}

	uc := create.New(repoMock, txMgr)

	userID := uuid.New()
	driverID := uuid.New()

	mp := metric.NewMeterProvider()
  meter := mp.Meter("test")
  telemetry.GlobalMetrics.TripsCreated, _ = meter.Int64Counter("tripgo.trips.created")

	repoMock.EXPECT().
		CreateTrip(gomock.Any(), gomock.Any()).
		Return(nil)

	dto := model.CreateTripDTO{
		UserID:   userID,
		DriverID: driverID,
		StartPoint: model.CoordinatesDTO{
			Latitude:  59.9,
			Longitude: 30.3,
		},
		EndPoint: model.CoordinatesDTO{ 
			Latitude:  59.8,
			Longitude: 30.4,
		},
		Price: 1500,
	}

	tr, err := uc.Execute(context.Background(), dto)

	assert.NoError(t, err)
	assert.NotNil(t, tr)
	assert.Equal(t, trip.StatusActive, tr.Status)
	assert.Equal(t, userID, tr.UserID)
	assert.Equal(t, driverID, tr.DriverID)
	assert.Equal(t, int64(1500), tr.Price)
}