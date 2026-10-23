package ai

import (
	"context"
	"errors"
	"net/http"
)

// The code is deliberately independent of provider bodies, request URLs and
// transport error strings, which can contain credentials or signed media URLs.
type leadModelFailure string

func (e leadModelFailure) Error() string { return string(e) }

func LeadModelFailureCode(err error) string {
	if err == nil {
		return "none"
	}
	var failure leadModelFailure
	if errors.As(err, &failure) {
		return string(failure)
	}
	return "unclassified"
}

func leadTransportFailure(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return leadModelFailure("timeout")
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return leadModelFailure("canceled")
	}
	return leadModelFailure("transport")
}

func leadProviderFailure(status int) error {
	switch status {
	case http.StatusBadRequest, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity:
		return leadModelFailure("provider_rejected_input")
	case http.StatusUnauthorized, http.StatusForbidden:
		return leadModelFailure("provider_auth")
	case http.StatusTooManyRequests:
		return leadModelFailure("provider_rate_limit")
	default:
		return leadModelFailure("provider_http_error")
	}
}
