package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	brainPlanPath       = "/v1/plan"
	brainDefaultCap     = 200
	brainDefaultTimeout = 1500 * time.Millisecond
)

// BrainClient talks to the local Python planner sidecar. The sidecar is not a
// tool executor; it only returns a schema-first action plan for Go to validate.
type BrainClient struct {
	baseURL string
	client  *http.Client
}

func NewBrainClient(baseURL string, timeout time.Duration) *BrainClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if timeout <= 0 {
		timeout = brainDefaultTimeout
	}
	return &BrainClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *BrainClient) Available() bool {
	return c != nil && strings.TrimSpace(c.baseURL) != ""
}

func (c *BrainClient) Plan(ctx context.Context, req BrainPlanRequest) (*BrainPlanResponse, error) {
	if !c.Available() {
		return nil, errors.New("agent brain url is empty")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+brainPlanPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("agent brain HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out BrainPlanResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}
