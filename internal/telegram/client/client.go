// Package client is the Telegram Bot API transport. It exposes ONLY what the runtime needs
// (sendMessage) and implements control.Sender. The bot token is held privately and is NEVER
// logged, returned, or embedded in an error (the request URL contains the token, so errors are
// deliberately generic). No command/business logic lives here.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Client is a minimal Telegram Bot API client.
type Client struct {
	token string
	http  *http.Client
	base  string
}

// New builds a client with a 10s timeout. An empty token yields a client whose Send fails safely.
func New(token string) *Client {
	return &Client{token: token, http: &http.Client{Timeout: 10 * time.Second}, base: "https://api.telegram.org"}
}

// Configured reports token presence WITHOUT exposing the value.
func (c *Client) Configured() bool { return c.token != "" }

// Send delivers a plain-text message to a chat. Implements control.Sender. Errors never include
// the token or the (token-bearing) URL.
func (c *Client) Send(chatID int64, text string) error {
	if c.token == "" {
		return errors.New("telegram: bot token not configured")
	}
	payload, _ := json.Marshal(map[string]any{"chat_id": chatID, "text": text})
	url := c.base + "/bot" + c.token + "/sendMessage"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return errors.New("telegram: build request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("telegram: send failed (network)")
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("telegram: send failed (status %d)", resp.StatusCode)
	}
	return nil
}
