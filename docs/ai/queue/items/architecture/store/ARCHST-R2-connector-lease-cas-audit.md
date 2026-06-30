---
id: ARCHST-R2
status: REVIEW
lane: RED
risk: RED
depends_on: []
parallel_safe: false
branch: "audit/archst-r2-lease-cas-decision"
pr_url: ""
---

# ARCHST-R2 — AUDIT: connector pairing lease / CAS consistency

## Goal (audit-only)
Determine whether the comment-reverify claim lease and the connector pairing claim/lease are independent or should be unified; document divergence if intentional.

## Component / domain
store/connectors CAS/lease + coordination reverify queue. RED.

## Files likely involved
connectors/connector_pairing.go, coordination/comment_reverify*.go.

## Dependencies
None.

## Risk notes
RED — connector CAS/lease/ownership semantics. Human decision; no autonomous change.

## Validation
N/A (audit).

## Done criteria
Decision record: unify or document-divergence, with rationale for future maintainers.

---

# DECISION RECORD (ARCHST-R2) — audit-only, no code/schema/CAS/lease change

**Class:** E3 controlled-zone (connector CAS/lease, action_ledger-adjacent) — RED, human decision.
**Trigger:** queue asks whether the connector pairing claim and the comment-reverify claim
lease are independent or should be unified.

## Verified current state (read-only, vs HEAD)

The repo has **two distinct categories** of pull-based single-claim, not one:

### Category 1 — one-shot resource consume (NOT a lease)
- `connectors.ClaimConnectorPairingCode` (`connector_pairing.go`).
- Consumes a **pairing code** to mint an `agent_token`. **Terminal**: once claimed it is
  never re-offered.
- Single-claim CAS = transactional `UPDATE connector_pairing_codes SET used_at=CURRENT_TIMESTAMP
  … WHERE id=? AND used_at IS NULL` + `RowsAffected==1` (→ `ErrPairingCodeConsumed`), **plus** a
  structural backstop unique index `uq_agent_tokens_active_profile` (→ `ErrDevicePairedToAnotherUser`),
  **plus** the ownership guard `guardBrowserProfileBindingTx`.
- "TTL" here = `expires_at` **code expiry checked before claim** (default 10 min) — NOT a
  post-claim, re-offerable lease. There is **no** `claimed_at`/re-claim concept.

### Category 2 — recurring work-queue lease (re-offerable)
- `coordination.ClaimDueReverifies` (`comment_reverify.go`) and its sibling
  `coordination.ClaimDueDirectPostCommentWorkflows` (`direct_post_workflow_transitions.go`).
- Claim a job, process, report; **re-offered** if the worker crashes before reporting.
- Single-claim CAS = `WHERE outcome=pending AND (claimed_at IS NULL OR claimed_at <= leaseCutoff)
  AND claim_count < N`, then stamp `claimed_at`, `claim_count++`, `claimed_by_token_id`.
- Real **lease**: `ReverifyClaimLease = 5m`; `DPDefaultLeaseDuration = 5m` whose code comment
  states it *"mirrors the reverify claim lease."* Both add a claim-budget self-heal
  (`claim_count >= 3` → retire as error) so a job never loops forever.

There is **no shared claim helper today**; each `ClaimDue*` is its own SQL.

## Why the divergence is INTENTIONAL and correct
Pairing-consume and reverify-lease are different object lifecycles: **terminal consumption of a
credential** vs **renewable lease over a repeatable work item**. Their failure modes differ
(code-expiry-before-claim vs worker-crash-after-claim), so their invariants differ. Forcing
them under one "lease/CAS" abstraction would be a leaky abstraction that hides the terminal-vs-
re-offer distinction — exactly the kind of premature unification the platform freeze guards
against. They correctly share only the *conceptual* "SQL single-claim via a guarded UPDATE"
idiom, not code.

## Options
- **A — Document divergence; keep the two categories independent (RECOMMENDED, applied as the
  decision; preserves all current behavior).** No code/schema/CAS/lease change. Records the
  two-category model above for future maintainers. Zero risk; preserves pull-based execution,
  single-claim CAS, idempotency, and no-double-claim exactly.
- **B — Extract a shared "guarded single-claim UPDATE" helper across both categories.**
  Rejected: pairing is terminal + has a unique-index backstop + ownership guard; reverify is a
  budgeted renewable lease. A shared helper would either leak both parameter sets or erase the
  terminal/re-offer distinction. Net negative; no duplication pressure justifies it (two call
  sites, different columns).
- **C — Consolidate ONLY the two Category-2 lease queues (reverify + direct-post-workflow) behind
  one lease helper.** Plausible *future* YELLOW, but deferred: they already align on the 5-min
  constant by intent, yet differ in columns (`comment_reverify` vs `direct_post_comment_workflows`),
  budget, worker-id vs token-id, and outcome vocabulary. Revisit only when a **3rd** concrete
  Category-2 queue appears and forces real duplication — then a `coordination` lease helper +
  characterization tests, never a cross-domain (connectors↔coordination) merge.

## Recommended default: **A** (document divergence; no unification).
Rationale: the connector pairing claim and the comment-reverify lease are **independent by
design** — different lifecycles, different invariants. Unifying them (B) would degrade clarity
and risk the single-claim/no-double-claim guarantees this zone exists to protect. Any future
DRY work is bounded to Category 2 only (C), deferred until duplication is real.

## Why safe / remaining risk
Audit-only: **no production code, schema, migration, CAS, lease, or idempotency change**.
Behavior preserved by construction. Remaining risk: none introduced. Follow-up (optional, not
filed): if Category-2 queues proliferate, open a YELLOW item for option C.

Item stays `REVIEW` for founder ratification of option A; DONE is set only by queue reconcile
after merge.
