# Component Structure Rules

**Status**: ACTIVE_BINDING. **Created**: 2026-06-14. **Enforced by**: `scripts/check_component_structure.py` (warn-only baseline), code review, and the pre-add checklist below.

These rules are binding for Claude and every human/agent refactor. They sit alongside the Engineering Guardrails in `CLAUDE.md` (200-line limit, SOLID/DRY/SRP, feature-based structure) and the topology/tenant guards. The goal: **think in components before adding or moving files**, so packages stay navigable, testable, and debuggable instead of flattening into same-prefix dumping grounds (`comment_*`, `agent_*`, `business_*`).

> Companion docs: `specs/COMPONENT_HOTSPOTS.md` (current inventory + first refactor target), `internal/store/DOMAINS.md` (store domain map), `specs/RUNTIME_TOPOLOGY.md` + `scripts/check_topology.sh` (boundary enforcement).

## 0. Architecture layers (binding direction)

This is a SaaS automation platform; Facebook is only the FIRST surface (Instagram /
TikTok / Telegram / Taobao / 1688 / POD come later). Structure must never become
Facebook-only. Three layers, with a one-way dependency direction:

1. **Intelligence layer** (`internal/ai/<capability>/`, e.g. `internal/ai/comment/`) —
   platform-NEUTRAL reasoning over text + grounded business identity (comment
   quality / repair / brand-URL / contact grounding / persona / duplicate /
   neutral prompt+decision helpers). MUST NOT import any platform / Facebook /
   outbound / connector / server / store package.
2. **Platform adapter layer** (`internal/platform/<platform>/` or existing
   connector packages) — platform-specific details: URL parsing, post/group/page
   context, actor/session/account rules, composer/runtime. May depend on
   platform-neutral interfaces/types.
3. **Application / usecase orchestration** (`internal/usecase/<workflow>/` or the
   existing command/service wiring) — gets context from a platform adapter, calls
   the intelligence layer to validate/repair/generate, calls outbound + connector
   readiness, notifies Telegram/Copilot.

**Dependency direction (one way):** usecase → (intelligence, platform adapters,
outbound); platform adapters → neutral types; **intelligence imports none of the
platform / outbound / connector / server layers**. Reusable intelligence stays
platform-neutral. Do not create the full platform/usecase layer speculatively —
extract a neutral intelligence component first; place platform-specific logic in
the adapter/usecase layer, never under `internal/ai`.

---

## 1. Component-first thinking

- A directory/package is a **bounded component**, not a folder of conveniences.
- A file is **one role** inside a component (a generator, a validator, a store, a router…), not a grab-bag.
- Do **not** add a new top-level file just because it's convenient to drop it next to the others.
- **Before adding any file, answer four questions** (put the answer in the PR/commit body):
  1. Which **component owns** this behavior?
  2. Which **public API (facade)** should expose it?
  3. Which **tests** prove it?
  4. Which existing file/component **should NOT know** about it (boundary you must not cross)?

If you can't name the owning component, you're probably about to grow a flat namespace — stop and check this doc.

## 2. Flat-package smell (review triggers)

Review a package for a component split when ANY holds:
- it has **> 15 source files**, or
- **> 5 files share a prefix** (e.g. `comment_*`), or
- its tests cannot be mapped one-to-one to a component, or
- a one-feature change forces touching **unrelated** files, or
- the folder hosts **multiple business capabilities**.

A trigger is a **review signal**, not an automatic mandate to move code. Triage in `COMPONENT_HOTSPOTS.md` first.

## 3. Prefix-cluster rule

When a folder accumulates many same-prefix files, the prefix is the component trying to be born:
- `comment_*` → a **comment** component (generation / quality / repair / dedup / persona / decision).
- `agent_*` → an **agent** component (routing / prompt / tools / memory / policy / preflight / responses).
- `business_*` → a **business / context** component.
- role-suffix peers (`*_policy`, `*_quality`, `*_decision`, `*_persona`, `*_duplicate`) belong **under the owning capability**, not as independent peers forever.

