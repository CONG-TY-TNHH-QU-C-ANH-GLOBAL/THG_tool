# THG AutoFlow — Structural Refactor Plan

**Created:** 2026-05-07
**Status:** Living document. Update when each item lands or its trigger fires.
**Purpose:** Track structural decisions deferred during the 2026-05-07 cleanup audit so the codebase doesn't rot as it scales toward production multi-tenant SaaS.

---

## Why this plan exists

The 2026-05-07 audit removed 4 dead packages and consolidated 9 duplicate helpers (see *§ Quick wins shipped* below). Three structural questions stayed open because each one is either a rename across many call sites, a delete that needs production confidence, or a principle that needs to be documented before it gets re-violated.

Leaving them open is fine for a week. Leaving them open for a quarter is how the system rots: new code lands on the wrong side of a fuzzy boundary, then the boundary becomes impossible to fix without breaking 30 things at once.

This file lists each open item with: **state today → why it matters → options → recommendation → trigger to act → rollback → verification**. Anyone (including future-you) can pick up an item and execute without re-deriving the context.

---

## Decision 1 — `internal/auth` ↔ `internal/server/auth` boundary

### State today

| Package | Owns | Imports |
|---|---|---|
| [internal/auth/](../internal/auth/) | JWT sign/validate, AES-256-GCM encrypt, bcrypt password hashing, refresh token issuance, `RequireAuth` Fiber middleware | stdlib + jwt + bcrypt only |
| [internal/server/auth/](../internal/server/auth/) | HTTP routes: login, signup, refresh, logout, Google SSO, invite accept, password reset | internal/auth (aliased `authpkg`), internal/mailer, internal/store |

The two packages share the literal name `auth`. Callers that need both today write:

```go
import (
    authpkg "github.com/thg/scraper/internal/auth"
    // and the routes package as just `auth`
)
```

Workaround docstrings landed in [internal/auth/doc.go](../internal/auth/doc.go) and [internal/server/auth/doc.go](../internal/server/auth/doc.go) on 2026-05-07.

### Why it matters

- A new dev grepping `auth` lands in two unrelated places. They don't know which is the lib and which is the routes layer until they read three files.
- The `authpkg` alias is a code smell — when an alias becomes mandatory, the names are wrong.
- The next security primitive (OAuth refresh, MFA enrollment, session-revocation list) makes this worse: more code piles into `internal/auth`, more aliases pile up.

### Options

**A. (recommended) Rename `internal/auth` → `internal/security`**
- Files to touch: 10 callers (see grep result from audit) + the package itself.
- Net change: package move + import path rewrite. The struct/function APIs stay identical.
- Risk: low. Pure mechanical refactor; `go build ./...` catches every miss.
- Effort: ~30 minutes.
- Why "security" over "crypto": it owns crypto + middleware + token lifecycle, not pure crypto. "security" is the umbrella that fits without overpromising.

**B. Rename `internal/server/auth` → `internal/server/authroutes`**
- Files to touch: ~6 callers.
- Risk: low.
- Effort: ~30 minutes.
- Trade-off vs A: Smaller blast radius, but leaves the more-imported lib name (`internal/auth`) untouched, so the alias problem persists in caller code.

**C. Status quo: keep both, lean on doc.go**
- Already done. Zero effort.
- Risk: confusion grows linearly with new contributors and new security primitives.

### Recommendation

**Option A** when one of the triggers below fires. Don't pre-emptively spend the 30 minutes today.

### Trigger to act

Pick whichever comes first:
- A second engineer joins the codebase (the alias confuses 2 people, not just you).
- A third security primitive lands in `internal/auth/` (e.g., MFA, OAuth refresh, IP allowlist) — the package's scope visibly grew past "auth".
- An import alias collision blocks a feature.

### Concrete steps when executing

1. `git mv internal/auth internal/security`
2. Search-replace `"github.com/thg/scraper/internal/auth"` → `"github.com/thg/scraper/internal/security"` in all .go files.
3. Search-replace `authpkg` alias → `security` (or leave the alias, the rename is the point).
4. Update `internal/server/auth/routes.go` import.
5. Update [AGENTS.md](../AGENTS.md), [CLAUDE.md](../CLAUDE.md), [specs/PRODUCTION_FLOW.md](PRODUCTION_FLOW.md) references.
6. `go build ./... && go vet ./... && go test ./...`
7. Update [internal/auth/doc.go](../internal/auth/doc.go) → `internal/security/doc.go` content reflects new name.

