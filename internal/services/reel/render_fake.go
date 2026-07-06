package reel

import "context"

// FakeRenderer is the zero-cost, deterministic VideoRenderer used until a
// real provider adapter lands. No I/O, no external cost, no failure mode.
type FakeRenderer struct{}

// Render implements VideoRenderer.
func (FakeRenderer) Render(_ context.Context, _ RenderRequest) error {
	return nil
}
