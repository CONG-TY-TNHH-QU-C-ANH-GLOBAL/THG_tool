package runtime

import (
	"context"
	"time"
)

// RetryConfig controls retry behaviour for CDP operations.
type RetryConfig struct {
	MaxAttempts int
	BaseBackoff time.Duration
}

// DefaultRetryConfig retries up to 3 times with exponential backoff starting at 2s.
var DefaultRetryConfig = RetryConfig{
	MaxAttempts: 3,
	BaseBackoff: 2 * time.Second,
}

// WithRetry calls fn up to cfg.MaxAttempts times.
// It retries only when IsRetryable(err) is true.
// Backoff doubles each attempt: BaseBackoff, 2×, 4×, ...
func WithRetry[T any](ctx context.Context, cfg RetryConfig, fn func() (T, error)) (T, error) {
	var zero T
	backoff := cfg.BaseBackoff
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}
		if !IsRetryable(err) || attempt == cfg.MaxAttempts {
			return zero, err
		}
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	return zero, CDPError{Code: ErrChromeUnreachable, Message: "max retries exceeded"}
}
