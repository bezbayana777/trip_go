package push

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRequestPositionUsesContractPathAndPayload(t *testing.T) {
	driverID, tripID := uuid.New(), uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/push" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: method=%s path=%s content-type=%s", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var body struct {
			RequestID   string            `json:"request_id"`
			RecipientID uuid.UUID         `json:"recipient_id"`
			Kind        string            `json:"kind"`
			Data        map[string]string `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode push body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if body.RequestID == "" || body.RecipientID != driverID || body.Kind != "REQUEST_POSITION" || body.Data["trip_id"] != tripID.String() {
			t.Errorf("unexpected push body: %+v", body)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := New(server.URL+"/", time.Second)
	require.NoError(t, client.RequestPosition(context.Background(), driverID, tripID))
}

func TestRequestPositionReturnsErrorsForClientAndServerStatuses(t *testing.T) {
	for _, test := range []struct {
		status  int
		message string
	}{
		{http.StatusNotFound, "client status 404"},
		{http.StatusServiceUnavailable, "server status 503"},
		{http.StatusOK, "unexpected status 200"},
	} {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(test.status) }))
			defer server.Close()
			client := New(server.URL, time.Second)
			err := client.RequestPosition(context.Background(), uuid.New(), uuid.New())
			require.ErrorContains(t, err, test.message)
		})
	}
}
