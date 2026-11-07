package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/bezbayana777/trip_go/internal/domain/trip"
	triphttp "github.com/bezbayana777/trip_go/internal/http"
	createHandler "github.com/bezbayana777/trip_go/internal/http/trip/create"
	finishHandler "github.com/bezbayana777/trip_go/internal/http/trip/finish"
	getHandler "github.com/bezbayana777/trip_go/internal/http/trip/get"
)

type panicGet struct{}

func (panicGet) Execute(context.Context, uuid.UUID) (*trip.Trip, error) { panic("test panic") }
func TestHealthAndReadiness(t *testing.T) {
	r := triphttp.NewRouter(triphttp.RouterDeps{CreateHandler: createHandler.New(nil), GetHandler: getHandler.New(nil), FinishHandler: finishHandler.New(nil)})
	for _, tc := range []struct {
		path   string
		status int
		body   string
	}{{"/health", http.StatusOK, `"status":"ok"`}, {"/ready", http.StatusServiceUnavailable, `"status":"unavailable"`}} {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			require.Equal(t, tc.status, rec.Code)
			require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			require.Contains(t, rec.Body.String(), tc.body)
		})
	}
}

func TestPanicReturnsProblemJSON(t *testing.T) {
	r := triphttp.NewRouter(triphttp.RouterDeps{CreateHandler: createHandler.New(nil), GetHandler: getHandler.New(panicGet{}), FinishHandler: finishHandler.New(nil)})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/trips/11111111-1111-4111-8111-111111111111", nil))
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	require.Contains(t, rec.Body.String(), `"code":"internal_error"`)
}
