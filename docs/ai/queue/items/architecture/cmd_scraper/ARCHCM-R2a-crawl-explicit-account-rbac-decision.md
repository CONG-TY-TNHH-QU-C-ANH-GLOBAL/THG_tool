---
id: ARCHCM-R2a
status: BLOCKED
lane: RED
risk: RED
depends_on: []
parallel_safe: false
branch: ""
pr_url: ""
blocked_on: founder-decision
boundary_target: blocked-decision
---

# ARCHCM-R2a — DECISION: crawl explicit account_id RBAC (account-scope, RED)

## Goal (decision-only — account-scope / RBAC, security-relevant)
Decide whether a crawl with an **explicit** `account_id` must be owner-filtered like
the auto-pick path is. Surfaced by the ARCHCM-R2 crawl-runtime semantics audit
([`ARCHCM-R2`](ARCHCM-R2-crawl-runtime-semantics-audit.md) §Q3). This is a behavior
decision, NOT a refactor.

## Current behavior (verified 2026-06-28)
`resolveCrawlAccountID` (cmd/scraper/crawl_runtime.go) owner-filters **only** when no
account is chosen:
- **auto-pick (`account_id<=0`):** runs `pickReadyFacebookAccountIDForCrawl`, whose
  `allow` gate restricts an identified, non-privileged sales member to accounts they
  own (admin/platform + `userID<=0` scheduler stay org-wide). PR-M3 member scope.
- **explicit (`account_id>0`):** the value is used **as-is — no ownership check**.

So a sales member can launch a crawl on ANOTHER member's account by passing that
account's id explicitly, bypassing the member-scope the auto-pick path enforces.

## Risk
- A sales member can use another member's account connector/identity for a crawl
  (information exposure: reads another member's account context; consumes their
  connector). It is a READ (no public side-effect), which is why the FB write-control
  gate (`canRequesterControlAccount`) deliberately excludes crawl today.
- The asymmetry is the concern: auto-pick is owner-scoped, explicit is org-trusted —
  inconsistent account-scope on the same action.

## Options
- **Option A (recommended): preserve current explicit-account behavior during the
  ARCHCM4 move; decide hardening separately here.** Keeps ARCHCM4 a pure
  behavior-preserving move (architecture-move speed) and does not couple a refactor to
  an auth-behavior change. RBAC hardening is then taken as its own small, tested,
  behavior-CHANGING PR if the founder wants owner-consistency.
- **Option B: make explicit `account_id` owner-consistent before/with ARCHCM4** —
  route the explicit path through the same owner check (sales must own the account;
  admin/platform/scheduler unchanged). Behavior-CHANGING; needs RBAC characterization
  tests (sales-owned / sales-not-owned / admin / platform / scheduler) and a typed
  block reason. Bundling it with ARCHCM4 mixes a refactor with an auth change — only do
  so if the founder explicitly wants them together.
- **Option C: defer ARCHCM4** (see ARCHCM-R2 Option C) — orthogonal to this RBAC call.

## Recommended default: **Option A**
Preserve current behavior for architecture-move speed; track RBAC hardening as a
separate RED decision (this item). Do NOT auto-code either path: a founder must choose,
because both "read-only, org-trusted" and "owner-consistent" are defensible product
behaviors and the code cannot infer which is intended.

## Relationship to ARCHCM4
- Under Option A, ARCHCM4 proceeds preserving current behavior (invariant #6 in
  ARCHCM-R2); this item is then resolved independently.
- Under Option B, ARCHCM4 waits on this item (the move must carry the new gate).

## Validation
N/A (decision). A future Option-B implementation PR needs RBAC characterization tests
for the explicit-account path + a typed block reason.

## Done criteria
Founder chooses A or B (recorded here). If B, a separate behavior-changing PR adds the
owner check + tests. Stays BLOCKED until the founder decides.
