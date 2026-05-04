# Open-Prompt Agent — Phase 6 Design Proposal

> Status: **DRAFT — awaiting user approval before implementation.**
> Drafted: 2026-05-04. Author: Claude (THG AutoFlow agent).

This document specifies the runtime architecture that lets a tenant
type **one open prompt** (Vietnamese / English / mixed) and have the
system orchestrate every Facebook capability the org has enabled —
post, comment, scrape, fanpage care, profile care, inbox automation —
without hardcoding any vertical (HR, POD, sales, support, …).

It does **not** open a path for arbitrary LLM-generated code. Skills
are validated blueprints over shared primitives.

---

## 1. North star, restated

User flow:

```
[Telegram message] / [Dashboard chat box]
         │
         ▼
┌─────────────────────────────────────────────┐
│  POST /api/agent/prompt {prompt, account?}  │
└──────────────────┬──────────────────────────┘
                   │
                   ▼
       Agent.ProcessPromptForOrg(...)
                   │
                   ▼
            Skill Resolver
            (LLM picks skill_id + args
             from prompt + business context)
                   │
                   ▼
            Skill Registry — built-in
            ┌──────────────────────────┐
            │ comment_on_post          │
            │ inbox_lead               │
            │ post_to_group            │
            │ scrape_group             │
            │ scrape_post_comments     │
            │ search_groups            │
            │ bulk_comment_leads       │
            │ bulk_inbox_leads         │
            │ scan_fanpage_inbox  ★new │
            │ post_to_profile     ★new │
            │ care_fanpage        ★new │
            │ describe_business        │
            └──────────────────────────┘
                   │
                   ▼
            Primitives (Phase 1-3 helpers)
            • QueueOutboundForOrg
            • requireAccountForOrg
            • ClaimNextLocalJob
            • applyConnectorIdentity
            • ConnectorOwnsAccountStream
            • workspace.AcquireProfileLock
                   │
                   ▼
            Outbox / Crawl queue / Agent runtime
```

The architectural shift is **skills, not flat tools**. Today the
agent has 12 free-floating tools plus a switch in
`makeAgentActionHandler`. Phase 6 wraps each one as a typed skill,
adds three new skills (fanpage / profile / inbox-scan), gives orgs
the ability to enable/disable skills per blueprint, and routes every
execution through one auditable entry point.

---

## 2. Concrete data model

### 2.1 `Skill` (Go-side, registered at process boot)

```go
type Skill struct {
    ID          string                 // stable string, snake_case
    Title       string                 // human-readable label (Vi)
    Description string                 // LLM-facing description
    Category    SkillCategory          // post / comment / inbox / scrape / care / admin
    Outbound    bool                   // true → must use QueueOutboundForOrg
    NeedsAccount bool                  // true → resolver MUST pass account_id
    Parameters  []SkillParam           // typed; rendered as JSON Schema for LLM
    DefaultBlueprint bool              // true = enabled out-of-the-box for new orgs
    Run         SkillRun               // executor — receives validated params + ctx
}

type SkillCategory string
const (
    CategoryPost    SkillCategory = "post"
    CategoryComment SkillCategory = "comment"
    CategoryInbox   SkillCategory = "inbox"
    CategoryScrape  SkillCategory = "scrape"
    CategoryCare    SkillCategory = "care"     // fanpage / profile maintenance
    CategoryAdmin   SkillCategory = "admin"    // describe_business, get_stats
)

type SkillParam struct {
    Name        string                 // arg key
    Type        string                 // string | int | bool | url | enum
    Description string
    Required    bool
    Enum        []string               // when Type == "enum"
    MaxLen      int                    // applied via sanitizeForPrompt
}

type SkillRun func(ctx context.Context, env SkillEnv, args map[string]any) (SkillResult, error)

type SkillEnv struct {
    DB        *store.Store
    AppStore  *store.AppStore
    JobStore  *jobs.Store
    MsgGen    *ai.MessageGenerator
    Notify    func(string)
    OrgID     int64
    UserID    int64       // operator who initiated the prompt; 0 for Telegram
    AccountID int64       // resolved Facebook account, 0 if NeedsAccount==false
}

type SkillResult struct {
    Summary    string                 // one-line, returned to chat
    Entities   map[string]any         // structured payload for UI / audit
    Approved   int                    // outbound rows queued as approved
    Drafted    int                    // outbound rows queued as draft
    Skipped    int
    SkipReasons map[string]int
}
```

