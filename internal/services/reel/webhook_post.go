package reel

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
)

// postRenderWebhook signs an arbitrary render-result payload with the shared HMAC secret
// (X-Reel-Signature, SHA-256 over the raw body) and POSTs it to the local render webhook.
// Both the fake renderer and the real renderer self-report completion through this single
// helper so the signing contract lives in exactly one place (DRY). A non-2xx response is an
// error so callers can log a failed self-report instead of silently dropping a shot.
func postRenderWebhook(client *http.Client, url, secret string, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	req.Header.Set("X-Reel-Signature", hex.EncodeToString(mac.Sum(nil)))

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("render webhook POST: status %d", resp.StatusCode)
	}
	return nil
}
