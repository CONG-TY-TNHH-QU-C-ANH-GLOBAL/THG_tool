---
id: ARCHST1
status: BLOCKED
lane: YELLOW
risk: YELLOW
depends_on: []
parallel_safe: false
branch: "chore/archst1-connectors-test-ownership"
pr_url: ""
last_batch: connectors
blocked_on: shared-test-seam-decision
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
- **remaining (leads, coordination, outbound) — BLOCKED, reclassified YELLOW
  (verified 2026-06-27):** these are NOT clean GREEN moves like connectors was.
  Connectors was uniquely trivial — those tests were already `package store_test`
  (external), exported-API-only, with local bootstraps. Every remaining root test is
  `package store` (internal) and coupled to shared root test infrastructure:
  - **Shared seeders** defined once in root `package store` test files and used across
    many: `seedUser` / `seedAccount` / `seedLead` / `seedListableLead` /
    `containsLeadID` / `markLedger` / `newEngagementTestStore` (in
    lead_engagement_test.go, schema_template_test.go). An external `<domain>_test`
    package loses access to all of them.
  - **Raw `db.db` access** (unexported `*Store.db` field) in work_queue_test (1),
    lead_lifecycle_test (1), lead_engagement_test (4) — invisible to an external test
    package; needs an exported test seam.
  - **Cross-domain + RED setup:** e.g. soft_touch_test (nominally leads) drives
    `db.Coordination().MarkActionLedgerOutcomeByOutbound` (action_ledger — RED) and
    `QueueOutboundForOrg` (outbound). So a "leads" test is not single-domain and its
    setup touches controlled zones.

## BLOCKED — E3 shared-test-seam decision (awaiting founder)
Before any of leads/coordination/outbound can migrate, the shared test scaffolding
needs a home reachable from external `<domain>_test` packages. Options:

- **Option A — build the shared seam first (YELLOW infra PR, recommended):** relocate
  the shared seeders (`seedUser`/`seedAccount`/`seedLead`/`containsLeadID`/`markLedger`)
  into an exported `internal/store/storetest` (or a `storetest`-adjacent) helper, add a
  per-subpackage `newXStore` wrapper (canonical shape: `knowledge/testing_helpers_test.go`),
  and decide where cross-domain tests live (the test belongs to the domain it asserts,
  even if setup spans others). Then the per-domain moves become mechanical.
- **Option B — defer:** connectors (the only pre-migrated domain) is done; the root
  `package store` tests still pass and own no extracted-subpackage gap that breaks CI.
  Leave the rest until the seam is prioritised.
- **Option C — partial, leave cross-domain in root:** only move tests that are
  single-domain + exported-API-only + db.db-free. Survey found NONE among
  leads/coordination/outbound (all share seeders or touch db.db / RED setup), so this
  yields nothing today.

## Risk notes
Connectors batch was GREEN (test-only, self-contained). Remaining domains are YELLOW:
they need a shared-seam design (Option A) — do not force a per-file move that would
duplicate seeders or reach into `db.db` / action_ledger. Per the item's own rule
("may need a tiny exported test seam — if so, classify YELLOW and stop"), stopped here.

## Validation
go test ./internal/store/... ; ai_validate.sh

## Done criteria
No top-level *_test.go for an already-extracted domain; equivalent external test exists in the subpackage; CI green; zero production diff.
