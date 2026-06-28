---
doc_type: architecture
status: active
owner: platform
last_reviewed: 2026-06-28
related_pr_or_issue: chore/docs2-architecture-backlinks-frontmatter
---

# THG Sonar Factory Operating Protocol

> Part of the [architecture docs index](INDEX.md).

**Status:** binding SOP. Mandatory for **all** future Sonar cleanup work in
`THG_tool`.
**Scope:** how to burn down the large Sonar backlog at high throughput **without
creating production risk**.
**Companions:** [`REFACTOR_ROADMAP.md`](./REFACTOR_ROADMAP.md) (sprint log /
history), [`.claude/agents/README.md`](../../.claude/agents/README.md) (the agent
roles that execute this protocol). This doc is the **policy**; the roadmap is the
**record**.

> This file is documentation only. Following it must never change application
> behavior, Sonar Quality Gate thresholds, or CI behavior.

## Core principle

Do **not** optimize for the raw number of Sonar issues fixed.

Optimize for:

- same Sonar rule family
- same risk lane
- limited file count
- low conflict risk
- behavior-preserving changes
- no controlled-zone surprise
- no noisy diff
- **zero avoidable New Code Sonar issues before merge**
- clear rollback traceability

Old backlog can be burned down over time. New PRs must **not** introduce
avoidable new Sonar issues into `main`.

---

## 1. Risk classes

### Class P — Product / customer correctness bugs

*Examples:* connector pairing stuck, Comment AI wrong contact, Facebook
readiness/status issues, customer-visible workflow blockers.

- 1 bug per PR; no unrelated Sonar cleanup mixed in.
- Full validation; manual E2E required.
- Wait for `main` CI/CD green before starting another production-sensitive branch.
- Architecture / security review required if touching controlled paths.

### Class S — Security / Reliability

*Examples:* auth/session/cookie, tenant isolation, nil/panic, wrong error
handling, permission bypass, secrets/logging, unbounded retry/timeout, resource leak.

- Small scoped PR; test-first when behavior-risky.
- Security-review required; no broad batching.
- No false-positive / won't-fix without explicit user approval.
- No Quality Gate threshold changes.

### Class F — Sonar Factory low-risk cleanup

*Examples:* frontend type-only fixes, TypeScript/JS mechanical maintainability,
Dockerfile consistency, test-only complexity, readonly props, simple local helper
extraction, simple prompt/test complexity extraction if behavior is pinned.

- High throughput allowed: **20–60 issues per PR if safe**.
- **Max 7–12 files per PR** when AI edits manually.
- Same rule family preferred; same directory/layer preferred.
- No controlled zones; no business behavior change.
- No UX/copy change unless explicitly part of the selected issue.
- Zero avoidable New Code Sonar issues before merge.

### Class M — Migration / schema / data ownership

*Examples:* database migrations, schema changes, table ownership changes,
backfill/replay logic, data ownership changes.

- **Serial only — one migration PR at a time. No parallel migration PRs.**
- No batching with Sonar cleanup.
- Requires expand/migrate/contract or rollback plan.
- Requires explicit user approval before implementation. **No AI autopilot.**

---

## 2. Batch size policy

Do not choose batch size by issue count alone. Use
**issue count + file count + risk lane + rule family**.

| Lane | Issues/PR | Files/PR | Constraints |
|---|---|---|---|
| Frontend type-only | 20–60 | 7–12 | same rule family; no API contract / auth / billing / connector flow (e.g. readonly props, pure type annotations) |
| Frontend mechanical, JSX-visible | 10–25 | 5–8 | no copy/design/route changes unless approved; no behavior hidden in JSX cleanup |
| Backend pure helper / read-model | 5–15 | 3–6 | no DB write semantics; no policy/readiness/outbound/ledger/connector behavior; no auth/session behavior; prefer same-package private helper extraction |
| Backend runtime / customer path | 1 bug or tiny cluster | — | tests required; architecture/security gate where relevant |
| Migration / auth / outbound / connector | 1 scoped PR | — | no parallel work; no bulk cleanup |