### Rollback

`git revert` the rename commit. No data migration involved.

### Verification

- `go build ./...` exit 0
- `go test ./...` all green
- `grep -r 'authpkg' internal/ cmd/` returns nothing (or only intentional alias)

---

## Decision 2 — Worker path B (CDP-driven crawl) keep or drop

### State today

There are **two crawl execution paths** in the codebase:

| Path | Files | Trigger | Status |
|---|---|---|---|
| **A — Chrome Extension** (production) | [internal/server/agent/crawl.go](../internal/server/agent/crawl.go), [local-connector-extension/](../local-connector-extension/) | `cmd/scraper/crawl_runtime.go:submitConnectorCrawl` routes here when a connector is online | Live, used today |
| **B — Worker CDP** (legacy fallback) | [cmd/worker/main.go](../cmd/worker/main.go), [internal/jobhandlers/facebook_crawl/handler.go](../internal/jobhandlers/facebook_crawl/handler.go), [internal/runtime/cdp_runtime.go](../internal/runtime/cdp_runtime.go), [internal/runtime/mock.go](../internal/runtime/mock.go) | `submitConnectorCrawl` falls back to `jobStore.Submit` when no extension is online; cmd/worker polls and dispatches | Wired, but CDPRuntime is "stateless single-batch mode" — not production-grade scrape |

Both paths converge on `leadingest.IngestPost` (post-crawl classify+persist consolidated 2026-05-04).

### Why it matters

- Two execution paths = two debug surfaces. A bug in one rarely surfaces in the other; both must be reasoned about when triaging crawl failures.
- `internal/runtime/cdp_runtime.go` returns `nil, nil` on `offset > 0` — single-batch only. A crawl that ran via path B would never paginate. This is a foot-gun if path B ever silently activates.
- `cmd/worker` is a separate binary with its own deploy concerns (Docker container, scheduler poll loop, recovery loop). Worth running only if it earns its keep.
- `internal/runtime/mock.go` exists solely as a worker fallback for `ALLOW_MOCK_RUNTIME=true`. If path B goes, mock goes.
- Estimated dead/legacy footprint if dropped: ~2000 LOC across ~6 files plus the worker binary.

### Options

**A. (recommended once Chrome Extension proven stable) Drop path B entirely**
- Delete: [cmd/worker/](../cmd/worker/), [internal/jobhandlers/facebook_crawl/](../internal/jobhandlers/facebook_crawl/), [internal/runtime/cdp_runtime.go](../internal/runtime/cdp_runtime.go), [internal/runtime/mock.go](../internal/runtime/mock.go), [internal/runtime/runtime.go](../internal/runtime/runtime.go) (Runtime interface), [internal/runtime/budget.go](../internal/runtime/budget.go) (only used by handler), [internal/runtime/factory.go](../internal/runtime/factory.go).
- Modify: [cmd/scraper/crawl_runtime.go](../cmd/scraper/crawl_runtime.go) — when no connector online, return clear error `"Chrome Extension not online; pair extension and retry"` instead of falling through to `jobStore.Submit`.
- Verify before delete: confirm `internal/livesession/` has no callers outside path B. If yes, also delete it.
- Verify: `internal/jobs/` is still needed for recurring scheduler (`rememberRecurringCrawlIntents`) and skill_executions audit — keep it.
- Risk if Chrome Extension fails: no server-side fallback. Crawl returns explicit error to user, who pairs extension and retries. Acceptable trade-off because:
  - Extension errors are visible (dashboard shows offline status).
  - Path B was never actually saving us — its single-batch mode meant it produced 1-3 posts at best.
- Effort: 4–6 hours implement, 2 hours test.

**B. Keep + clear demarcation**
- Rename `internal/jobhandlers/facebook_crawl` → `internal/jobhandlers/facebook_crawl_fallback`.
- Add `// FALLBACK: do not extend; production path is internal/server/agent/crawl.go` comment at top of every path-B file.
- Document in [PRODUCTION_FLOW.md](PRODUCTION_FLOW.md) the fallback role, when it activates, and the known single-batch limitation.
- Effort: 1–2 hours.
- Trade-off: keeps the foot-gun, just labels it.

