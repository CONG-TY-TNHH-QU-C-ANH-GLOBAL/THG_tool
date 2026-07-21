---
doc_type: engineering
status: active
owner: platform
last_reviewed: 2026-07-13
related_pr_or_issue: docs/spec-ia-completion-mega-sprint
---

# Canonical Helper Layer

Ported verbatim from the former AGENTS.md "Helper Layer (Phase 1, 2026-05-03)"
section during the spec IA completion sprint. Use these instead of writing org
checks / session writes from scratch.

- `store.GetAccountForOrg(id, orgID)` — data-layer org boundary. The
  unscoped `GetAccount(id)` is reserved for worker code that already
  proved org ownership; tenant-facing handlers must use the org variant.
- `(s *Server) requireAccountForOrg(c, accID, orgID)` — fiber HTTP guard.
  Writes 404 on miss; caller returns the error.
- `(s *Server) requireAccountForOrgWS(orgID, role, accID)` — WebSocket
  guard, honours `IsPlatformUser` for founder/superadmin.
- `(s *Server) rejectIfFacebookProfileMismatch(c, ctx, acc, fbUID, orgID)`
  — call before mutating any FB identity. Writes 409 + records
  `local_error` session row on mismatch.
- `(s *Server) applyConnectorIdentity(ctx, snap)` — single pipeline that
  upserts browser_sessions, sets the FB identity, and flips the account
  active flag. Heartbeat / chrome-status / screenshot all funnel here.
- `store.LocalSessionStatus` enum + `store.LocalSessionStatusFromStream`
  / `store.LocalFacebookNotReady` — typed session lifecycle states.
- `store.AppStore.RecordLocalSession(ctx, accID, orgID, status, errMsg)`
  — upsert browser_sessions row with the typed enum.
- `store.ConnectorOwnsAccountStream(orgID, agentID, accID)` — call before
  trusting any work an agent attributes to an account.
- `clampPresenceFields(*store.AgentPresence)` — clamp every connector
  string before it lands in the DB; size limits in `input_limits.go`.
- `cfg.MustValidateProductionSecrets()` — boots refuse to start if
  `APP_ENV=production` and JWT_SECRET / ENCRYPTION_KEY are missing.
- `ai.sanitizeForPrompt(value, maxRunes)` — strip control chars + clamp;
  use it whenever you mix user-controlled text into an LLM prompt.
  Wrap user data in `BEGIN USER_DATA … END USER_DATA` markers.
- `store.QueueOutboundForOrg(msg, requestedAuto, cooldown)` — canonical
  write path for AI / agent / Telegram outbound. Atomic guard +
  store-layer approval policy + UNIQUE index race protection. Use
  this, not `InsertOutboundMessage`, from any LLM-driven path.
- `store.IsAutoOutboundEnabledForOrg(orgID)` — single source of truth
  for whether the org has opted into auto-execute. Backed by
  `org:{id}:outbound_mode` user_context key, admin-only.
- `workspace.AcquireProfileLock(profileDir)` — cross-process exclusive
  claim on a Chrome `--user-data-dir`. Held for the lifetime of the
  container; released on Stop / StopAll / failed Start. Don't bypass.
- `RestartController` per-account in-flight + 30 s cooldown debounce —
  multiple OnUnhealthy calls for the same account no-op while a
  restart is in progress.
- `session.CheckpointVerifier` — server.go wires
  `workspaceCheckpointVerifier` so `ResolveCheckpoint` cannot transition
  back to ready while Chrome is still on a verification page. Returns
  `*ErrCheckpointStillActive` → handlers map to HTTP 409
  (`CHECKPOINT_STILL_ACTIVE`).
- `skills.Registry` — open-prompt agent catalog. Built-ins registered
  in `cmd/scraper/skills_register.go` from main.go's existing action
  handler closure. The agent uses `skills.OpenAITools(reg.EnabledFor)`
  for the LLM tool list and `reg.Execute` for the typed validation +
  per-org enablement check + audit logging in `skill_executions`.
- `store.SetOrgSkillEnabled / GetOrgSkillConfig / SetOrgSkillConfig` —
  admin-only writes for the per-org skill blueprint and per-skill
  config JSON. The dashboard chat and Telegram bot both go through
  `Agent.ProcessPromptForOrgWithAccount`, so adding a skill anywhere
  exposes it to both surfaces simultaneously.
- New skills `scan_fanpage_inbox`, `care_fanpage`, `post_to_profile`
  ship as scaffolds today; live Chrome-driving execution lands after
  Phase 4 (CDP whitelist).

Adding a new endpoint that touches accounts? Use the helpers. If you
find yourself writing `if acc.OrgID != orgID` by hand, you're working
against the boundary — switch to `requireAccountForOrg` instead.