**Codemod exception.** A deterministic codemod/script with tests may touch more
files, but only after: a clear plan, a diff preview, user approval, and validation
proving behavior preservation.

**AI manual-edit warning.** If 100 issues are spread across 40 files, split the
batch. If 50 issues are in 5–10 files of the same safe rule family, it may be
acceptable.

---

## 3. Main stability policy

**Never build on a broken `main`.**

Before starting any product-sensitive branch:

1. `git checkout main`
2. pull latest `main`
3. confirm `main` CI/CD is green
4. confirm working tree clean
5. confirm `.mcp.json` is untracked/unstaged

For low-risk Sonar Factory branches, work may be prepared while `main` CD is
running **only if** the branch is isolated, low-risk, does not depend on the
deployment result, and does not touch production-sensitive paths. Before
opening/pushing the PR, rebase or pull latest green `main`.

If `main` CI/CD fails: **freeze new merges**, stop new feature/customer-sensitive
work, diagnose and fix `main` first, and do not continue building
production-sensitive branches on a broken `main`.

*Alerting:* `main` CI/CD failure should be surfaced via GitHub notification,
Slack, Telegram, or equivalent. If no alerting exists, the operator must manually
check `main` before starting sensitive work.

---

## 4. Parallel PR policy

**Allowed:**

- At most **2–3 active low-risk Sonar Factory PRs**.
- Each PR must touch **different file clusters or different safe rule families**.
- No overlapping hot files.

**Not allowed:**

- 25 active PRs at once; parallel PRs touching the same files.
- Parallel PRs containing migrations.
- Parallel PRs touching auth/session/tenant/outbound/connector/ledger.
- Parallel product bug PRs unless explicitly coordinated.

**Conflict rule:** if two PRs may touch the same file, serialize them.

**Merge sequencing:** low-risk Sonar PRs may be reviewed in parallel;
product/customer bug PRs are merged carefully after CI/Sonar/manual E2E. If a
merge makes `main` red, **stop all factory work and fix `main`**.

---

## 5. Zero New Issues policy

Backlog issues are handled by the Sonar Factory over time. But new PRs must not
add avoidable new Sonar issues to `main`.

If Sonar reports New Code issues on the current PR:

- Fix them in the same PR if small and behavior-preserving.
- Do **not** mark false-positive / won't-fix without approval.
- Do **not** lower the Quality Gate.
- Do **not** merge with avoidable new issues.

If a new issue cannot be fixed safely: report why, ask for a user decision, and do
not silently accept or suppress it.

> Old backlog can remain temporarily. Avoidable New Code issues should be **zero**
> before merge.

---

## 6. Controlled zones

**Default: no Sonar Factory bulk cleanup in controlled zones.**

Controlled zones include:

- `cmd/scraper/outbound_actions.go`
- `cmd/scraper/main.go`
- connector claim/CAS/lease
- `action_ledger` / `execution_attempts`
- policy/readiness gates
- auth/admin/tenant isolation
- migrations
- `internal/server/agent/*`
- workspace CDP/session/connector flows
- `queueLeadOutreach`, `runPooledOutreach`, `commentSinglePost`,
  `guardFacebookWriteAccount`, `queueGroupPost`, `queueProfilePost`
- the Phase D typed `CommandBus`

These are **gated, not forbidden forever.** Editable only when the current sprint
explicitly provides all six:

1. exact files/functions
2. required characterization tests
3. expected behavior contracts
4. rollback plan
5. required reviewer roles
6. explicit user approval before implementation

For controlled zones: no autopilot, no broad cleanup, no unrelated Sonar fixes,
test-first plan required.

---

## 7. No blind deletion policy

Sonar rules that suggest deleting unused/dead code are **not** automatically safe —
e.g. `S1481` (unused local), `S1144` (unused private methods), unused parameters,
unused struct fields, unused JSON fields/tags, dead branches,
duplicate/unreachable code.

Do **not** blindly delete variables, parameters, struct fields, JSON tags,
exported symbols, event handlers, manifest fields, DTO fields, or dead-looking
branches. Before deleting, prove the symbol is not required by:

