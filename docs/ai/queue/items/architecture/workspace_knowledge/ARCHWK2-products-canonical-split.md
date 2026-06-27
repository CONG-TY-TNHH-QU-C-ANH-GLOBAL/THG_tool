---
id: ARCHWK2
status: DONE
lane: GREEN
risk: GREEN
depends_on: []
parallel_safe: true
branch: "chore/archwk2-products-canonical-split"
pr_url: https://github.com/CONG-TY-TNHH-QU-C-ANH-GLOBAL/THG_tool/pull/143
---

# ARCHWK2 — Split products/canonical.go by responsibility

## Goal
Break the `products` god-file canonical.go (376-line baseline) into responsibility-named siblings: canonical type, availability enum/variant, pricing/currency, normalization helpers.

## Component / domain
KnowledgeOS products (canonical product model). Package `products`.

## Files likely involved
- internal/workspace_knowledge/products/canonical.go
- NEW availability.go / pricing.go / normalize.go (same package)
- scripts/file_size_allowlist.txt

## Dependencies
None (parallel-safe with ARCHWK1; disjoint files).

## Risk notes
GREEN. Same package; types are used by many adapters but a same-package sibling split changes NO imports. Verify each extracted helper stays pure. canonical_test.go (320) guards behavior.

## Validation
go test ./internal/workspace_knowledge/products/... ; ai_validate.sh

## Done criteria
canonical.go < 200 lines; siblings each < 200; allowlist entry removed; tests green; no behavior change.
