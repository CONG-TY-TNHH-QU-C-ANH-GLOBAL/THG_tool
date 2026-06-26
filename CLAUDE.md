# THG AutoFlow - Claude Code Guide

Claude Code should treat `AGENTS.md` as the short instruction file and
`specs/FACEBOOK_BUSINESS_ANALYSIS_AUTOMATION_PLAN.md` as the detailed product
and implementation plan.

## Read First

1. `specs/FACEBOOK_BUSINESS_ANALYSIS_AUTOMATION_PLAN.md`
2. `openspec/root-architecture.md`
3. `specs/ROOT_ARCHITECTURE.md`
4. `AGENTS.md`

## Product North Star

Build toward:

> AI Facebook Sales Intelligence Workspace for each business.

The system is not a fixed scraper and not a spam automation tool. It learns each
organization's business, customer segments, sources, market signals, and sales
strategy. Facebook automation is used after analysis, with visible browser
sessions and human approval for risky outbound actions.

The platform should include a Workspace Skill Designer: admins describe a
Facebook-related business workflow in natural language, and the system turns it
into a validated blueprint of data entities, classifiers, dashboard views,
actions, and approval rules. Treat HR/recruitment, POD sourcing, sales lead
discovery, and similar verticals as playbooks on shared primitives, not
hardcoded scrapers.

## Current Stack

- Go backend with Gofiber.
- Next.js frontend in `frontend/`.
- SQLite for the current MVP database.
- Browser automation through persistent per-account workspaces.
- Prompt-scoped jobs through the job/task pipeline.

Do not reintroduce legacy `internal/server/static/` production UI files.

## Highest-Priority Direction

The next product work should implement:

1. Org-scoped business profiles.
2. Customer segment definitions and AI suggestions.
3. Market signals beyond simple leads.
4. Source discovery and source quality scoring.
5. Opportunity map and strategy recommendations.
6. Campaign approval and safe outbound execution.
7. Outcome learning.
8. Workspace Skill Designer and blueprint validation.
9. HR/recruitment reference blueprint.

## Hard Rules

- Every tenant feature needs `org_id` ownership checks.
- Business profile and customer segments must drive AI classification.
- Do not hardcode one industry.
- User-designed skills must compile to validated blueprints and approved
  primitives; do not execute arbitrary LLM-generated code in production.
- Do not run broad scan-all behavior.
- Browser automation must be observable.
- Default outbound automation to approval-required.
- Return `human_required` on login wall/checkpoint.
- Do not generate AI images. Use real uploaded files/images only.

## Engineering Guardrails

These rules are binding for every implementation, refactor, and feature PR.

### Code size and modularity

* Do not create new production code files over **200 lines**.
* Do not grow existing large files unless explicitly approved.
* If a feature needs more than 200 lines, split it into smaller modules.
* Legacy large files are tolerated temporarily, but every touch should move them toward extraction, not make them worse.
* Generated files, migrations, schema/bootstrap files, fixtures, and intentionally large test data may be exceptions, but the completion report must state why.
* The rule is enforced by `scripts/check_file_size.py` (**baseline-aware**): a new / non-allowlisted production file over 200 lines **FAILs**; legacy files that already exceeded the limit are listed in `scripts/file_size_allowlist.txt` and only **WARN**. The allowlist is a **temporary baseline, not a licence to keep growing god files** — never add a NEW file to it to dodge the limit, and remove a path once the file is split to <= 200 lines.

### SOLID, DRY, and SRP

* Apply DRY and SOLID principles by default.
* Each function, class, component, package, or module must have one clear responsibility.
* Do not duplicate logic. If the same logic appears twice, extract it into a reusable helper, hook, service, policy, or domain function.
* Do not mix unrelated layers:

  * UI rendering and API calls
  * business logic and transport handlers
  * storage queries and policy decisions
  * generic DOM helpers and platform-specific selectors
  * orchestration and execution
  * proof/evidence collection and action execution

### Feature-based structure

Organize code by feature/domain, not by dumping everything into large generic files.

Frontend feature folders should prefer:

```text
components/
hooks/
services/
types/
utils/
```

Backend domain folders should separate:

```text
contracts/models
store/repository
policy/evaluator
handlers/transport
tests
```

Browser extension code should separate:

```text
core/        reusable DOM/click/type/wait primitives
runtime/     action routing and execution context
platforms/   Facebook/Taobao/1688-specific selectors and actions
content/     thin bridge/entrypoint only
```

### Component structure (think in components before adding files)

**Binding:** before adding OR moving any file, classify the component owner and check
`specs/COMPONENT_STRUCTURE_RULES.md`. A package is a bounded component, not a flat
dumping ground of same-prefix peers (`comment_*`, `agent_*`, `business_*`). For each
new file, name: (1) the owning component, (2) its public facade, (3) the tests that
prove it, (4) the boundary it must not cross. A package trips review when it has >15
`.go` files or >5 same-prefix files — triaged in `specs/COMPONENT_HOTSPOTS.md` and
surfaced (warn-only) by `scripts/check_component_structure.py`. Structural refactors
are move-only/wrapper-first and declare their PR type (no big-bang).

### No god files

Do not add more responsibility to files that are already large.

Before editing any file over 300 lines, check whether the change can be extracted into a smaller module.

Known risky areas:

* `local-connector-extension/content/outbound.js`
* large backend handlers
* large frontend view components
* schema/bootstrap files

Touching a legacy large file requires explaining:

1. why the edit cannot be extracted now;
2. whether the file grew;
3. what future extraction should happen.

### Proactive refactoring

