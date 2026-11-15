package http

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRouteFromPathUsesTemplates(t *testing.T) {
	require.Equal(t, "/api/v1/trips/{tripId}", routeFromPath(http.MethodGet, "/api/v1/trips/4fd3b646-17ce-4af7-9c32-9644bb2634ce"))
	require.Equal(t, "/api/v1/trips/{tripId}/finish", routeFromPath(http.MethodPost, "/api/v1/trips/4fd3b646-17ce-4af7-9c32-9644bb2634ce/finish"))
	require.Empty(t, routeFromPath(http.MethodGet, "/api/v1/unknown/123"))
}
