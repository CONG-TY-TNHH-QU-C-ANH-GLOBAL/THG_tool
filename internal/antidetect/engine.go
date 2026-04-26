package antidetect

import (
	"context"
	"math/rand"
	"time"
)

// BehaviorPolicy configures human-simulation parameters.
type BehaviorPolicy struct {
	MinActionDelayMs  int // minimum delay between page actions
	MaxActionDelayMs  int // maximum delay between page actions
	MinTypingDelayMs  int // minimum delay per character
	MaxTypingDelayMs  int // maximum delay per character
	BreakEveryNItems  int // take a longer break every N items
	BreakDurationSec  int // break duration in seconds
	ScrollProbability float64 // probability of random scroll between items
}

func DefaultPolicy() BehaviorPolicy {
	return BehaviorPolicy{
		MinActionDelayMs:  800,
		MaxActionDelayMs:  2500,
		MinTypingDelayMs:  60,
		MaxTypingDelayMs:  180,
		BreakEveryNItems:  20,
		BreakDurationSec:  8,
		ScrollProbability: 0.30,
	}
}

// Engine injects human-like timing into automation loops.
type Engine struct {
	policy    BehaviorPolicy
	itemCount int
	r         *rand.Rand
}

func New(p BehaviorPolicy) *Engine {
	return &Engine{
		policy: p,
		r:      rand.New(rand.NewSource(time.Now().UnixNano())), //nolint:gosec
	}
}

// ActionDelay sleeps a human-like duration between page interactions.
// Respects context cancellation.
func (e *Engine) ActionDelay(ctx context.Context) error {
	ms := e.policy.MinActionDelayMs +
		e.r.Intn(e.policy.MaxActionDelayMs-e.policy.MinActionDelayMs+1)
	return sleep(ctx, time.Duration(ms)*time.Millisecond)
}

// TypingDelay sleeps per character when simulating text input.
func (e *Engine) TypingDelay(ctx context.Context) error {
	ms := e.policy.MinTypingDelayMs +
		e.r.Intn(e.policy.MaxTypingDelayMs-e.policy.MinTypingDelayMs+1)
	return sleep(ctx, time.Duration(ms)*time.Millisecond)
}

// OnItemProcessed must be called after each item is processed.
// It may sleep for a break period if BreakEveryNItems threshold is reached.
func (e *Engine) OnItemProcessed(ctx context.Context) error {
	e.itemCount++
	if e.policy.BreakEveryNItems > 0 && e.itemCount%e.policy.BreakEveryNItems == 0 {
		// Add ±20% jitter to break duration
		base := e.policy.BreakDurationSec
		jitter := e.r.Intn(max(1, base/5))
		dur := time.Duration(base+jitter) * time.Second
		if err := sleep(ctx, dur); err != nil {
			return err
		}
	}
	if e.r.Float64() < e.policy.ScrollProbability {
		// Simulate reading time (scroll pause) — brief
		ms := 300 + e.r.Intn(700)
		if err := sleep(ctx, time.Duration(ms)*time.Millisecond); err != nil {
			return err
		}
	}
	return nil
}

// ShouldTakeBreak returns true when the engine is due for a long break.
// Callers may use this to skip processing until the break is served via OnItemProcessed.
func (e *Engine) ShouldTakeBreak() bool {
	return e.policy.BreakEveryNItems > 0 && e.itemCount > 0 &&
		e.itemCount%e.policy.BreakEveryNItems == 0
}

func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

