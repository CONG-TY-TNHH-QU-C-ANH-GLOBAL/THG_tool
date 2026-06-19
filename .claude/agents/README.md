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
constraints, the explicit forbidden/high-risk areas, an output checklist, and the rule that
high-risk areas get a **characterization-test-first plan only**.

## Forbidden / high-risk areas (shared by every agent)

`cmd/scraper/outbound_actions.go`, `cmd/scraper/main.go`, connector claim/CAS/lease,
`action_ledger` / `execution_attempts`, policy/readiness gates, auth/admin/tenant isolation,
migrations, `internal/server/agent/*`, workspace CDP/session/connector flows,
`queueLeadOutreach`, `runPooledOutreach`, `commentSinglePost`, `guardFacebookWriteAccount`,
`queueGroupPost`, `queueProfilePost`, the Phase D typed `CommandBus` (unless explicitly
approved), and `.mcp.json`.

## Note on version control

`.claude/` is listed in `.gitignore`, so these agent files are **force-added** (`git add -f`)
to track them — matching how `.claude/commands/opsx/*` and `.claude/skills/openspec-*` are
already tracked. New agent files added later must also be force-added.
