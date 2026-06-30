---
id: ARCHST-R3
status: DONE
lane: RED
risk: RED
depends_on: []
parallel_safe: false
branch: "audit/archst-r3-direct-post-boundary"
pr_url: ""
---

# ARCHST-R3 — AUDIT: direct-post lookup boundary (leads vs coordination)

## DECISION (2026-06-30, senior-architect audit) — KEEP-AS-IS. Boundary is CORRECT. No move.
Two domains that share a feature name, NOT one domain split across two packages.

**No cross-domain SQL projection exists (verified by grep):**
- `internal/store/leads/direct_post_lookup.go` reads ONLY the `leads` table (all three methods
  `postLeadsByFBID`/`GetPostLeadByDirectPostRef`/`FindConflictingPostLead` are `SELECT … FROM leads l`);
  0 hits for `direct_post_comment_workflows` in the leads package.
- `internal/store/coordination/direct_post_workflow*.go` reads/writes ONLY
  `direct_post_comment_workflows`; 0 hits for `FROM/JOIN/UPDATE/INTO leads` in coordination.
  `MarkDirectPostCommentQueued` (transitions.go) deliberately does NOT store an outbound id —
  comment: "duplicating its id in this table would invert the ownership boundary." The package
  header states it imports NO leads/outbound (single-table CRUD). Neither package imports the other.

**Truth-ownership (two distinct owners):** `leads` owns observed business facts (the `leads`
table); its direct-post matchers are a read-only lead-domain *projection/query API* + pure-Go
identity matchers (`directPostGroupCompatible`/`isDirectPostConflict`/`isAllDigits`, fburl-only,
correctly colocated with the SELECTs they guard). `coordination` owns process-manager runtime
state (the `direct_post_comment_workflows` CAS/lease state machine — RED runtime). The cross-domain
**join happens ABOVE both, in the composition root** — `cmd/scraper/direct_post_intake_scheduler.go`
(`db.Coordination().Claim…` then `db.Leads().GetPostLeadByDirectPostRef(…)`) and
`internal/server/agent/crawl_direct_post.go` — exactly per the repo's "downstream consumes upstream
via projections/contracts; no bidirectional domain knowledge" rule.

**Rejected (for the record):** merging into one `directpost` store package, or moving the leads
matchers into `coordination`, would *create* the violation this audit checks for — fusing
observed-fact truth with process-runtime CAS state and manufacturing a `coordination → SELECT leads`
edge that does not exist today.

**ARCHCM3 impact — UNBLOCKS it (store-boundary side).** ARCHCM3 (the cmd direct-post-intake move)
needed confirmation that the store boundary is sound. Established: (1) the two store packages are
independent (no import edge, no shared table) → ARCHCM3 is a pure composition-root move that keeps
calling `db.Leads()` + `db.Coordination()` as two distinct seams, no store-layer refactor prerequisite;
(2) ARCHCM3 MUST preserve the join location (keep the leads-read ↔ coordination-CAS composition in the
composition root; do NOT push either call into the other store package); (3) no further store-boundary
work gates ARCHCM3. With ARCHCM2 now closed too, ARCHCM3's `depends_on: [ARCHCM2, ARCHST-R3]` is satisfied.

**Guard:** `internal/store/coordination` must never SELECT/UPDATE `leads`; `internal/store/leads`
must never touch `direct_post_comment_workflows`. Enforced by `check_topology.sh` §6.2/§6.3 +
`check_import_boundaries.sh` (both pass today — no such edge). Audit-only; no code changed.

## Goal (audit-only)
leads/direct_post_lookup.go (lead-matching) and coordination/direct_post_workflow*.go (workflow state machine) appear to be one domain split across two packages. Confirm the boundary is correct or propose a move.

## Component / domain
store leads ↔ coordination cross-domain projection. RED.

## Files likely involved
leads/direct_post_lookup.go, coordination/direct_post_workflow*.go; spec specs/DIRECT_POST_INTAKE_WORKFLOW.md.

## Dependencies
Relates to ARCHCM3 (cmd direct-post move).

## Risk notes
RED — truth-ownership + cross-domain SQL projection. Human + design-doc decision.

## Validation
N/A (audit).

## Done criteria
Boundary decision recorded (keep-as-is + rationale, or a move plan) before any code.
