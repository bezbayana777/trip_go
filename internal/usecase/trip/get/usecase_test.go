package get_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	"github.com/bezbayana777/trip_go/internal/usecase/trip/get"
)

func TestUsecase_Get_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repoMock := NewMocktripRepository(ctrl)
	uc := get.New(repoMock)

	tripID := uuid.New()
	expectedTrip := &trip.Trip{
		ID:     tripID,
		Status: trip.StatusActive,
	}

	// Ожидаем вызов получения поездки по ID (проверьте имя метода в contract.go вашего get юзкейса: GetTrip или GetTripByID)
	repoMock.EXPECT().
		GetTrip(gomock.Any(), tripID).
		Return(expectedTrip, nil)

	tr, err := uc.Execute(context.Background(), tripID)

	assert.NoError(t, err)
	assert.NotNil(t, tr)
	assert.Equal(t, tripID, tr.ID)
	assert.Equal(t, trip.StatusActive, tr.Status)
}

func TestUsecase_Get_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repoMock := NewMocktripRepository(ctrl)
	uc := get.New(repoMock)

	tripID := uuid.New()
	expectedErr := errors.New("trip not found")

	repoMock.EXPECT().
		GetTrip(gomock.Any(), tripID).
		Return(nil, expectedErr)

	tr, err := uc.Execute(context.Background(), tripID)

	assert.Error(t, err)
	assert.Nil(t, tr)
}