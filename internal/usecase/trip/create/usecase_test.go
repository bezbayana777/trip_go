package create_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	"github.com/bezbayana777/trip_go/internal/model"
	"github.com/bezbayana777/trip_go/internal/usecase/trip/create"
)

func TestUsecase_Create_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repoMock := NewMocktripRepository(ctrl)
	txMgr := NewMocktxManager(ctrl)

	uc := create.New(repoMock, txMgr, nil, nil)

	userID := uuid.New()
	driverID := uuid.New()

	repoMock.EXPECT().
		CreateTrip(gomock.Any(), gomock.Any()).
		Return(nil)
	txMgr.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) })

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

func TestUsecase_Create_RepositoryError(t *testing.T) {
	for _, want := range []error{trip.ErrDriverBusy, errors.New("repository unavailable")} {
		t.Run(want.Error(), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			tx := NewMocktxManager(ctrl)
			want := trip.ErrDriverBusy
			repo.EXPECT().CreateTrip(gomock.Any(), gomock.Any()).Return(want)
			tx.EXPECT().Do(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) })
			uc := create.New(repo, tx, nil, nil)
			_, err := uc.Execute(context.Background(), model.CreateTripDTO{UserID: uuid.New(), DriverID: uuid.New()})
			assert.ErrorIs(t, err, want)
		})
	}
}

func TestUsecase_Create_TransactionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	tx := NewMocktxManager(ctrl)
	want := assert.AnError
	tx.EXPECT().Do(gomock.Any(), gomock.Any()).Return(want)
	uc := create.New(repo, tx, nil, nil)
	_, err := uc.Execute(context.Background(), model.CreateTripDTO{UserID: uuid.New(), DriverID: uuid.New()})
	assert.ErrorIs(t, err, want)
}