### 2.2 Org enablement (`org_skills` table)

```sql
CREATE TABLE IF NOT EXISTS org_skills (
    org_id     INTEGER NOT NULL,
    skill_id   TEXT    NOT NULL,
    enabled    INTEGER NOT NULL DEFAULT 1,
    config     TEXT    NOT NULL DEFAULT '{}',  -- per-skill overrides (templates, caps)
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (org_id, skill_id)
);
```

When an org has zero rows, it gets the default blueprint —
all skills with `DefaultBlueprint=true`. Admin can disable a skill
(e.g. an HR-focused org turns off `bulk_inbox_leads`) without
deleting it.

`config` is a small JSON object the skill can read from the env
(e.g. caps on `bulk_*`, default templates). It is **admin-controlled**
— same protection as `outbound_mode`, `set_context` rejects writes.

### 2.3 Audit log (`skill_executions`)

```sql
CREATE TABLE IF NOT EXISTS skill_executions (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    org_id       INTEGER NOT NULL,
    user_id      INTEGER NOT NULL DEFAULT 0,
    source       TEXT    NOT NULL,           -- dashboard | telegram | api
    skill_id     TEXT    NOT NULL,
    args_json    TEXT    NOT NULL DEFAULT '{}',
    summary      TEXT    NOT NULL DEFAULT '',
    success      INTEGER NOT NULL DEFAULT 0,
    error        TEXT    NOT NULL DEFAULT '',
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_skill_executions_org
    ON skill_executions(org_id, created_at DESC);
```

Every prompt run inserts one row. Operators can review which skill
fired with which args, why a draft was rejected, etc. This is the
only audit trail across the open-prompt path.

---

## 3. Resolver — how a prompt becomes a skill

The current code mixes two paths:

1. **Deterministic action** (`deterministicFacebookAction`) — regex
   on the prompt for fast-path commands like "scrape …".
2. **LLM tool calling** with the 12-tool list.

Phase 6 keeps both layers but unifies the output:

- The deterministic path returns a `(skillID, args)` tuple, not a
  free-form action name.
- The tool list is generated dynamically from the org's enabled
  skills (so an HR org doesn't see POD-specific tooling appear
  in its function-call manifest).
- The LLM is prompted with the skill catalog + business profile +
  account list (already in `agent_prompt.go`) and is told that the
  only valid actions are `skill:<id>`.

Pseudocode:

```go
func (a *Agent) ProcessPromptForOrgWithAccount(...) (string, error) {
    skills := a.registry.EnabledFor(orgID)            // filtered list
    if id, args, ok := DeterministicSkill(prompt); ok && skills.Has(id) {
        return a.runSkill(ctx, id, args, env)
    }
    sysPrompt := buildDynamicSystemPromptWithSkills(userCtx, accounts, skills)
    tools := skills.AsOpenAITools()
    chosen := callOpenAI(sysPrompt, prompt, tools)     // returns skillID + args
    return a.runSkill(ctx, chosen.ID, chosen.Args, env)
}

func (a *Agent) runSkill(ctx, id, args, env) (string, error) {
    skill, ok := a.registry.Get(id)
    if !ok || !a.registry.EnabledFor(env.OrgID).Has(id) {
        return "", fmt.Errorf("skill %q not enabled for this org", id)
    }
    if err := skill.Validate(args); err != nil { return "", err }
    if skill.NeedsAccount && env.AccountID == 0 {
        env.AccountID = pickReadyFacebookAccountID(...)
    }
    res, err := skill.Run(ctx, env, args)
    a.auditExecution(env, id, args, res, err)         // writes skill_executions
    return res.Summary, err
}
```

