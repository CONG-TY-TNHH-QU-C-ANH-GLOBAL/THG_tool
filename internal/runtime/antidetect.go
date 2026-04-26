package runtime

import (
	"context"
	"math/rand"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

// BehaviorConfig controls human-like scroll and interaction simulation.
type BehaviorConfig struct {
	MinReadPauseMs int     // minimum pause between scrolls (simulates reading)
	MaxReadPauseMs int     // maximum pause
	LongPauseMs    int     // occasional long pause (distraction)
	LongPauseP     float64 // probability of a long pause (0.05 = 5%)
	ReverseScrollP float64 // probability of scrolling up briefly (0.12 = 12%)
	ScrollStepMin  int     // minimum scroll delta in pixels
	ScrollStepMax  int     // maximum scroll delta in pixels
}

// DefaultBehavior is the recommended config for undetectable scraping.
var DefaultBehavior = BehaviorConfig{
	MinReadPauseMs: 800,
	MaxReadPauseMs: 3500,
	LongPauseMs:    8000,
	LongPauseP:     0.05,
	ReverseScrollP: 0.12,
	ScrollStepMin:  220,
	ScrollStepMax:  380,
}

// HumanScroller simulates human-like scrolling inside a CDP tab context.
type HumanScroller struct {
	cfg BehaviorConfig
	rng *rand.Rand
}

// NewHumanScroller creates a scroller seeded with the account ID for deterministic randomness.
func NewHumanScroller(accountID int64) *HumanScroller {
	return &HumanScroller{
		cfg: DefaultBehavior,
		rng: rand.New(rand.NewSource(accountID + time.Now().UnixNano())),
	}
}

// ScrollOnce performs one human-like scroll step inside the given chromedp context.
// It may scroll up first (reverse), then down, then pause to simulate reading.
func (h *HumanScroller) ScrollOnce(ctx context.Context, x, y int) error {
	// Occasionally scroll up a little (reading back)
	if h.rng.Float64() < h.cfg.ReverseScrollP {
		reverseY := -(h.rng.Intn(150) + 50)
		if err := chromedp.Run(ctx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				return input.DispatchMouseEvent(input.MouseWheel, float64(x), float64(y)).
					WithDeltaX(0).WithDeltaY(float64(reverseY)).Do(ctx)
			}),
		); err != nil {
			return err
		}
		h.sleep(ctx, 200, 500)
	}

	// Main scroll down
	step := h.rng.Intn(h.cfg.ScrollStepMax-h.cfg.ScrollStepMin+1) + h.cfg.ScrollStepMin
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(input.MouseWheel, float64(x), float64(y)).
				WithDeltaX(0).WithDeltaY(float64(step)).Do(ctx)
		}),
	); err != nil {
		return err
	}

	// Reading pause
	minMs := h.cfg.MinReadPauseMs
	maxMs := h.cfg.MaxReadPauseMs
	if h.rng.Float64() < h.cfg.LongPauseP {
		// Long pause (distraction / reading deeply)
		minMs = h.cfg.LongPauseMs
		maxMs = h.cfg.LongPauseMs * 2
	}
	return h.sleep(ctx, minMs, maxMs)
}

func (h *HumanScroller) sleep(ctx context.Context, minMs, maxMs int) error {
	ms := minMs
	if maxMs > minMs {
		ms = h.rng.Intn(maxMs-minMs) + minMs
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(ms) * time.Millisecond):
		return nil
	}
}

// MoveMouse moves the mouse to (x, y) via a rough bezier approximation.
// This avoids the teleportation pattern that anti-bot systems detect.
func MoveMouse(ctx context.Context, fromX, fromY, toX, toY int, rng *rand.Rand) error {
	steps := 12 + rng.Intn(8)
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		// Simple linear interpolation with small noise
		nx := float64(fromX) + t*float64(toX-fromX) + float64(rng.Intn(5)-2)
		ny := float64(fromY) + t*float64(toY-fromY) + float64(rng.Intn(5)-2)
		if err := chromedp.Run(ctx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				return input.DispatchMouseEvent(input.MouseMoved, nx, ny).Do(ctx)
			}),
		); err != nil {
			return err
		}
		delay := time.Duration(rng.Intn(20)+10) * time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil
}

// checkBanSignals inspects the current page URL and title for Facebook anti-scraping indicators.
// Returns a CDPError with the appropriate ban code if detected, nil otherwise.
func checkBanSignals(ctx context.Context) error {
	var url, title string
	if err := chromedp.Run(ctx,
		chromedp.Location(&url),
		chromedp.Title(&title),
	); err != nil {
		return nil // can't check — don't block
	}

	if containsAny(url, "checkpoint", "login?next", "recover") {
		return CDPError{Code: ErrFacebookCheckpoint, Message: "checkpoint page detected: " + url}
	}
	if containsAny(url, "/login") {
		return CDPError{Code: ErrFacebookLogout, Message: "redirected to login: " + url}
	}
	if containsAny(title, "you've been temporarily blocked", "tạm thời bị chặn") {
		return CDPError{Code: ErrFacebookBanned, Message: "ban page detected: " + title}
	}
	return nil
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
