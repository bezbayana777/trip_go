package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Client struct {
	endpoint string
	http     *http.Client
}

func New(endpoint string, timeout time.Duration) *Client {
	return &Client{endpoint: strings.TrimRight(endpoint, "/") + "/api/v1/push", http: &http.Client{Timeout: timeout}}
}

func (c *Client) CloseIdleConnections() { c.http.CloseIdleConnections() }

func (c *Client) RequestPosition(ctx context.Context, driverID, tripID uuid.UUID) error {
	body, err := json.Marshal(map[string]any{
		"request_id": uuid.NewString(), "kind": "REQUEST_POSITION", "recipient_id": driverID,
		"data": map[string]string{"trip_id": tripID.String()},
	})
	if err != nil {
		return fmt.Errorf("marshal push request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create push request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send push request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 500 {
		return fmt.Errorf("push service returned server status %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("push service returned client status %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("push service returned unexpected status %d", resp.StatusCode)
	}
	return nil
}