Existing helpers (`pickReadyFacebookAccountID`,
`facebookBrowserPreflight`, `businessCalibrationPreflight`) keep
working — they just sit inside the skill's `Run` instead of in the
agent loop.

---

## 4. The new skills

These three skills are explicitly user-asked-for and are not yet
covered by the existing tool list:

### 4.1 `scan_fanpage_inbox`
- Category: `inbox`
- Args: `page_url` (string, required), `since_minutes` (int, default 60)
- Run: opens the org's Fanpage Messenger, pulls unread threads,
  classifies each via `ai.classifier`, queues a draft reply via
  `QueueOutboundForOrg`. Threads where the lead replied (per
  `threads.last_inbound_at`) get top priority.
- Approval: required (default draft) unless `outbound_mode == auto`.

### 4.2 `care_fanpage`
- Category: `care`
- Args: `page_url`, `actions` (enum array of `react_recent`,
  `pin_post`, `repost_announcement`)
- Run: a maintenance routine over the fanpage admin surface. Uses
  the workspace browser. No outbound to leads — only fanpage-internal
  housekeeping.

### 4.3 `post_to_profile`
- Category: `post`
- Args: `content` (string), `account_id` (int)
- Run: posts to the operator's own profile timeline (not a group).
  Uses `QueueOutboundForOrg` with `type='profile_post'`.
- Adds a new outbound `Type` enum value — schema migration: extend
  the dedup key to include `profile_post`.

The remaining skills (`comment_on_post`, `bulk_comment_leads`, …)
are direct wraps of the existing handler functions in
`cmd/scraper/main.go`. No behavioural change, only the
typed/structured wrapper.

---

## 5. Approval model — unchanged but enforced uniformly

The store-layer rule established in Phase 2 stays the only gate:

- Outbound skills MUST call `Store.QueueOutboundForOrg` with
  `requestedAuto = (skill saw an explicit auto request) AND
  Store.IsAutoOutboundEnabledForOrg(orgID)`.
- Care skills (fanpage maintenance) are NOT outbound — no outbox row.
  They still pass through `requireAccountForOrg` and audit log.
- Scrape skills don't queue outbound but DO get logged for
  per-org cost tracking (each crawl burns OpenAI tokens).

The agent prompt no longer needs to tell the LLM about AUTO mode
because the store enforces it. Section in `agent_prompt.go` now
reads "outbound default = draft; admin must enable auto via
Settings".

---

## 6. API surface

```
GET  /api/skills                   list skills enabled for caller's org
GET  /api/skills/all               list every registered skill (admin)
PUT  /api/skills/:id/enable        admin only; flips org_skills row
PUT  /api/skills/:id/disable       admin only
GET  /api/skills/executions?limit  recent skill_executions rows
```

The chat / prompt endpoint (`POST /api/agent/prompt`) keeps its
existing shape. Internally it now goes through the resolver.

Frontend changes are minimal — a Settings page tab that lists the
catalog and lets admins toggle skills. The chat box itself doesn't
change.

---

## 7. Implementation phases

| # | Step | Files touched | LOC est. |
|---|------|---------------|----------|
| 6.1 | Define `Skill` types + registry in `internal/skills/` | new | +250 |
| 6.2 | Wrap existing 12 handlers as built-in skills | new in `internal/skills/builtin.go`; thin updates to `cmd/scraper/main.go` to remove the switch | +400 / −150 |
| 6.3 | Add 3 new skills (`scan_fanpage_inbox`, `care_fanpage`, `post_to_profile`) — at first as **approval-required scaffolds** that delegate to existing primitives where possible; full Messenger automation lands in 6.3b | new | +250 |
| 6.4 | `org_skills` + `skill_executions` migrations + helpers | `internal/store/skills.go` | +200 |
| 6.5 | Resolver — replace deterministic action loop, dynamic tool list | `internal/ai/agent.go`, `internal/ai/agent_tools.go`, `internal/ai/agent_prompt.go` | ±300 |
| 6.6 | API endpoints + handlers | `internal/server/skills_handlers.go`, route wiring in `api.go` | +180 |
| 6.7 | Tests — 1 per skill (smoke), 1 for resolver (golden prompt → skill), 1 for org enablement | `internal/skills/*_test.go`, `internal/store/skills_test.go` | +400 |
| 6.8 | Docs — update `specs/PRODUCTION_FLOW.md` §13 + add §15 "Skills", `AGENTS.md` | doc | +150 |