- external API payload shape
- JSON serialization/deserialization
- reflection
- interface compliance
- framework conventions
- Chrome extension messaging
- Facebook DOM/payload integration
- tests/fixtures/golden outputs
- public API compatibility
- analytics/webhook/event schemas

If proof is weak, **do not delete** — ask for approval or defer. Prefer
behavior-preserving fixes: keep public/external payload shape stable, add an
explanatory comment if a field is intentionally retained, and narrow only truly
local, private, proven-unused code. **Never delete code in controlled zones during
Sonar Factory cleanup.**

Any PR fixing unused/dead-code rules must include: exact symbols deleted, why each
is safe to delete, proof no external contract depends on it, and tests/validation
run.

---

## 8. Scope-altering JavaScript / TypeScript changes

Some JS/TS Sonar fixes look mechanical but can change runtime semantics:
`var → let/const`, moving declarations, collapsing conditions, simplifying
closures, changing optional chaining/nullish coalescing, changing equality rules,
replacing loops with map/filter/reduce, changing async/await promise flow.

Treat `var → let/const` as **scope-altering, not blindly mechanical**. Before
changing it, explicitly verify: no hoisting dependency, no Temporal Dead Zone
issue, no closure behavior change, no loop/callback capture change, no
global/window binding dependency, no extension content-script lifecycle issue.

- Prefer `const` only when the variable is never reassigned.
- Prefer `let` only when reassignment is required and block scope is safe.
- If scope behavior is unclear, **defer** the issue.
- For Chrome extension scripts and Facebook DOM automation code, be extra conservative.

Any such PR must report: variables changed, why scope semantics are preserved,
validation commands run, and whether a browser/manual check is needed.

---

## 9. Noisy diff / formatting churn policy

A Sonar PR that should change 2 lines must **not** rewrite 500 lines because of
unrelated formatting, quote-style changes, import reordering, line wrapping, or
whitespace churn.

- Do **not** make unrelated formatting changes (quote style, semicolons, import
  order, whitespace, line wrapping, file formatting) unless required by the
  selected issue or the existing project formatter.
- Do **not** run broad formatters across unrelated files. Format only changed Go
  files with `gofmt` when required.
- For frontend/TS/JS, do not run a whole-file-rewriting formatter unless the
  project already enforces it and the PR is explicitly a formatting PR.
- Keep diffs mapped to the selected Sonar issue lines. If a file is opened only to
  fix one issue, touch only the minimum lines for that issue. No opportunistic
  cleanup. No import sorting unless necessary for compile/typecheck. No copy/design
  changes unless approved.

**Review rule:** a PR with noisy diff unrelated to the selected rules should be
rejected / sent back for changes — revert the formatting churn, keep only the
exact changes for the selected rule.

**Fix-up instruction:** *"Revert formatting changes. Only touch the exact lines
needed for the selected Sonar rule. Preserve existing quote style, import order,
whitespace, and formatting unless required by compilation or the selected rule."*

---

## 10. Agent workflow

Every Sonar Factory PR runs the chain. No agent self-approves its own
implementation. See [`.claude/agents/README.md`](../../.claude/agents/README.md)
for the role definitions.

- **Phase A — `sonar-triage`:** inventory open issues; group by rule/path/risk
  lane; recommend **one** batch; list exact issue keys; estimate file count and
  conflict risk; identify deletion / scope-altering / noisy-diff risks; exclude
  controlled zones unless explicitly approved.
- **Phase B — `senior-architect`:** verdict **ALLOW / ALLOW WITH CONDITIONS /
  VETO**; check controlled zones, dependency boundaries, behavior risk, batch
  size, file count, deletion/scope proof needs, expected diff shape.
- **Phase C — specialist implementation:** `senior-frontend` (frontend),
  `senior-backend` (backend), `senior-data-engineer` (data/read-model),
  `senior-devops` (Docker/CI), `security-review` (security-sensitive),
  `senior-fullstack` (cross-layer).
- **Phase D — `qa-test-engineer`:** run correct validation; add
  characterization/unit tests where needed; report missing scripts honestly;
  remove artifacts; confirm `.mcp.json` not staged.
