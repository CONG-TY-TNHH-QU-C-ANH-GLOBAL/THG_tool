package rest_json

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/ingestion"
)

// HTTPDoer is the constrained surface the rest_json adapter needs
// from an HTTP client. *http.Client satisfies it; tests pass a fake
// so the adapter is exercisable without a network. Keeping the
// surface narrow prevents the adapter from reaching for irrelevant
// http.Client knobs (transport, jar, …) that production code does
// not need.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// applyAuth attaches auth headers to req based on cfg. Token / header
// values come from environment variables; the connection_config blob
// only carries the env var NAME, never the secret itself.
//
// Returns a permanent ingestion error if the configured env var is
// unset — that is a deployment problem, not a transient one.
func applyAuth(req *http.Request, cfg AuthConfig) error {
	switch cfg.Type {
	case "none":
		return nil
	case "bearer":
		v := os.Getenv(cfg.TokenEnv)
		if v == "" {
			return ingestion.WrapPermanent(fmt.Errorf("rest_json: %s env var is empty", cfg.TokenEnv))
		}
		req.Header.Set("Authorization", "Bearer "+v)
		return nil
	case "header":
		v := os.Getenv(cfg.ValueEnv)
		if v == "" {
			return ingestion.WrapPermanent(fmt.Errorf("rest_json: %s env var is empty", cfg.ValueEnv))
		}
		req.Header.Set(cfg.HeaderName, v)
		return nil
	}
	return ingestion.WrapPermanent(fmt.Errorf("rest_json: unknown auth type %q", cfg.Type))
}

// buildURL composes the per-page request URL. For "page" scheme it
// adds the configured page/limit query params; for "none" it returns
// the base URL unchanged. Extra query params already on the base URL
// are preserved.
func buildURL(cfg *Config, page int) (string, error) {
	u, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return "", ingestion.WrapPermanent(fmt.Errorf("rest_json: parse base_url: %w", err))
	}
	if cfg.Pagination.Scheme == "none" {
		return u.String(), nil
	}
	q := u.Query()
	q.Set(cfg.Pagination.PageParam, strconv.Itoa(page))
	if cfg.Pagination.LimitParam != "" && cfg.Pagination.LimitValue > 0 {
		q.Set(cfg.Pagination.LimitParam, strconv.Itoa(cfg.Pagination.LimitValue))
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// classifyHTTPError maps an HTTP status into the ingestion error
// taxonomy. Centralised so every call site uses the same mapping —
// retry / backoff / health-status decisions all read off this
// classification.
//
//	2xx        — caller treats as success (this function not called)
//	401, 403   — permanent (auth misconfig — operator must fix env)
//	408, 429   — recoverable (rate limit / timeout — backoff + retry)
//	4xx other  — permanent (URL / schema problem)
//	5xx        — recoverable (upstream blip — retry)
//	otherwise  — recoverable (unknown — let the dispatcher retry once)
func classifyHTTPError(status int, body string) error {
	excerpt := body
	if len(excerpt) > 200 {
		excerpt = excerpt[:200] + "…"
	}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return ingestion.WrapPermanent(fmt.Errorf("rest_json: auth failure %d: %s", status, excerpt))
	case status == http.StatusRequestTimeout, status == http.StatusTooManyRequests:
		return ingestion.WrapRecoverable(fmt.Errorf("rest_json: transient %d: %s", status, excerpt))
	case status >= 400 && status < 500:
		return ingestion.WrapPermanent(fmt.Errorf("rest_json: client error %d: %s", status, excerpt))
	case status >= 500:
		return ingestion.WrapRecoverable(fmt.Errorf("rest_json: upstream %d: %s", status, excerpt))
	}
	return ingestion.WrapRecoverable(fmt.Errorf("rest_json: unexpected %d: %s", status, excerpt))
}

// doRequest issues one HTTP GET with the configured auth + UA and
// reads the entire body (cap 16 MB to avoid OOM on a misconfigured
// source). Returns the body bytes and any classified error.
//
// Network-level errors are recoverable (DNS blip, TCP reset); HTTP
// status errors are classified per classifyHTTPError. Body-read
// errors after a 2xx are recoverable (partial transfer can succeed
// on retry).
func doRequest(ctx context.Context, doer HTTPDoer, cfg *Config, fullURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, ingestion.WrapPermanent(fmt.Errorf("rest_json: build request: %w", err))
	}
	req.Header.Set("Accept", "application/json")
	if cfg.Request.UserAgent != "" {
		req.Header.Set("User-Agent", cfg.Request.UserAgent)
	}
	if err := applyAuth(req, cfg.Auth); err != nil {
		return nil, err
	}

	resp, err := doer.Do(req)
	if err != nil {
		// Network-level — context cancel is preserved as recoverable
		// but the dispatcher will not retry a cancelled context anyway.
		return nil, ingestion.WrapRecoverable(fmt.Errorf("rest_json: http: %w", err))
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, 16<<20) // 16 MB safety cap
	body, readErr := io.ReadAll(limited)
	if readErr != nil {
		if errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) {
			return nil, ingestion.WrapRecoverable(readErr)
		}
		return nil, ingestion.WrapRecoverable(fmt.Errorf("rest_json: read body: %w", readErr))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return body, classifyHTTPError(resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if len(body) == 0 {
		return body, ingestion.WrapRecoverable(fmt.Errorf("rest_json: empty body from %s", fullURL))
	}
	return body, nil
}
