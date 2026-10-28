package finish_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/bezbayana777/trip_go/internal/http/trip/finish"
)

func TestHandler_Finish_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUC := NewMockUsecase(ctrl)
	handler := finish.New(mockUC)

	tripID := uuid.New()


	mockUC.EXPECT().
		Execute(gomock.Any(), tripID).
		Return(nil)

	r := chi.NewRouter()
	r.Post("/trips/{id}/finish", handler.ServeHTTP)

	req := httptest.NewRequest(http.MethodPost, "/trips/"+tripID.String()+"/finish", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}