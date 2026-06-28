---
doc_type: architecture
status: active
owner: platform
last_reviewed: 2026-06-28
related_pr_or_issue: chore/docs2-architecture-backlinks-frontmatter
---

# Database Table Ownership

> Part of the [architecture docs index](INDEX.md).

**Status:** OFFICIAL STANDARD. **Companion of** `ARCHITECTURE_STANDARD.md`.
Builds on the existing `internal/store/DOMAINS.md` truth-ownership matrix (§2.4)
and `specs/RUNTIME_TOPOLOGY.md`. This doc is the **table-level** contract: every
table has exactly ONE owner module that may write it; everyone else reads via the
owner's API or a documented `// tenant-ok` cross-domain projection.

**Rules**
1. **One writer module per table.** Other modules calling a write = a violation.
2. **Reads via projection.** Downstream reads through the owner accessor
   (`s.Outbound()…`, `s.Leads()…`) or a `// tenant-ok: cross-domain projection`
   SQL join, never by reaching into another domain's row semantics.
3. **`org_id` on every tenant table.** Enforced by `scripts/check_tenant_isolation.sh`.
4. **Append-only tables** (`execution_attempts`, `action_ledger`) are written by
   `coordination` ONLY, insert-only; corrections are new rows, never UPDATE/DELETE.

Owner names are the *logical modules* from `MODULE_BOUNDARIES.md`; the parenthetical
is the current store subpackage (`internal/store/<x>`).

---

## Identity & tenancy

| Table | Owner | Allowed readers | Allowed writers | Forbidden writers | Migration notes |
|---|---|---|---|---|---|
| `users` | platform (store top-level) | auth, all (via accessor) | platform/users | services, outbound, ai | tenant root; foundational |
| `organizations` | platform (workspace/org) | all (via accessor) | platform/org | services, drivers | workspace identity |
| `org_invites` | platform (org) | org, notifications (event) | org | services, ai | invite lifecycle → emits events |
| `refresh_tokens` | auth (store top-level) | auth | auth | everyone else | never logged |
| `agent_tokens` | connectors | connectors, drivers/connector | connectors | services, ai | extension auth tokens; secret |
| `staff_contact_profiles` | brand (contactprofile) | services (grounding), server | brand | ai, outbound | persona/contact grounding |
| `staff_kpi`, `kpi_config` | app | server (dashboard) | app | services | reporting |

## Accounts, browser, connectors

| Table | Owner | Allowed readers | Allowed writers | Forbidden writers | Migration notes |
|---|---|---|---|---|---|
| `accounts` | identities | services, outbound (readiness), connectors | identities | ai, drivers/copilot | Facebook accounts; cookies encrypted in-domain |
| `account_behaviour_profiles` | coordination (behaviour) | services (caps) | coordination | services direct | static behaviour profile |
| `account_runtime_state` | coordination (behaviour) | services (caps) | coordination | services direct | runtime cooldown/risk state |
| `connector_commands` | connectors | drivers/connector | connectors | services direct | pull-based command queue |
| `connector_pairing_codes` | connectors | drivers/connector | connectors | everyone else | pairing secrets |
| `connector_screenshots` | connectors | server (readiness) | connectors | services | evidence/heartbeat |
| `extension_policies` | connectors | server | connectors | services | version gating |
| `selector_cache` | connectors | services (selectors) | connectors | ai | DOM selector cache |

## Leads, posts, crawl

| Table | Owner | Allowed readers | Allowed writers | Forbidden writers | Migration notes |
|---|---|---|---|---|---|
| `leads` | leads | services, outbound (target resolve), server | leads (via `leadingest`/InsertLead) | ai, drivers, outbound | source_type post/comment; `GetLeadByPostRef` is the canonical lookup |
| `classification_log` | leads | server | leads | services direct | classifier audit |
| `niches` | leads | services, ai (context) | leads | — | taxonomy |
| `posts`, `groups`, `group_quality` | crawl | services, leads (projection) | crawl | ai, outbound | crawl artifacts |
| `comments`, `inbox_messages` | crawl | services | crawl | ai | scraped engagement |
| `scan_logs` | crawl | server | crawl | — | crawl forensics |
| `jobs` | crawl/jobs | scheduler, drivers/connector | jobs | services direct | crawl job queue (`internal/jobs`) |
| `org_crawl_intents` | crawl/jobs | scheduler | crawl/jobs | — | recurring crawl plans |

## Outbound coordination spine