- **Phase E — `code-reviewer`:** final gate — verify only selected issues changed,
  no unrelated cleanup, no controlled-zone surprise, behavior contract held,
  deletion proof if anything removed, JS/TS scope proof if semantics could change,
  no noisy diff, New Code issues zero or explained.

---

## 11. Git traceability and rollback

Every Sonar Factory PR must be easy to identify and revert.

- PR **title** must include the main Sonar rule ID(s).
- Commit message must include the main Sonar rule ID(s).
- PR body must list: rule IDs, issue count fixed, file count changed, risk lane,
  validation, rollback risk.
- Prefer **one rule family per PR**. If multiple, limit to 2–3 closely related
  low-risk rules. More than 3 rule families ⇒ split.

**Example titles**

```
refactor(frontend): fix Sonar S6759 readonly props
refactor(frontend): fix Sonar S6582 optional chaining
refactor(test): reduce Sonar S3776 test complexity
chore(devops): fix Sonar Dockerfile issues
```

**Branch naming**

```
refactor/sonar-s6759-readonly-props-batch-1
refactor/sonar-s6582-optional-chaining-batch-1
refactor/sonar-s3776-test-complexity-batch-1
```

If production regresses after merge, the operator should identify the responsible
rule family from git history without opening every diff.

---

## 12. Validation rules

**Go / backend:** `gofmt` changed Go files · `go vet ./...` · `go build ./...` ·
`go test ./...` · `git diff --check` · `python scripts/check_file_size.py` if
present · check import/topology/tenant scripts if present · revert
`specs/RETRIEVAL_SOAK_REPORT.md` if tests rewrite it · remove coverage/build
artifacts.

**Frontend:** detect package manager from lockfile/`package.json` · run existing
typecheck script if present · run existing build script if present · run existing
tests if present · lint only if non-interactive/configured · `git diff --check` ·
no generated build artifacts staged.

**Chrome extension:** run extension/unit tests if present · run extension build if
present · check manifest changes carefully · verify `host_permissions` if relevant
· manual browser test required for customer-facing extension flows.

**Always:** `.mcp.json` must remain untracked/unstaged · no secrets · no generated
artifacts · no Quality Gate threshold changes · no false-positive / won't-fix
without approval.

---

## 13. Next Sonar Factory sprint selection

After the current product/customer PR is merged and `main` CI/CD is green, propose
the next sprint.

**Target:** 30–50 issues · max 7–12 files · same rule family · same layer · no
controlled zones · zero expected runtime behavior change · zero avoidable New Code
issues · minimal reviewable diff.

**Preferred candidates:** TypeScript/React type-only maintainability · JS/TS
mechanical code smells *only after a scope audit* · Dockerfile consistency ·
test-only complexity · backend pure helper extraction only if small and isolated.

**Before editing, print:** selected rule family · Sonar rule ID · issue count ·
file count · exact issue keys · files to change · whether any deletion is involved
· whether any JS/TS scope-altering change is involved · expected diff shape · why
this batch is safe · excluded high-risk items · validation plan · expected Sonar
impact · conflict risk · rollback risk · architecture verdict.

Proceed only after **ALLOW** or **ALLOW WITH CONDITIONS**. Do not merge. Do not
start the following sprint.

---

## 14. Required final report (per Sonar Factory PR)

Every PR must report: branch · commit(s) · PR title · Sonar rule IDs fixed · issue
count fixed · files changed · file count · risk class · whether deletion occurred ·
whether JS/TS scope semantics changed · expected diff shape · actual diff stat ·
whether any formatting-only changes occurred · confirmation that no unrelated
formatting/import/quote/whitespace churn was introduced · behavior-preservation
proof · validation results · New Code issue status · controlled zones excluded ·
rollback risk · `.mcp.json` status · recommendation:

- **READY TO MERGE** after CI/Sonar green
- **NEEDS FIX-UP**
- **DO NOT MERGE**

Do not merge without user approval. Do not start another sprint unless explicitly
approved.