## 4. Go package rule

- Do **not** create a micro-package per file. Split only by **bounded context**:
  - comment: generation / quality / repair / duplicate
  - agent: routing / prompt / tools / memory / policy
  - scoring / classification
  - outbound: queue / claim / finalize / dedup / policy
  - connectors: session / readiness
  - leads: workqueue / eligibility
- A subpackage MUST have:
  - a clear **owner README or package doc comment**,
  - a **minimal public API** (facade),
  - **no import cycle** (verify with `go build ./...` + `go vet`),
  - **tests for exported behavior**,
  - **no direct peer-domain DB writes** unless topology allows it (`scripts/check_topology.sh`; cross-domain reads need `// tenant-ok`, writes need a Hooks struct).

## 5. File-responsibility rule

Name files by responsibility, not by accident:

`types.go` · `service.go` · `policy.go` · `quality.go` · `repair.go` · `dedup.go` · `prompt.go` · `router.go` · `handler.go` · `store.go` · `<behavior>_test.go`.

Avoid vague catch-alls — `utils.go`, `helpers.go`, `misc.go`, `manager.go` — unless tightly scoped AND documented with a package-doc explaining the single responsibility. The component-structure guard flags new `utils.go`/`helpers.go` without a component prefix.

## 6. Public-API (facade) rule

Each component exposes a **small facade** of intent-named verbs: `Evaluate…`, `Generate…`, `Plan…`, `Queue…`, `Resolve…`, `Pick…`. Internal helpers stay **unexported**. Other components call the facade — never reach into a deep helper file. (Mirrors the existing outbound `s.Outbound().Queue/Claim/Finalize` and connectors `PickReadyConnector` facades.)

## 7. Refactor process (binding sequence)

Every structural refactor follows this order:
1. **inventory** current files,
2. **identify** component clusters,
3. **define** the target map (written down, reviewed),
4. **move-only or wrapper-first** PR (no semantic change),
5. **preserve behavior** (extraction is not redesign),
6. **add/update tests** (tests move with the component),
7. run **topology / file-size / tenant** guards,
8. only THEN add new feature work.

## 8. No-big-bang rule

Do **not** mix in one PR: behavior fix · file split · schema migration · feature addition · API redesign. Each PR **declares its type** in the body:
- `docs-only` · `move-only` · `additive behavior` · `cleanup/removal` · `feature`.

(Reinforces the locked `feedback_staged_evolution_over_big_bang` + `feedback_extraction_is_not_redesign` rules.)

## 9. Tests-mapping rule

Every component owns its tests:
- comment → quality / repair / dedup / persona tests
- agent → routing / prompt / tool / memory tests
- outbound → queue / dedup / claim / finalize / reset / policy tests
- connector → readiness / session / pairing tests

When moving files, **tests move with the component** in the same move-only PR.

## 10. Guardrail rule

Lightweight guards (warn-first, do not block CI until hotspots are triaged):
- flag a package with **> 15 `.go` files**,
- flag **> 5 same-prefix** files in one package,
- flag a new `utils.go` / `helpers.go` without a component prefix,
- keep the existing **file-size** guard (`check_file_size.py`, 200 lines),
- keep the **topology** guard (`check_topology.sh`),
- keep the **tenant-isolation** guard (`check_tenant_isolation.sh`).

`scripts/check_component_structure.py` implements the first three as **warnings only** (exit 0) against a baseline, so existing hotspots don't break CI while new growth is surfaced.

---

## Pre-add checklist (paste into the PR body when adding/moving files)

```
[ ] PR type declared (docs-only | move-only | additive | cleanup | feature)
[ ] Component owner named for each new file
[ ] Facade (public API) identified — internal helpers unexported
[ ] Tests named and mapped to the component
[ ] No new flat same-prefix peer that should be a component (rule 3)
[ ] No vague utils.go/helpers.go/manager.go without a documented single responsibility
[ ] Guards run: file-size, topology, tenant-isolation, component-structure
[ ] If structural: behavior preserved, tests moved with the component
```