Phase 6.3b (Messenger automation, fanpage care actions on real
Chrome) is intentionally separate — it depends on the Phase 4 CDP
whitelist work because the skill execution path will need
production-grade CDP access.

---

## 8. What this design deliberately does NOT do

- **No arbitrary code execution.** Skills are compiled into the Go
  binary. Admin "blueprints" only toggle the catalog and supply
  config JSON — they don't add new code. Custom blueprints are a
  later phase that requires a sandbox + spec validator.
- **No new LLM provider.** Existing OpenAI client stays.
- **No removal of `agentloop/`** sandbox — it remains the experimental
  multi-step planner, separate from the synchronous skill resolver.
- **No Python rewrite.** `services/agent-brain/brain.py` stays
  optional; the resolver does not depend on it.
- **No frontend rewrite.** Only an additive Settings tab.

---

## 9. Risks + mitigations

| Risk | Mitigation |
|---|---|
| LLM picks the wrong skill on ambiguous prompts | Deterministic regex fast-path covers the unambiguous cases; LLM falls back. Add `confidence` threshold and surface "Bạn muốn X hay Y?" clarifier when low. |
| Adding `org_skills` blocks first-run UX | If org has zero rows, treat as "default blueprint" and serve the curated default set without requiring admin setup. |
| New skills (fanpage care) need real Chrome work that depends on Phase 4 | Ship 6.3 as "scaffold + approval-required draft" first; flip to live execution once Phase 4 lands. |
| Audit log grows unbounded | Add periodic prune (older than 90 days) — same pattern as `RecoverStaleLocalJobs`. |
| Custom config JSON becomes a prompt-injection vector | Treat config the same as user_context: rejected from `set_context`, only writable via admin API. |

---

## 10. Open questions for user

1. Should `post_to_profile` ship in 6.3 or is `post_to_group` enough
   for the first cut?
2. For `care_fanpage`, which actions matter most operationally —
   pin announcements, react to recent posts, scheduled posts?
3. Default blueprint composition — should new orgs see all 12+3
   skills, or a curated "starter" subset (scrape + draft-only
   outbound) until the admin opts into automation?
4. Per-skill rate limits — yes/no for v1? Could live in `org_skills.config`.
5. Telegram entry — keep it as a thin pass-through to
   `Agent.ProcessPromptForOrgWithAccount` (no change), or expose
   skill-specific shortcuts?

---

## 11. Sequencing relative to Phase 4

- **Phase 6.1 → 6.7** are independent of Phase 4 — they only need the
  Phase 1-3 helpers plus the existing `agent_handlers` plumbing.
- **Phase 6.3b** (live Messenger / fanpage care via CDP) needs Phase 4
  to land first because it expands the CDP surface area exposed to
  the runtime.

So the recommended order:

```
6.0 (this doc, approve)
   ↓
6.1 → 6.7  (skill registry, existing capabilities, +scaffold for new)
   ↓
4.x        (CDP whitelist, JWT cookie, VNC handshake)
   ↓
6.3b       (live execution for fanpage / profile / Messenger skills)
```

This matches the user's stated ordering "phase 6 then phase 4" while
acknowledging that the skill registry can be built today and the
fully-live new skills land after the CDP hardening.

---

**Action requested:** read sections 1–3 + 9–11 carefully and respond
with either "approve, proceed with 6.1" or specific changes. The
implementation lands in one bundled commit set per the user's
preferred refactor style (memory: feedback_refactor_style.md).
