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

// heygenAvatar renders a talking-head avatar video (HeyGen's own Vietnamese voice speaks
// text) and downloads it to destPath. Flow: POST v2/video/generate → {data:{video_id}};
// poll v1/video_status.get until status:"completed"; download video_url. Async render can
// take minutes; we poll with a bounded timeout and degrade honestly on failure.
func heygenAvatar(ctx context.Context, client *http.Client, apiKey, avatarID, voiceID, text, destPath string) error {
	payload := map[string]any{
		"video_inputs": []map[string]any{{
			"character": map[string]any{
				"type": "avatar", "avatar_id": avatarID, "avatar_style": "normal",
			},
			"voice": map[string]any{
				"type": "text", "input_text": text, "voice_id": voiceID,
			},
		}},
		"dimension": map[string]any{"width": 720, "height": 1280},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.heygen.com/v2/video/generate", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("heygen generate: status %d: %s", resp.StatusCode, sliceStr(string(raw), 200))
	}
	var sub struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Data struct {
			VideoID string `json:"video_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &sub); err != nil {
		return fmt.Errorf("heygen generate decode: %w", err)
	}
	if sub.Error != nil {
		return fmt.Errorf("heygen generate: %s", sub.Error.Message)
	}
	if sub.Data.VideoID == "" {
		return fmt.Errorf("heygen generate: no video_id")
	}
	// Poll status (~up to 7.5min).
	for i := 0; i < 90; i++ {
		if err := waitCtx(ctx, 5*time.Second); err != nil {
			return err
		}
		status, url, err := heygenStatus(ctx, client, apiKey, sub.Data.VideoID)
		if err != nil {
			return err
		}
		switch status {
		case "completed":
			return downloadTo(ctx, client, url, destPath)
		case "failed":
			return fmt.Errorf("heygen: render failed")
		default: // processing, pending, waiting
			continue
		}
	}
	return fmt.Errorf("heygen: render timed out")
}

// heygenStatus polls v1/video_status.get and returns (status, video_url).
func heygenStatus(ctx context.Context, client *http.Client, apiKey, videoID string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.heygen.com/v1/video_status.get?video_id="+videoID, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("X-Api-Key", apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("heygen status: %d", resp.StatusCode)
	}
	var out struct {
		Data struct {
			Status   string `json:"status"`
			VideoURL string `json:"video_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", "", fmt.Errorf("heygen status decode: %w", err)
	}
	return out.Data.Status, out.Data.VideoURL, nil
}
