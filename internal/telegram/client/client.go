// Package client is the Telegram Bot API transport. It exposes only what the runtime needs
// (sendMessage, and resolving a chat reference to its id/title) and satisfies the control.Bot
// interface structurally — it imports NO domain package. The bot token is held privately and is
// NEVER logged, returned, or embedded in an error (the request URL contains the token, so errors
// are deliberately generic). No command/business logic lives here.
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

// New builds a client with a 10s timeout. An empty token yields a client whose calls fail safely.
func New(token string) *Client {
	return &Client{token: token, http: &http.Client{Timeout: 10 * time.Second}, base: "https://api.telegram.org"}
}

// Configured reports token presence WITHOUT exposing the value.
func (c *Client) Configured() bool { return c.token != "" }

// sendMessageResp is the slice of the Telegram sendMessage response we read.
type sendMessageResp struct {
	OK     bool `json:"ok"`
	Result struct {
		Chat struct {
			ID       int64  `json:"id"`
			Title    string `json:"title"`
			Username string `json:"username"`
		} `json:"chat"`
	} `json:"result"`
}

// sendMessage POSTs to sendMessage with an arbitrary chat reference (numeric id or "@username") and
// returns the resolved chat from the response. Shared by Send + Resolve.
func (c *Client) sendMessage(chatRef any, text string) (sendMessageResp, error) {
	var out sendMessageResp
	if c.token == "" {
		return out, errors.New("telegram: bot token not configured")
	}
	payload, _ := json.Marshal(map[string]any{"chat_id": chatRef, "text": text})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		c.base+"/bot"+c.token+"/sendMessage", bytes.NewReader(payload))
	if err != nil {
		return out, errors.New("telegram: build request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return out, errors.New("telegram: send failed (network)")
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return out, fmt.Errorf("telegram: send failed (status %d)", resp.StatusCode)
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out, nil
}

// Send delivers a plain-text message to a known numeric chat id (satisfies control.Bot.Send).
func (c *Client) Send(chatID int64, text string) error {
	_, err := c.sendMessage(chatID, text)
	return err
}

// Resolve sends `text` to a chat reference ("@username" for a public channel, or a numeric id) and
// returns the resolved chat id/title/username — used to connect a public channel in one verified
// call (satisfies control.Bot.Resolve). Primitive returns keep this package domain-free.
func (c *Client) Resolve(ref, text string) (chatID int64, title, username string, err error) {
	r, err := c.sendMessage(ref, text)
	if err != nil {
		return 0, "", "", err
	}
	return r.Result.Chat.ID, r.Result.Chat.Title, r.Result.Chat.Username, nil
}
