# Component Hotspots — Inventory

**Status**: ACTIVE_BINDING (living). **Generated**: 2026-06-14 (counts include `_test.go`). **Rules**: `specs/COMPONENT_STRUCTURE_RULES.md`.

Inventory of packages that trip the flat-package smell (rule 2). **No code is moved by this doc** — it triages and names the first refactor target. Counts from `find internal cmd -name '*.go'` per directory.

## Packages over the file-count / prefix-cluster trigger

| Package | `.go` files | Prefix clusters (≥3) | Likely components | Risk | Recommended action | Now / later |
|---|---|---|---|---|---|---|
| `internal/store` | 46 | (root of the store) | already decomposing into subpackages | Medium | **In progress** — `STORE_SUBPACKAGE_REFACTOR.md` (Phases 0–8b shipped); keep extracting | later (ongoing) |
| `internal/ai` | 43 | `comment_*` (12), `agent_*` (11) | **comment**, **agent**, **scoring** | **HIGH** | split into bounded subpackages — **first target (see below)** | **next** (move-only PR) |
| `internal/models` | 42 | — (data contracts) | shared DTOs | Low | acceptable flat — a contracts package is intentionally one role ("data shapes"); leave | later/none |
| `internal/server/agent` | 31 | mixed handlers | agent HTTP transport | Medium | group by sub-capability (outbox / heartbeat / pairing) when next touched | later |
| `internal/store/coordination` | 28 | mixed | ledger / attempts / caps / risk / actor | Medium | already a bounded domain (DOMAINS.md); split internally if it keeps growing | later |
| `internal/store/knowledge` | 19 | mixed | retrieval / ingest / assembly | Low-Med | bounded domain; monitor | later |
| `cmd/scraper` | 19 | `outbound_*` (4), `crawl_*` (3), `comment_*` (3) | command wiring (composition root) | Medium | thin the composition root — move logic into owning components, keep cmd as wiring | later |
| `internal/runtime` | 18 | mixed | event/runtime composition | Low | bounded; monitor | later |
| `internal/telegram/control` | 17 | mixed | telegram control-plane | Low | bounded; monitor | later |
| `internal/store/connectors` | 16 | mixed | session / readiness / pairing / version | Low-Med | bounded domain; readiness/session split if it grows | later |
| `internal/agentloop` | 15 | mixed | agent loop | Low | at threshold; monitor | later |
| `internal/server/auth` | 14 | mixed | auth/invite/membership | Low | bounded; monitor | later |

**Key reading:** only `internal/ai` is BOTH over the file count AND carries two distinct >5 prefix clusters that map to different business capabilities. `internal/store` is already mid-decomposition. `internal/models` is a contracts package (flatness is its correct shape). So the single highest-value, lowest-risk first target is **`internal/ai`**.

---

## First refactor target (PART D candidate) — `internal/ai`

**Status: PROPOSAL ONLY — not implemented. Move-only PR, separate from any feature work.**

### Verified facts (gathered 2026-06-14)
- `comment_*` non-test files: `comment_contacts.go`, `comment_decision.go`, `comment_duplicate.go`, `comment_persona.go`, `comment_quality.go` (+ tests).
- `agent_*` non-test files: `agent_action_router.go`, `agent_brain.go`, `agent_memory.go`, `agent_preflight.go`, `agent_prompt.go`, `agent_request.go`, `agent_responses.go`, `agent_tools.go`, plus `agent.go` (+ tests).
- Other (scoring/shared): `business.go`, `classifier.go`, `group_scorer.go`, `msggen.go`, `policy_gate.go`, `pricer.go`, `profile_inference.go`, `routing_decision.go`, `selector.go`, `selector_healer.go`, `universal.go`, `url_normalize.go`.
- **No cross-cluster symbol coupling:** `comment_*` files reference no agent symbols (Agent/dispatchTool/deterministicFacebook/agentTools); `agent_*` files reference no comment-generation symbols (SanitizeComment/ScreenCommentContacts/GenerateCommentV2/RepairComment/DetectRepeatedText). → splitting comment vs agent will **not** create an import cycle.
- **Blast radius: 10 packages import `internal/ai`.** Renaming `ai.X` → `comment.X`/`agent.X` touches their import lines + call sites → this MUST be a dedicated move-only PR.

