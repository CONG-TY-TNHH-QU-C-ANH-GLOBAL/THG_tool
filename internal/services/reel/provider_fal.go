package reel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// falVideo renders a silent text-to-video clip via FAL's queue API and downloads it to
// destPath. Flow: submit {prompt} → {request_id, status_url, response_url}; poll status_url
// until {status:"COMPLETED"}; GET response_url → {video:{url}}; download. Any error degrades
// the shot honestly (caller self-reports "failed").
func falVideo(ctx context.Context, client *http.Client, apiKey, model, prompt, destPath string) error {
	body, _ := json.Marshal(map[string]any{"prompt": prompt})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://queue.fal.run/"+model, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Key "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("fal submit: status %d: %s", resp.StatusCode, sliceStr(string(raw), 200))
	}
	var sub struct {
		StatusURL   string `json:"status_url"`
		ResponseURL string `json:"response_url"`
	}
	if err := json.Unmarshal(raw, &sub); err != nil {
		return fmt.Errorf("fal submit decode: %w", err)
	}
	if sub.StatusURL == "" || sub.ResponseURL == "" {
		return fmt.Errorf("fal submit: missing status/response url")
	}
	// Poll the queue until COMPLETED (~up to 6min).
	for i := 0; i < 90; i++ {
		if err := waitCtx(ctx, 4*time.Second); err != nil {
			return err
		}
		st, err := falStatus(ctx, client, apiKey, sub.StatusURL)
		if err != nil {
			return err
		}
		switch st {
		case "COMPLETED":
			url, err := falResultURL(ctx, client, apiKey, sub.ResponseURL)
			if err != nil {
				return err
			}
			return downloadTo(ctx, client, url, destPath)
		case "IN_QUEUE", "IN_PROGRESS":
			continue
		default:
			return fmt.Errorf("fal: unexpected status %q", st)
		}
	}
	return fmt.Errorf("fal: render timed out")
}

// falStatus GETs a queue status_url and returns the status string.
func falStatus(ctx context.Context, client *http.Client, apiKey, statusURL string) (string, error) {
	raw, err := falGet(ctx, client, apiKey, statusURL)
	if err != nil {
		return "", err
	}
	var out struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("fal status decode: %w", err)
	}
	return out.Status, nil
}

// falResultURL GETs the completed response_url and extracts the video URL.
func falResultURL(ctx context.Context, client *http.Client, apiKey, responseURL string) (string, error) {
	raw, err := falGet(ctx, client, apiKey, responseURL)
	if err != nil {
		return "", err
	}
	var out struct {
		Video struct {
			URL string `json:"url"`
		} `json:"video"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("fal result decode: %w", err)
	}
	if out.Video.URL == "" {
		return "", fmt.Errorf("fal result: no video url")
	}
	return out.Video.URL, nil
}

// falGet is a small authenticated GET returning the body, erroring on non-2xx.
func falGet(ctx context.Context, client *http.Client, apiKey, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Key "+apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fal GET %s: status %d", url, resp.StatusCode)
	}
	return raw, nil
}