| Table | Owner | Allowed readers | Allowed writers | Forbidden writers | Migration notes |
|---|---|---|---|---|---|
| `outbound_messages` | outbound | services (status projection), server, notifications (event) | outbound | services direct, ai, drivers | legacy `status` column DEPRECATED (use `execution_state`/`verification_outcome`); column drop is V2 PR2, gated |
| `execution_attempts` | coordination | outbound, server | **coordination ONLY** | everyone else | **append-only**; raw writes elsewhere = topology violation |
| `action_ledger` | coordination | services (attribution projection), server | **coordination ONLY** | everyone else | **append-only**; corrections emit `engagement_revoked` rows |
| `action_policies` | outbound | outbound | outbound | services hardcode | domain-agnostic policy (replaces hardcoded gates) |
| `comment_reverify`, `comment_verification_audit` | coordination | server, drivers/connector | coordination | services direct | async reverify pipeline |
| `runtime_events` | events/coordination | server (SSE), audit | coordination | services direct | runtime audit stream (NOT yet the durable outbox — see TRANSACTIONAL_OUTBOX.md) |

## Knowledge, brand, content

| Table | Owner | Allowed readers | Allowed writers | Forbidden writers | Migration notes |
|---|---|---|---|---|---|
| `knowledge_assets`, `knowledge_sources`, `knowledge_events`, `knowledge_feedback` | knowledge | ai (grounding), services | knowledge | outbound, drivers | grounding substrate |
| `data_sources`, `private_files`, `company_images` | knowledge | services, ai | knowledge | outbound | uploaded real assets only (no AI images) |
| `price_items` | app/knowledge | services (grounding) | app | ai | catalog pricing — grounds outbound claims |
| `career_jobs` | app | server | app | — | HR/recruitment blueprint data |

## Threads, conversations

| Table | Owner | Allowed readers | Allowed writers | Forbidden writers | Migration notes |
|---|---|---|---|---|---|
| `conversation_threads`, `conversation_messages` | threads | outbound (conversation gate, via adapter), server | threads | services direct | `conversationGateForOutbound` composes threads+outbound at top-level store |

## Telegram & notifications

| Table | Owner | Allowed readers | Allowed writers | Forbidden writers | Migration notes |
|---|---|---|---|---|---|
| `telegram_bindings`, `telegram_bind_codes`, `telegram_destinations`, `telegram_settings`, `telegram_bot_credentials`, `telegram_alert_prefs`, `telegram_audit` | notifications/telegram | drivers/telegram | telegram | services, ai | bot tokens encrypted; never logged |
| `notifications` | notifications | server (bell) | notifications | services direct | in-app bell; fed by events |

## AI / prompts / audit / KV

| Table | Owner | Allowed readers | Allowed writers | Forbidden writers | Migration notes |
|---|---|---|---|---|---|
| `ai_memory` | prompts/ai | drivers/copilot | prompts | services | few-shot memory |
| `prompt_logs` | prompts | server (observability) | prompts | — | routing/decision audit |
| `skill_executions`, `org_skills` | platform (skills) | server | skills | services direct | open-prompt skill catalog + audit |
| `audit_logs` | platform (audit) | server, admin | any module via audit API | direct INSERT bypass | cross-cutting; write via audit port |
| `user_context` | leads (KV) | any (config reads) | leads (`SetContext`/`DeleteContext`) | — | **key-value config table.** ⚠️ Used by the P1 prototype as a continuation store (`comment_cont:<org>:<post>`). That is acceptable for a prototype but is NOT the standard storage for workflow state — durable workflow state belongs in the transactional outbox / a process-manager table (see TRANSACTIONAL_OUTBOX.md). |
| `user_execution_context` | identities/coordination | services | owner | — | per-user execution scope |

---

## Open ownership questions (resolve before the relevant phase)

- **`runtime_events` vs a new `outbox` table.** `runtime_events` is an audit stream
  today. The transactional outbox (roadmap Phase E) should be a *separate*
  insert-in-same-tx table with relay state (`pending/published/failed`), not
  overloaded onto `runtime_events`. Decide at Phase E design.
- **`user_context` continuation rows.** The P1 prototype's `comment_cont:*` keys
  should migrate to a process-manager table when the import-continuation feature is
  re-implemented on the outbox (roadmap Phase H). Until then they are tolerated,
  org-scoped, and self-expiring (24h TTL).
- **`outbound_messages` legacy columns** (`status`, `claimed_by`, `claimed_at`,
  `execution_id`, `lease_expiry`, `sent_at`): drop is V2 Outbound PR2, gated on
  production verification (see `specs/V2_OUTBOUND_REFACTOR_DESIGN.md`).
