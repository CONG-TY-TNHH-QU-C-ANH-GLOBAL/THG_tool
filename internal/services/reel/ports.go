package reel

import "context"

// RenderRequest is one reel's render input. Intentionally minimal — no
// per-shot detail, no provider config — until a real async provider (PR-R5)
// needs more.
type RenderRequest struct {
	OrgID  int64
	ReelID int64
	Script string
}

// VideoRenderer is the consumer-owned port RenderFake depends on. The
// service owns this interface because it is the consumer; render_fake.go's
// FakeRenderer is the only implementation until a later PR adds a real
// provider adapter. No result value is returned: nothing in this PR
// persists an output location yet (object storage integration is deferred,
// per the ADR) — add one when a real consumer needs it.
type VideoRenderer interface {
	Render(ctx context.Context, req RenderRequest) error
}
