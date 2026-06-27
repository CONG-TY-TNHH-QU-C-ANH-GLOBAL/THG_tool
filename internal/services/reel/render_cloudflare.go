package reel

import (
	"context"
	"errors"
)

// CloudflareRenderer is the real (paid) render adapter. It is a stub in this milestone:
// the spend path, R2 wiring, and shot stitching land in a follow-up. It exists so the
// config-based selection in cmd/scraper has a real target to name; until implemented it
// refuses to start a render rather than silently no-op'ing (which would strand reels in
// rendering). Production runs FakeRenderer until this is finished.
type CloudflareRenderer struct {
	apiToken  string
	accountID string
}

// NewCloudflareRenderer constructs the adapter from credentials.
func NewCloudflareRenderer(apiToken, accountID string) *CloudflareRenderer {
	return &CloudflareRenderer{apiToken: apiToken, accountID: accountID}
}

// Name identifies the provider.
func (c *CloudflareRenderer) Name() string { return "cloudflare" }

// ErrRendererNotImplemented signals the real adapter is not wired yet.
var ErrRendererNotImplemented = errors.New("reel: cloudflare renderer not implemented yet")

// StartRender is not implemented in this milestone.
func (c *CloudflareRenderer) StartRender(_ context.Context, _ RenderRequest) (RenderHandle, error) {
	return RenderHandle{}, ErrRendererNotImplemented
}
