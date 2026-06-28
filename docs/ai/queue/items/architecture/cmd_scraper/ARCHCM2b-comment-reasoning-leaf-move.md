---
id: ARCHCM2b
status: READY
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCM1]
parallel_safe: false
branch: ""
pr_url: ""
boundary_target: leaf-move
---

# ARCHCM2b — Move outbound_comment_reasoning.go into internal/outbound (DI seam first)

## Goal
Move the one independently-movable L3 leaf — `outbound_comment_reasoning.go` — out of
the composition root into a new `internal/outbound` package. Independent of the L2
execution-context home decision (ARCHCM2a): this file carries no L2, queue, or RBAC
dependency. Establishes the `internal/outbound` package + facade for the later moves.

## Component / domain
outbound comment-reasoning (P2c knowledge-grounded comment decision).

## The only cmd-local coupling + how it is resolved (DI seam)
`applyCommentReasoning` builds the concrete cmd adapter `fbContactDirectory{in.db}`
(`outbound_comment_reasoning.go:94`). `fbContactDirectory`
(`facebook_contact_directory.go`, ~30-line composition-root adapter) already
implements the importable `facebook.ContactDirectory` interface. Fix: add a
`facebook.ContactDirectory` field to `commentReasoningInput` (or pass it as a param)
and have the cmd caller construct the adapter. After that, the file references only
importable packages (`ai`, `facebook`, `store`, `workspace_knowledge/runtime`) +
cluster-internal symbols, so it moves cleanly.

This DI seam is behavior-preserving (same adapter, constructed one call frame up). No
queue write and no OWNER/CONTROL/VISIBILITY RBAC gate is touched.

## Files involved
- `cmd/scraper/outbound_comment_reasoning.go` → `internal/outbound/comment_reasoning.go`
  (+ package doc + exported facade: `CommentReasoningMode`, `ApplyCommentReasoning`,
  `CommentReasoningInput`).
- caller: `outbound_lead_pipeline.go:72,192` switches to `outbound.*` and passes the
  `facebook.ContactDirectory` adapter.
- test: migrate the relevant coverage (`outbound_neutral_contract_test.go`) to the
  new package or keep a cmd-level contract test, whichever keeps the boundary clean.

## Dependencies
ARCHCM1 (DONE). NOT ARCHCM2a — no L2 dependency.

## Risk notes
YELLOW: crosses an import boundary (cmd → new internal/outbound) and adds a small DI
parameter. Behavior-preserving. Must add/keep characterization for the dryrun/live/off
modes before the move; verify no import cycle (internal/outbound must not import cmd).

## Validation
go build ./... ; go test ./... ; scripts/check_topology.sh ; scripts/go_cognitive_check.sh ;
scripts/check_file_size.py ; ai_validate.sh. New Code Sonar clean (no suppressions).

## Done criteria
`comment_reasoning` lives in `internal/outbound` behind a facade; `applyCommentReasoning`
takes an injected `facebook.ContactDirectory`; caller updated; no import cycle;
behavior + tests green; no queue/RBAC semantics change.
