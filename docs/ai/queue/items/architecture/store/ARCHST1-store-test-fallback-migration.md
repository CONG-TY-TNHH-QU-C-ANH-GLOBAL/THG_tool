---
id: ARCHST1
status: READY
lane: GREEN
risk: GREEN
depends_on: []
parallel_safe: false
branch: "chore/archst1-connectors-test-ownership"
pr_url: ""
last_batch: connectors
---

# ARCHST1 — Migrate top-level store test fallbacks into owning subpackages

## Goal
Resolve the Phase 2–8b "test fallback" pattern: cross-domain `*_test.go` files still sit at internal/store root but test extracted subpackages. Move them to `<domain>/*_test.go` (external `package <domain>_test`).

## Component / domain
store test ownership — coordination, leads, outbound, threads, connectors.

## Files likely involved
comment_*_test.go, lead_*_test.go, outbound_*_test.go, threads_test.go, connector_*_test.go (root) → owning subpackage; rewrite `newSharedStore` → `storetest.CopyTemplate` per DOMAINS.md §3.9.

## Dependencies
None, but sequential (touches many test files; keep batches small, one domain per PR).

## Progress (multi-batch — stays READY until all domains migrated)
- **connectors — DONE (this batch):** moved connector_identity_meta_test.go +
  connector_pairing_ownership_test.go (already `package store_test`, exported API +
  local bootstrap helpers, no cycle) → `internal/store/connectors/` as
  `package connectors_test`. No top-level connector test remains.
- **threads — needs YELLOW handling (NOT a clean GREEN move):**
  `threads_test.go::TestSeedThreadForOrg_ConversationGateAllowsFirstSend` calls the
  UNEXPORTED root method `s.conversationGateForOutbound(...)`. Per the risk note this
  needs a tiny exported test seam (or leaving that one cross-domain test in root) —
  classify YELLOW for that batch, don't force it.
- **remaining (future batches):** coordination, leads, outbound.

## Risk notes
GREEN — test-only moves, no production code, no schema/ownership change. Watch for unexported helpers a moved test relied on (may need a tiny exported test seam — if so, classify YELLOW and stop).

## Validation
go test ./internal/store/... ; ai_validate.sh

## Done criteria
No top-level *_test.go for an already-extracted domain; equivalent external test exists in the subpackage; CI green; zero production diff.
