---
id: ARCHST1
status: READY
lane: GREEN
risk: GREEN
depends_on: []
parallel_safe: false
branch: ""
pr_url: ""
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

## Risk notes
GREEN — test-only moves, no production code, no schema/ownership change. Watch for unexported helpers a moved test relied on (may need a tiny exported test seam — if so, classify YELLOW and stop).

## Validation
go test ./internal/store/... ; ai_validate.sh

## Done criteria
No top-level *_test.go for an already-extracted domain; equivalent external test exists in the subpackage; CI green; zero production diff.
