// Package client is the Telegram Bot API transport for ONE bot token. It implements control.Bot.
// The token is held privately and is NEVER logged, returned, or embedded in an error (the request
// URL contains the token, so transport errors are deliberately generic; Telegram's own error
// code/description ARE surfaced — they carry no secret). No command/business logic lives here.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/thg/scraper/internal/telegram/control"
)

// Client is a minimal Telegram Bot API client bound to a single token.
type Client struct {
	token string
	http  *http.Client
	base  string
}

// New builds a client for a token (10s timeout). Empty token → calls fail safely.
func New(token string) *Client {
	return &Client{token: token, http: &http.Client{Timeout: 10 * time.Second}, base: "https://api.telegram.org"}
}

// Bot adapts *Client to control.Bot — used as the BotFactory: control.BotFactory(client.Bot).
func Bot(token string) control.Bot { return New(token) }

type apiResp struct {
	OK     bool `json:"ok"`
	Result struct {
		ID        int64  `json:"id"`
		FirstName string `json:"first_name"`
		Username  string `json:"username"`
		Chat      struct {
			ID       int64  `json:"id"`
			Title    string `json:"title"`
			Username string `json:"username"`
		} `json:"chat"`
	} `json:"result"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
}

func (c *Client) call(method string, body any) (apiResp, error) {
	var out apiResp
	if c.token == "" {
		return out, errors.New("telegram: bot token not configured")
	}
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.base+"/bot"+c.token+"/"+method, rdr)
	if err != nil {
		return out, errors.New("telegram: build request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return out, errors.New("telegram: network error")
	}
	defer resp.Body.Close()
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out, nil
}

// Send delivers a plain-text message to a known numeric chat id.
func (c *Client) Send(chatID int64, text string) error {
	r, err := c.call("sendMessage", map[string]any{"chat_id": chatID, "text": text})
	if err != nil {
		return err
	}
	if !r.OK {
		return errors.New("telegram: send rejected")
	}
	return nil
}

// Resolve sends `text` to a chat reference ("@username" or numeric) and returns the resolved chat,
// or Telegram's error code/description on rejection (token-free) so the domain can classify it.
func (c *Client) Resolve(ref, text string) (control.SendResult, error) {
	r, err := c.call("sendMessage", map[string]any{"chat_id": ref, "text": text})
	if err != nil {
		return control.SendResult{}, err
	}
	return control.SendResult{
		ChatID: r.Result.Chat.ID, Title: r.Result.Chat.Title, Username: r.Result.Chat.Username,
		ErrCode: r.ErrorCode, ErrDesc: r.Description,
	}, nil
}

// GetMe verifies the token and returns the bot identity.
func (c *Client) GetMe() (control.BotInfo, error) {
	r, err := c.call("getMe", nil)
	if err != nil {
		return control.BotInfo{}, err
	}
	if !r.OK || r.Result.ID == 0 {
		return control.BotInfo{}, errors.New("telegram: invalid bot token")
	}
	return control.BotInfo{BotID: r.Result.ID, Username: r.Result.Username, DisplayName: r.Result.FirstName}, nil
}
