package finish_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	"github.com/bezbayana777/trip_go/internal/usecase/trip/finish"
)

func TestUsecase_Finish_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repoMock := NewMocktripRepository(ctrl)
	txMgr := NewMocktxManager(ctrl)

	uc := finish.New(repoMock, txMgr, nil, nil)

	tripID := uuid.New()

	repoMock.EXPECT().
		GetTripForUpdate(gomock.Any(), tripID).
		Return(&trip.Trip{ID: tripID, Status: trip.StatusActive}, nil)
	repoMock.EXPECT().SaveFinishedTrip(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, result *trip.Trip) error {
		assert.Equal(t, trip.StatusCompleted, result.Status)
		assert.NotNil(t, result.FinishedAt)
		return nil
	})
	txMgr.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) })

	result, err := uc.Execute(context.Background(), tripID)

	assert.NoError(t, err)
	assert.Equal(t, trip.StatusCompleted, result.Status)
}

func TestUsecase_Finish_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repoMock := NewMocktripRepository(ctrl)
	txMgr := NewMocktxManager(ctrl)

	uc := finish.New(repoMock, txMgr, nil, nil)

	tripID := uuid.New()
	expectedErr := errors.New("trip not found or already finished")

	// Ожидаем ошибку от репозитория
	repoMock.EXPECT().
		GetTripForUpdate(gomock.Any(), tripID).
		Return(nil, expectedErr)
	txMgr.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) })

	_, err := uc.Execute(context.Background(), tripID)

	assert.ErrorIs(t, err, expectedErr)
}

func TestUsecase_Finish_TransactionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	want := errors.New("commit failed")
	tx.EXPECT().Do(gomock.Any(), gomock.Any()).Return(want)
	uc := finish.New(repo, tx, nil, nil)
	_, err := uc.Execute(context.Background(), uuid.New())
	assert.ErrorIs(t, err, want)
}
