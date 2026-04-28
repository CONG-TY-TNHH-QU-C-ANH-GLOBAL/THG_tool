package browser

import (
	"context"
	"math/rand"
	"time"
)

// HumanDelay blocks for a random duration between min and max, simulating
// human reading/thinking time between browser actions.
// Returns ctx.Err() if the context is cancelled before the delay elapses.
func HumanDelay(ctx context.Context, min, max time.Duration) error {
	if max <= min {
		max = min + time.Second
	}
	jitter := rand.Int63n(int64(max - min))
	delay := min + time.Duration(jitter)

	select {
	case <-time.After(delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ActionDelay applies the standard 2–5s inter-action delay recommended to
// avoid Facebook rate-limiting and bot detection.
func ActionDelay(ctx context.Context) error {
	return HumanDelay(ctx, 2*time.Second, 5*time.Second)
}

// ScrollDelay applies a shorter 800ms–2s delay between scroll steps,
// simulating the pace of a human reading a feed.
func ScrollDelay(ctx context.Context) error {
	return HumanDelay(ctx, 800*time.Millisecond, 2*time.Second)
}

// TypeDelay applies a per-character typing delay (55–180ms) to simulate
// realistic human typing speed when filling input fields.
func TypeDelay(ctx context.Context) error {
	return HumanDelay(ctx, 55*time.Millisecond, 180*time.Millisecond)
}
