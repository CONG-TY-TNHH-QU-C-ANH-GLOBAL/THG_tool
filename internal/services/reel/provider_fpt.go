package reel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// fptTTS synthesizes Vietnamese voiceover via FPT.AI TTS v5 and downloads the resulting mp3
// to destPath. The v5 API is async: the POST returns {async:<mp3 url>, error:0} immediately,
// then that URL 404s until the audio is rendered — so we poll it with downloadTo until it
// serves a 200. Any error degrades the shot honestly (the caller self-reports "failed").
func fptTTS(ctx context.Context, client *http.Client, apiKey, voice, text, destPath string) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("fpt tts: empty text")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.fpt.ai/hmi/tts/v5", strings.NewReader(text))
	if err != nil {
		return err
	}
	req.Header.Set("api_key", apiKey)
	req.Header.Set("voice", voice)
	req.Header.Set("speed", "")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("fpt tts: status %d: %s", resp.StatusCode, sliceStr(string(raw), 200))
	}
	var out struct {
		Async   string `json:"async"`
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("fpt tts: decode: %w", err)
	}
	if out.Error != 0 || out.Async == "" {
		return fmt.Errorf("fpt tts: error=%d msg=%s", out.Error, out.Message)
	}
	// Poll the async URL until the mp3 is ready (~up to 60s).
	var lastErr error
	for i := 0; i < 30; i++ {
		if err := waitCtx(ctx, 2*time.Second); err != nil {
			return err
		}
		if err := downloadTo(ctx, client, out.Async, destPath); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return fmt.Errorf("fpt tts: audio not ready: %w", lastErr)
}
