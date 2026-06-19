# THG AutoFlow — Claude Code sub-agents

Project-level Claude Code sub-agents specialized for this repository, runnable as official
`.claude/agents/*.md` agents. They encode a **two-layer design**: a professional specialist
framework on top, and the THG operating policy (architecture invariants, controlled high-risk
zones, hard rules) underneath — so automated work stays safe and behavior-preserving.

## Final agent list

| Agent | Role |
|---|---|
| `sonar-triage` | Read-only Sonar backlog triage into risk lanes; recommends the smallest safe sprint. |
| `senior-architect` | Architecture / boundary decisions and staged refactor plans; **holds the architecture veto**. |
| `senior-backend` | Go backend implementation of low-risk, behavior-preserving changes. |
| `senior-frontend` | Next.js/React/TS UI, routing, service-scoped UI, accessibility, frontend Sonar work. |
| `senior-fullstack` | Cross-boundary FE↔BE integration; **coordinates, does not replace specialists**. |
| `senior-data-engineer` | SQLite store, KnowledgeOS retrieval/embedding, read-model projections, SQL/data correctness. |
| `security-review` | Auth/tenant/secrets/injection/cookie-session security audit; report-and-propose first. |
| `senior-devops` | Dockerfile / CI / build & release hygiene; preserves build+deploy semantics exactly. |
| `qa-test-engineer` | Validation, focused behavior-pinning tests, characterization tests before high-risk changes. |
| `code-reviewer` | Final diff gate; verifies each change maps to a selected issue and contracts held. |

## Layer 1 — professional framework

Each agent behaves like a real-world specialist (backend, frontend, fullstack, data engineer,
security reviewer, QA/test engineer, DevOps/release engineer, architect, Sonar triage lead, code
reviewer) and carries the professional depth of that discipline.

## Layer 2 — THG operating policy

Each agent also understands repo-specific constraints: `org_id` tenant isolation, the outbound
safety spine, connector claim/CAS/lease, `action_ledger` / `execution_attempts`, policy/readiness
gates, auth/admin/tenant isolation, migrations, the Sonar Quality Gate, CI/CD, the rule that
`.mcp.json` must never be committed, and the principle that high-risk zones are **gated, not
forbidden forever**.

## How the roles relate (read before Sprint 3)

- **`senior-fullstack` coordinates cross-layer work but does not replace specialist reviews** — it
  must call out when backend, frontend, data, security, QA, or architect review is required.
- **`senior-frontend` owns UI / client-side / frontend Sonar work** (UX and copy preserved).
- **`senior-data-engineer` owns SQL / data / retrieval / embedding correctness.**
- **`senior-architect` has veto over architecture risk** (boundaries, dependency direction, spine, outbox).
- **`code-reviewer` is the final diff gate.**
- **`security-review` is required for auth / security / tenant-sensitive work.**

Use the triage → specialist → review chain: `sonar-triage` proposes a lane, the matching
specialist implements (or plans), `qa-test-engineer` validates, and `code-reviewer` gates the diff.

## Binding protocol — read before any Sonar cleanup

All Sonar Factory work is governed by the
[**THG Sonar Factory Operating Protocol**](../../docs/architecture/SONAR_FACTORY_PROTOCOL.md)
(`docs/architecture/SONAR_FACTORY_PROTOCOL.md`). It is **mandatory** and defines the
risk classes (P/S/F/M), batch-size budgets, main-stability and parallel-PR rules,
the zero-New-Issues rule, the no-blind-deletion and JS/TS scope-altering rules, the
noisy-diff rule, the §10 agent workflow (Phase A→E maps onto the agents above), git
traceability, validation, next-sprint selection, and the required final report.
Follow it for every Sonar PR; the agent roles here are the executors of that protocol.

## Runtime healthcheck

During Sprint 3, named subagents became unavailable mid-session, so work
continued via direct-role execution as a fallback. Before and during a sprint:

- After restarting Claude Code, run `/agents` and confirm all 10 project agents
  above are listed.
- Run `/status` to confirm the active main model.
- Agents must **inherit the main model** — frontmatter `model` is omitted or
  `model: inherit`, and `CLAUDE_CODE_SUBAGENT_MODEL` is unset or `inherit`.
- If subagents drop mid-session, **stop high-risk work and restart** the
  session; do not push on with fallback.
- Direct-role fallback is allowed **only** for low-risk type-only (Lane F) or
  mechanical/devops (Lane A) work that touches no controlled zone.
- Direct-role fallback is **forbidden** for Security, Reliability bugs, and any
  controlled high-risk zone (auth/session/tenant, outbound/connector/ledger,
  migrations, backend runtime, API contracts, fullstack flows).

See [`HEALTHCHECK.md`](./HEALTHCHECK.md) for the full verification and recovery
runbook.

## Provenance

The `latest` `claude-code-templates` CLI installer is currently broken on this host
(`ERR_REQUIRE_ESM` — its `index.js` `require()`s the ESM-only `@clack/prompts`), so each
agent was **adapted from the upstream template content** (fetched from
`davila7/claude-code-templates`) rather than installed by the CLI:

| Agent | Upstream base template |
|---|---|
| `sonar-triage` | `development-tools/technical-debt-manager.md` |
| `senior-architect` | `development-team/backend-architect.md` |
| `senior-backend` | `development-team/backend-developer.md` |
| `senior-frontend` | `development-team/frontend-developer.md` |
| `senior-fullstack` | `development-team/fullstack-developer.md` |
| `senior-data-engineer` | `data-ai/data-engineer.md` |
| `security-review` | `security/security-auditor.md` |
| `senior-devops` | `devops-infrastructure/devops-engineer.md` |
| `qa-test-engineer` | `development-tools/test-engineer.md` |
| `code-reviewer` | `development-tools/code-reviewer.md` |

Each file keeps the professional core of its base template, then adds: THG AutoFlow constraints,
the **controlled high-risk zones** (gated, not permanently forbidden), an output checklist, and the
rule that controlled zones get a **characterization-test-first plan only** unless the sprint prompt
supplies an explicit override.

## Controlled high-risk zones (shared by every agent)

These are **controlled, not banned forever.** Default during any generic cleanup/refactor/Sonar
sprint: do **not** edit — produce a characterization-test-first plan only. A zone becomes editable
only when the current sprint prompt explicitly approves and supplies all six: (1) exact
files/functions, (2) required characterization tests, (3) expected behavior contracts,
(4) rollback plan, (5) required reviewer roles, (6) user approval before implementation.

Zones: `cmd/scraper/outbound_actions.go`, `cmd/scraper/main.go`, connector claim/CAS/lease,
`action_ledger` / `execution_attempts`, policy/readiness gates, auth/admin/tenant isolation,
migrations, `internal/server/agent/*`, workspace CDP/session/connector flows,
`queueLeadOutreach`, `runPooledOutreach`, `commentSinglePost`, `guardFacebookWriteAccount`,
`queueGroupPost`, `queueProfilePost`, and the Phase D typed `CommandBus`. Frontend flows add:
auth/session UI, billing/access gates, workspace switching, connector/session UI, and
automation-trigger UI.

## Hard rules (shared by every agent — these stay hard)

Never commit `.mcp.json` or secrets; never lower a Sonar Quality Gate threshold; never mark a
Sonar issue accepted/won't-fix/false-positive without approval; never merge without user
approval; do not modify behavior outside the approved sprint scope; do not delete files casually;
do not start the Phase D typed `CommandBus` unless explicitly approved.

## Note on version control

`.claude/` is listed in `.gitignore`, so these agent files are **force-added** (`git add -f`)
to track them — matching how `.claude/commands/opsx/*` and `.claude/skills/openspec-*` are
already tracked. New agent files added later must also be force-added.