Before marking any implementation complete, self-review:

* Did I introduce duplicate logic?
* Did I grow a large file?
* Can any new logic be a pure function?
* Is this platform-specific or generic?
* Is this UI, business logic, storage, or transport mixed together?
* Are typed reason codes centralized?
* Are tests protecting the domain contract?

If the review reveals coupling or duplication, refactor before declaring done.

### Track separation

Do not mix unrelated architecture tracks in one PR.

Current tracks include:

* Facebook Automation Reliability
* Comment Intelligence
* Browser Automation Kit
* Omnichannel Sales Copilot / Telegram
* KnowledgeOS / Business Profile / Workspace Skill Designer

A PR should state which track it belongs to.

### Refactor vs behavior change

* A **refactor-only PR must NOT change behavior** — move / rename / split / re-namespace only. No "while I'm here" logic or selector fixes.
* A **behavior-changing PR must have tests** protecting the new behavior + its typed reason codes.
* State in the completion report whether the PR is behavior-changing or refactor-only.

### AI and Knowledge grounding

AI must not invent business facts.

Every concrete claim in outbound copy must be grounded by one of:

* KnowledgeOS asset
* catalog SKU
* pricing rule
* CTA asset
* company identity
* uploaded proof/media
* verified business profile data

Do not invent:

* price
* website
* email
* phone number
* proof/case study
* fulfillment capability
* delivery promise

If knowledge is missing, degrade honestly or return a typed reason such as `knowledge_gap`.

### Telegram and external interfaces

Telegram is an interface, not a separate business logic path.

Telegram commands must go through the shared backend:

```text
ActionContext → Readiness/PolicyGate → Execution/Ledger
```

Do not copy web command logic into Telegram handlers.

### Browser automation

Browser automation must keep generic and platform-specific code separate.

* Generic click/type/wait/visibility helpers belong in extension `core/`.
* Facebook selectors and Facebook identity logic belong in `platforms/facebook/`.
* Future Taobao/1688 logic must not duplicate core helpers.
* Evidence-on-failure must be preserved even if live stream/remote-control is removed.

### Completion report requirement

Every implementation report must include:

1. files changed;
2. whether any file exceeds 200 lines;
3. whether any large legacy file was touched;
4. what logic was extracted or reused;
5. any intentional exception to these rules;
6. tests/builds run;
7. whether the PR changed behavior or was refactor-only.


## Verification

Run the relevant checks after changes:

```powershell
python scripts/check_file_size.py
go test ./...
go vet ./...
npm --prefix frontend run build
```

## Operating Protocol (AI-assisted development)

This section lets Claude self-operate from the repo without long external
prompts. When the user says **"Execute NEXT from `docs/ai/AUTOPILOT_QUEUE.md`"**,
pick the first `READY` item (unless the user names a different item) and run it
end-to-end under the rules below. One queue item = one PR.

### Discipline

- **Ponytail / Lazy Senior Dev**: write the least code necessary; prefer moving
  or characterizing existing code over rewrites; no new abstraction, broad
  interface, helper soup, or framework rewrite; no noisy diff or formatting
  churn outside touched lines.
- **Refactor vs behavior**: a refactor-only PR must not change behavior; a
  behavior-changing PR must add tests protecting the new behavior + reason codes.
  State which in the report.

### Traffic-light classification

Classify each candidate before touching it:

- **GREEN** — pure / behavior-preserving / no queue·ledger·CAS·lease·auth·schema
  ·DTO dependency. Safe to move or characterize.
- **YELLOW** — domain logic that needs a *narrow, consumer-owned* port/adapter to
  move safely. Only proceed if the seam stays tiny; otherwise classify RED.
- **RED** — queue/outbox writes, action_ledger/execution_attempts, connector
  CAS/lease, crawler/jobhandler runtime, retry/scheduling, auth/session/cookie,
  schema/migrations, DTO/wire contracts. **Do not change semantics.** Characterize
  only; never "make it movable" by inventing abstractions.

### Boundary laws

- `internal/services/facebook` must not import `internal/store`, `internal/server`,
  `cmd/scraper`, `internal/connectors`, `internal/jobhandlers`, `internal/leadingest`,
  or sibling verticals. Adapters live in the composition root (`cmd/scraper`,
  `cmd/worker`).
- Neutral/internal packages must not import `internal/services/facebook` (reverse
  guard `NEUTRAL_NO_SERVICES_FACEBOOK_IMPORT`). `internal/fburl` stays a neutral leaf.
- Do not weaken or reconfigure the boundary guards; do not change Sonar config.

### Sonar

QA/QC only. Fix in-scope New Code issues in files you already touched, under
Ponytail discipline. No suppressions, no config changes, no chasing unrelated
backlog.

### Standard validation

Run `bash scripts/ai_preflight.sh` before editing and `bash scripts/ai_validate.sh`
before pushing (these wrap `go test ./...`, `go build ./...`, `go vet ./...`, the
import-boundary + file-size guards, and `git diff --check`). Keep `.mcp.json` and
`specs/knowledge/RETRIEVAL_SOAK_REPORT.md` (a test artifact) out of commits.

### Stop conditions

Stop and report (do not force it) when: the task is RED and would change the
forbidden semantics above; a safe move needs a broad port/abstraction; tests would
need a large new fixture/mock framework or real browser/Chrome/network I/O; the
diff becomes hard to review; or the behavior is ambiguous. A precise stopped report
is a successful outcome.

### Push rules

Push only after `ai_validate.sh` passes. **Never merge.** Report using
`docs/ai/AGENT_REPORT_TEMPLATE.md`.
