# Claude Agent Runtime Healthcheck

Tooling-only runbook for verifying that THG AutoFlow's project subagents are
loaded and healthy, and for recovering safely when named subagents drop
mid-session.

> Context: During Sprint 3, named subagents became unavailable mid-session and
> work continued via direct role execution as a fallback. This guide makes that
> failure visible early and bounds what fallback is allowed.

## 1. Verify project agents are visible

1. Run `/agents` in Claude Code.
2. Confirm the **Project agents** section lists all 10 agents below.
3. If any are missing, treat the session as degraded and follow §4 (recovery).

### Expected final agent list

- `sonar-triage`
- `senior-architect`
- `senior-backend`
- `senior-frontend`
- `senior-fullstack`
- `senior-data-engineer`
- `security-review`
- `senior-devops`
- `qa-test-engineer`
- `code-reviewer`

All 10 live in `.claude/agents/<name>.md`, where the file stem equals the
agent `name`.

## 2. Confirm model inheritance

Project agents must run on the **main session model** (no silent downgrade).

- Each agent's frontmatter `model` field must be either **omitted** or set to
  `model: inherit`. It must **not** be pinned to `sonnet`, `haiku`, or `fable`.
- The environment variable `CLAUDE_CODE_SUBAGENT_MODEL` must be **unset** or
  `inherit`. If it is set to a specific smaller model, agents silently run on
  that model — unset it (do not edit the shell profile without approval).

Quick checks:

```bash
# every agent should print "inherit" (or have no model line at all)
grep -H '^model:' .claude/agents/*.md

# should print "(unset)" or "inherit"
echo "${CLAUDE_CODE_SUBAGENT_MODEL:-(unset)}"
```

Run `/status` to confirm the active main model for the session.

## 3. Quick frontmatter sanity sweep

If agents fail to load, the cause is usually malformed frontmatter:

```bash
# frontmatter must open and close with --- ; name must match the file stem
for f in .claude/agents/{sonar-triage,senior-architect,senior-backend,senior-frontend,senior-fullstack,senior-data-engineer,security-review,senior-devops,qa-test-engineer,code-reviewer}.md; do
  head -1 "$f" | grep -q '^---' || echo "MISSING OPEN DELIM: $f"
done
```

Common breakers: missing closing `---`, a broken quote in `description`,
duplicate YAML keys, an unknown tool name in `tools`, or a stray parsing issue.
Note: the repo stores these files as **LF** (`git ls-files --eol`); a Windows
working-tree CRLF checkout via `core.autocrlf=true` is expected and is
normalized back to LF on commit — it is not a defect.

## 4. Recover when agents disappear mid-session

If named subagents vanish while you are working:

1. **Stop** the current task.
2. **Do not** continue high-risk work using direct-role fallback.
3. **Restart** the Claude Code terminal session.
4. Run `/agents` and confirm the project agents from §1 are present.
5. Run `/status` and confirm the main model.
6. **Resume only low-risk work** — and only if the agents are back.

## 5. Fallback (direct-role execution) policy

Direct-role execution means doing an agent's job inline, without the named
subagent. It is a **bounded** stopgap, not a substitute for the specialist.

### Fallback IS allowed

- Lane F: frontend **type-only** changes.
- Lane A: **mechanical / devops** changes.
- Only when **no controlled high-risk zone** is touched.

### Fallback is NOT allowed

- Security
- Reliability bugs
- auth / session / tenant isolation
- outbound / connector / ledger
- migrations
- backend runtime behavior
- API contracts
- fullstack flows

When in doubt, stop and restart the session (§4) rather than proceeding with
fallback on anything that is not clearly low-risk type-only or mechanical work.