**C. Convert path B to integration-test fixture**
- Move `cdp_runtime.go` + `mock.go` under `*_test.go` build tag.
- Drop `cmd/worker` binary.
- Use as test harness only when running e2e crawl tests against a local Chrome.
- Effort: 3–4 hours.

### Recommendation

**Path A → C → drop entirely** in two steps:

1. **Now:** execute Option B (rename + comments) so the foot-gun is visibly marked. ~1 hour, low risk.
2. **After 14 days of Chrome Extension production stability** (no incidents requiring fallback): execute Option A (delete entirely).

Why two-step: deleting working fallback code should be irreversible only when you're confident the primary path holds. Two weeks of dashboard data + zero "extension offline → fallback was useful" events = safe to delete.

### Trigger to act

- **Option B (now):** before any new feature touches crawl, so we don't widen the foot-gun.
- **Option A (later):** 14 consecutive days where:
  - No crawl request fell through to path B.
  - Chrome Extension uptime ≥ 99% per dashboard.
  - No production incident required `ALLOW_MOCK_RUNTIME=true`.

### Concrete steps when executing Option A (the eventual delete)

1. Verify zero traffic to path B for 14 days (check `scheduler_jobs` table — no rows with `intent='facebook_crawl'` claimed by worker in that window).
2. `git rm -r cmd/worker internal/jobhandlers/facebook_crawl`
3. `git rm internal/runtime/cdp_runtime.go internal/runtime/mock.go internal/runtime/factory.go internal/runtime/budget.go`
4. Inspect [internal/runtime/runtime.go](../internal/runtime/runtime.go) — if Runtime interface only used by deleted files, delete; else trim.
5. Inspect [internal/livesession/](../internal/livesession/) — if no callers remain, delete.
6. Modify [cmd/scraper/crawl_runtime.go:121-148](../cmd/scraper/crawl_runtime.go) `submitConnectorCrawl`: when no online connector, return explicit error instead of `jobStore.Submit` fallback.
7. Update [cmd/scraper/main.go](../cmd/scraper/main.go): remove worker-related setup if any.
8. Update [AGENTS.md](../AGENTS.md), [CLAUDE.md](../CLAUDE.md), [PRODUCTION_FLOW.md](PRODUCTION_FLOW.md) — drop "worker fallback" mentions.
9. `go build ./... && go vet ./... && go test ./...`
10. Manual smoke: paus Chrome Extension, send crawl prompt → expect clear error, no job stuck in scheduler.

### Rollback

- Option B: `git revert` rename commit.
- Option A: `git revert` delete commit. Note: requires `cmd/worker` binary to be rebuilt and redeployed if it was running. This is why the 14-day stability gate matters.

### Verification

- `go build ./...` exit 0
- `go test ./...` all green
- Crawl smoke test: prompt via Copilot with Chrome Extension online → command #N created, extension picks it up, leads ingested.
- Failure smoke test: stop Chrome Extension → prompt returns explicit error, no row in `scheduler_jobs`.
- 24-hour monitor post-deploy: alert if any task stuck.

---

## Decision 3 — Don't create files of one helper

### State today

`internal/textutil/textutil.go` (1 file, 2 functions) and `internal/auth/doc.go` / `internal/server/auth/doc.go` (1 doc comment each) created on 2026-05-07. These passed the bar because:

- `textutil` consolidated 7+ duplicate definitions across 7 packages → genuine value, leaf utility.
- `doc.go` files document package scope, the only Go-idiomatic way to attach a package-level docstring without a feature file.

### Why it matters

It is easy for a refactor to mint micro-packages — `internal/strs`, `internal/orgctx`, `internal/sigutil` — each with one helper. The codebase rots in the opposite direction from a monolith: by file proliferation. Each new package is one more thing a new dev has to load into their head.

### Principle

A new file or package is justified ONLY when ONE of:

1. **Consolidates duplicate** — replaces ≥ 3 duplicated definitions across packages.
2. **Breaks an import cycle** — the only way to express a dependency without `interface{}` workarounds.
3. **Is an entry point or test fixture** — `cmd/*/main.go`, `*_test.go`, `doc.go`.
4. **Houses ≥ 3 related symbols** — single-purpose file with multiple types/functions of one concept.

A new file is NOT justified when:

- It would contain one helper. Add to an existing file in the same package.
- It is "for future expansion." Add code when the second symbol arrives, not before.
- It mirrors a struct from another package "to avoid import cycle" without a strong reason. (See `learning.Weights` mirror that was deleted 2026-05-07 — the mirror was the smell, not the fix.)

### Application checklist

Before creating a new file, ask in order:

1. Is there an existing file in this package with related concern? → add there.
2. Is there a `internal/textutil` (or similar leaf utility) that could host this? → add there.
3. Am I creating a package because the boundary is conceptual or because the code "looks separate"? → conceptual only.
4. Will this file have ≥ 3 symbols within the next two PRs? → if no, delay creating it.

If 3 of 4 fail, don't create the file.

### Trigger to revisit

If `find internal -name '*.go' | wc -l` grows by > 20% in a month without a corresponding feature delivery, audit for one-helper files.

### No rollback needed — this is a guideline, not code.

---

## Architectural concerns lower priority

The 2026-05-07 audit also flagged these. They aren't urgent but should not be forgotten.

### Concern A — Three-layer browser stack

[internal/browser/](../internal/browser/) (interface) + [internal/browsergateway/](../internal/browsergateway/) (provider enum) + [internal/cdpclient/](../internal/cdpclient/) (CDP endpoint discovery).

The boundary is unclear. Cause: incremental growth as features landed.

**Action:** add `doc.go` to each of the three packages, or consolidate. Trigger when adding a second browser provider (e.g., Firefox CDP) or onboarding a 2nd browser-touching feature.

### Concern B — Three-layer session lifecycle

[internal/session/](../internal/session/) (state machine) + [internal/livesession/](../internal/livesession/) (running binding) + [internal/workspace/](../internal/workspace/) (Docker container manager).

Layers ARE distinct, but a new dev needs 30 minutes to map them.

**Action:** add `doc.go` to each. Trigger when onboarding 2nd dev.

### Concern C — `agentloop` orchestration scope

[internal/agentloop/](../internal/agentloop/) docs reference "Phase 2" and an `engine` package that no longer exists (deleted 2026-05-07).

**Action:** update [internal/agentloop/](../internal/agentloop/) doc comments to reflect post-engine reality. Trigger: next time anyone touches that package.

---

## Quick wins shipped 2026-05-07

For traceability — what the cleanup PR already did, so this plan starts from a known baseline:

- **Deleted:** `internal/learning/` (orphan), `internal/parser/` (interface-only), `internal/engine/` (CI tool with no callers), `internal/antidetect/` (parallel package, never imported).
- **Deleted methods:** `AppStore.GetFacebookAccounts`, `SetFacebookAccountLoggedIn`, `ListLeadsSince`, struct `FacebookAccount`.
- **Consolidated:** `containsAny` (4 sites) and `firstNonEmpty`/`firstNonEmptyBrain` (4 sites) → `internal/textutil`. `loadBusinessProfile`/`loadBusinessProfileForOrg` (2 sites) and `splitGuidancePhrases`/`splitSignalPhrases` (2 sites) → `ai.LoadProfileForOrg` + `ai.SplitSignalPhrases`.
- **Documented:** `internal/auth/doc.go`, `internal/server/auth/doc.go`, `MockRuntime` DEV-ONLY comment, `registerScaffoldSkills` SCAFFOLD STATUS comment.

Net: ~500 LOC removed, 4 packages removed, 0 files of "one helper" created.

---

## Maintenance schedule

- **Quarterly (Mar / Jun / Sep / Dec, week 1):** re-run audit covering: (1) dead code, (2) duplicate helpers cross-package, (3) unclear package boundaries, (4) reverse-direction imports.
- **Pre-onboard:** before adding a 2nd dev, walk this file end-to-end, action items rated "trigger when 2nd dev joins" must ship before they merge their first PR.
- **Each PR:** if a PR adds a new package or new file with one symbol, reviewer cites *§ Decision 3* and asks for justification.