### Proposed target structure (to be finalized in the move-only PR's design step)
```
internal/ai/comment/      (package comment)
  types.go        ← CompanyIdentity-facing comment types
  prompt.go       ← from msggen.go (comment generation prompts)
  quality.go      ← comment_quality.go
  repair.go       ← comment_contacts.go + url_normalize.go (host anchoring, link collapse)
  dedup.go        ← comment_duplicate.go
  persona.go      ← comment_persona.go
  decision.go     ← comment_decision.go
  policy.go       ← policy_gate.go
  service.go      ← facade: Generate… / Sanitize… / Screen… / Repair…
  *_test.go       ← move with the files

internal/ai/agent/        (package agent)
  router.go       ← agent_action_router.go
  prompt.go       ← agent_prompt.go
  tools.go        ← agent_tools.go
  memory.go       ← agent_memory.go
  policy.go       ← agent_brain.go / agent_request guards
  preflight.go    ← agent_preflight.go
  responses.go    ← agent_responses.go
  service.go      ← agent.go facade
  *_test.go

internal/ai/scoring/      (package scoring)
  classifier.go   ← classifier.go + universal.go
  group_scorer.go ← group_scorer.go
  business.go     ← business.go + profile_inference.go
  selector.go     ← selector.go + selector_healer.go + pricer.go + routing_decision.go
  *_test.go
```

### Open items the move-only PR MUST resolve first (do NOT skip)
1. **Shared core placement.** `msggen.go` (`MessageGenerator` / `callOpenAI`), `business.go`/`profile_inference.go` (`CompanyIdentity`, `BusinessProfile`), and `classifier`/`universal` may be depended on by BOTH comment and agent. Map every import before moving — shared deps stay at the `internal/ai` root (or a `internal/ai/core` subpackage) that `comment`/`agent`/`scoring` import one-directionally. Confirm no cycle with `go build ./...` + `go vet`.
2. **Facade naming.** Each subpackage exposes a small intent-verb facade (rule 6); callers in the 10 importers switch from `ai.Foo` to `comment.Foo`/`agent.Foo`/`scoring.Foo`.
3. **Tests move with files** (rule 9); no behavior change (rule 5/8 — extraction is not redesign).
4. **Sequence:** one move-only PR (or staged: comment first, then agent, then scoring), each behavior-preserving, guards green, before resuming feature work.

### Decision (founder, 2026-06-14): comment/ FIRST, staged
The `internal/ai` split is **staged move-only PRs in this order**: (1) extract
`internal/ai/comment/` first — it is the most active surface (4 comment fixes
just shipped there: URL-collapse repair, host-anchoring, eligibility), so it earns
the bounded component soonest; (2) then `internal/ai/agent/`; (3) then
`internal/ai/scoring/`. Each PR is behavior-preserving, tests move with the files,
guards green between, no feature work mixed in. Resolve the **shared-core
placement** (open item 1 above) in PR (1)'s design step before moving anything.

### Status — comment/ extraction PR1 SHIPPED (move-only, 2026-06-14)
Branch `refactor/internal-ai-comment-boundary`. **Moved** the self-contained
platform-neutral cluster into `internal/ai/comment/` (package `comment`):

| old (internal/ai) | new (internal/ai/comment) |
|---|---|
| `comment_quality.go` | `quality.go` |
| `comment_contacts.go` | `contact_grounding.go` |
| `url_normalize.go` | `brand_url.go` |
| `comment_duplicate.go` | `duplicate.go` |
| (+ their `_test.go`) | (+ `quality_test.go` / `contact_grounding_test.go` / `brand_url_test.go` / `duplicate_test.go`) |
| — | `doc.go` (package boundary) |

`internal/ai/comment` imports only `internal/models` + stdlib — boundary
verified (`go list -deps`): it imports NO store/server/platform/parent-ai
package. Callers updated: `cmd/scraper/outbound_actions.go` and
`internal/ai/comment_decision.go` now call `comment.*`. No wrappers added.

**Stayed at root (`internal/ai`), with follow-up reasons (NOT forced — rule 3/4):**
- `policy_gate.go` — imports `internal/store` (`LoadOrgCommentPolicies`); moving
  it would pull store into the neutral layer. Follow-up: split the pure
  `EvaluateGate`/`ApplyGate` from the store-bound loader, or relocate to the
  usecase layer.
- `comment_decision.go` — `DecideComment`/`GenerateCommentV2` are methods on the
  root `MessageGenerator` (msggen.go, which also does inbox/recruitment), and it
  defines `buildCompanyBlock`/`buildContactRule` consumed by msggen. Follow-up:
  export the builder helpers + move generation into the comment package, or keep
  generation as a usecase concern.
- `comment_persona.go` — `buildPersonaRule` (unexported) is consumed by msggen.
  Follow-up: export + move with the generation split above.

Two identity→repair→screen pipeline tests were relocated from
`url_normalize_test.go` into root `comment_brand_test.go` (they exercise root
`ResolveCompanyIdentity`/`buildContactRule` + the moved `comment.*` calls).

**Next:** `agent/` then `scoring/` (separate move-only PRs), and the comment
generation/persona/policy follow-ups above once the platform/usecase boundary is
introduced. Behavior unchanged in this PR.
