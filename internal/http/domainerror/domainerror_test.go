package domainerror_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	"github.com/bezbayana777/trip_go/internal/http/domainerror"
)

func TestProblem(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"busy", trip.ErrDriverBusy, http.StatusConflict, "driver_busy"},
		{"completed", trip.ErrTripCompleted, http.StatusConflict, "trip_completed"},
		{"not found", trip.ErrTripNotFound, http.StatusNotFound, "trip_not_found"},
		{"validation", &domainerror.ValidationError{Field: "price"}, http.StatusBadRequest, "invalid_request"},
		{"internal", errors.New("database secret"), http.StatusInternalServerError, "internal_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, p := domainerror.Problem(tt.err)
			require.Equal(t, tt.status, status)
			require.Equal(t, tt.status, int(p.Status))
			require.Equal(t, tt.code, p.Code)
			if p.Detail != nil {
				require.NotContains(t, *p.Detail, "database secret")
			}
		})
	}
}
