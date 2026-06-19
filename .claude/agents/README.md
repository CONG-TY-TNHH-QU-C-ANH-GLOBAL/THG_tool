# THG AutoFlow — Claude Code sub-agents

Project-level Claude Code sub-agents specialized for this repository. They encode the
"one risk lane per PR" cleanup doctrine, the THG architecture invariants, and an explicit
forbidden/high-risk list so automated refactors stay safe and behavior-preserving.

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
| `senior-data-engineer` | `data-ai/data-engineer.md` |
| `security-review` | `security/security-auditor.md` |
| `senior-devops` | `devops-infrastructure/devops-engineer.md` |
| `qa-test-engineer` | `development-tools/test-engineer.md` |
| `code-reviewer` | `development-tools/code-reviewer.md` |

Each file keeps the professional core of its base template, then adds: THG AutoFlow
constraints, the **controlled high-risk zones** (gated, not permanently forbidden), an output
checklist, and the rule that controlled zones get a **characterization-test-first plan only**
unless the sprint prompt supplies an explicit override.

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
`queueGroupPost`, `queueProfilePost`, and the Phase D typed `CommandBus`.

## Hard rules (shared by every agent — these stay hard)

Never commit `.mcp.json` or secrets; never lower a Sonar Quality Gate threshold; never mark a
Sonar issue accepted/won't-fix/false-positive without approval; never merge without user
approval; do not modify behavior outside the approved sprint scope; do not delete files casually;
do not start the Phase D typed `CommandBus` unless explicitly approved.

## Note on version control

`.claude/` is listed in `.gitignore`, so these agent files are **force-added** (`git add -f`)
to track them — matching how `.claude/commands/opsx/*` and `.claude/skills/openspec-*` are
already tracked. New agent files added later must also be force-added.
