package finish_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.uber.org/mock/gomock"

	"github.com/bezbayana777/trip_go/internal/infra/telemetry"
	"github.com/bezbayana777/trip_go/internal/usecase/trip/finish"
)

type dummyTxManager struct{}

func (dummyTxManager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

func TestUsecase_Finish_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repoMock := NewMocktripRepository(ctrl)
	txMgr := dummyTxManager{}

	uc := finish.New(repoMock, txMgr)

	tripID := uuid.New()

	mp := metric.NewMeterProvider()
  meter := mp.Meter("test")
  telemetry.GlobalMetrics.TripsCompleted, _ = meter.Int64Counter("tripgo.trips.completed")

	repoMock.EXPECT().
		FinishTrip(gomock.Any(), tripID).
		Return(nil)

	err := uc.Execute(context.Background(), tripID)

	assert.NoError(t, err)
}

func TestUsecase_Finish_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repoMock := NewMocktripRepository(ctrl)
	txMgr := dummyTxManager{}

	uc := finish.New(repoMock, txMgr)

	tripID := uuid.New()
	expectedErr := errors.New("trip not found or already finished")

	// Ожидаем ошибку от репозитория
	repoMock.EXPECT().
		FinishTrip(gomock.Any(), tripID).
		Return(expectedErr)

	err := uc.Execute(context.Background(), tripID)

	assert.ErrorIs(t, err, expectedErr)
}