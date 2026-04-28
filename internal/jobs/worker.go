package jobs

import "time"

// RetryDelay returns the backoff delay for a given attempt number (0-indexed).
// These values must match the CASE expression in Store.Fail().
//
//	attempt 0 → 1s   (first retry: fast)
//	attempt 1 → 3s
//	attempt 2 → 7s
//	attempt 3+ → 15s (final retry: slow)
func RetryDelay(attempt int) time.Duration {
	delays := []time.Duration{1, 3, 7, 15}
	if attempt < len(delays) {
		return delays[attempt] * time.Second
	}
	return 15 * time.Second
}
