---
id: ARCHCM3
status: DONE
lane: YELLOW
risk: YELLOW
depends_on: [ARCHCM2, ARCHST-R3]
parallel_safe: false
branch: "arch/archcm3-directpost-url-resolve"
pr_url: ""
---

# ARCHCM3 — Move direct-post intake into internal/directpost

## Goal
Move the direct-post application service + scheduler + coordinator (direct_post_intake.go, direct_post_intake_scheduler.go, direct_link_comment.go) from cmd into the existing internal/directpost package.

## Component / domain
direct-post intake domain.

## Files likely involved
cmd/scraper/direct_post_intake*.go, direct_link_comment.go → internal/directpost/.

## Dependencies
ARCHCM2 (intake calls the outbound queue facade); ARCHST-R3 (direct-post boundary audit must settle leads↔coordination ownership first).

## Risk notes
YELLOW move-only; scheduler state machine is domain logic, not infra. Preserve DP status transitions exactly. Account-guard duplication resolved via ARCHCM-R1.

## Validation
go build ./... ; go test ./... ; ai_validate.sh

## Done criteria
direct-post service/scheduler in internal/directpost; cmd only wires the goroutine; transitions unchanged; move-only.

## Resolution (architect-sprint decision — keep-as-is for the runtime, extract the pure leaf)

Feasibility-before-code (Boundary Migration Playbook §3) showed the original scope is
mis-scoped against the target package's actual character:

- `internal/directpost` is a **pure zero-trust validation leaf** — it imports only
  `internal/fburl` + stdlib (package doc: "ZERO-TRUST validation invariants ... pure
  helpers shared by the ingest path and the poller").
- The intake **service** (`direct_post_intake.go`), **scheduler**
  (`direct_post_intake_scheduler.go`), and **coordinator** (`direct_link_comment.go`)
  depend on `internal/store`, `ai.MessageGenerator`, `queueLeadOutreach`
  (→ `internal/services/facebook` = RED outbound queue, which **neutral packages may
  not import** per the boundary law) and `submitOpenCrawl` (jobs). Moving them into the
  pure leaf needs **3 injected ports into RED** and would couple the leaf to the whole
  runtime — destroying the leaf and inverting the boundary direction.

### Options
- **A — full move into `internal/directpost` (original scope).** Rejected: corrupts the
  pure validation leaf; requires 3 RED ports; high blast radius for no boundary gain.
- **B — new application package `internal/directpostsvc` + outreach/crawl ports.**
  Deferred: a real multi-seam YELLOW migration (queue/jobs ports = RED cutover), only
  worth doing if cmd/scraper's direct-post surface grows; tracked as a follow-up if so.
- **C (RECOMMENDED, applied) — extract only the genuinely-pure logic; keep the
  service/scheduler/coordinator in cmd.** `resolveDirectCommentURL` (+ its struct and
  unit test) moved into the validation leaf as `directpost.ResolveCommentURL` /
  `CommentURLResolution` — it uses only `fburl`, which the leaf already imports, so no
  new import, no cycle, no behavior change. The service/scheduler stay in cmd
  (composition root + durable poller), mirroring the **ARCHST-R3 keep-as-is** decision.

### Why safe (§4.1 self-approval)
Behavior-preserving leaf-move + a decision that *declines* a move. No RBAC / schema /
queue / CAS / ledger / DTO semantics changed. The pre-existing characterization test
moved alongside the function (now `directpost.TestResolveCommentURL`); the cmd-side
`TestCommentSinglePost_Delegation` still pins the orchestration contract.

Done: pure URL-resolution lives in the leaf; cmd orchestration delegates to it; the
service/scheduler keep-as-is is recorded. No further move pursued (would harm the
architecture).
