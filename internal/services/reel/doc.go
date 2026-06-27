// Package reel is the application/workflow layer for reel generation: it turns a brief
// into an AI-refined script, drives an external render provider shot-by-shot, then posts
// the finished video through the SHARED outbound spine as a `post_reel` action. It does
// NOT invent a new posting path — Publish builds a models.OutboundMessage and calls
// store.Outbound().Queue, reusing PolicyGate → Claim → Connector → Ledger.
//
// Architecture role: APPLICATION / WORKFLOWS (see MODULE_BOUNDARIES.md).
//
//   - Allowed imports: internal/store (domain access via Reel()/Outbound()), internal/ai,
//     models, internal/store/reel for its constants, stdlib.
//   - Forbidden imports: drivers/* (a service must not import its caller), sibling
//     services (services/facebook/taobao/1688), internal/server (transport).
//
// Money invariant: once a render starts, spend is committed and cannot be cancelled.
// Approve is the single spend gate (StartRenderCAS + per-shot ClaimShotForRender). The
// VideoRenderer port lets tests/CI run the full approve→render→publish flow at zero cost
// via FakeRenderer; the real Cloudflare adapter is selected by config in production.
package reel
