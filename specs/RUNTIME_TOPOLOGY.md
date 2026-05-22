# THG Runtime Topology

**Status**: living doc — describes runtime composition of the system as it actually runs in production. Pairs with the spatial decomposition in [internal/store/DOMAINS.md](../internal/store/DOMAINS.md) and the migration history in [STORE_SUBPACKAGE_REFACTOR.md](STORE_SUBPACKAGE_REFACTOR.md).

**Last verified**: 2026-05-22 (post-Phase 8b — store subpackage wave complete).

## What this doc is — and is not

This doc describes the **runtime topology** of THG: which layers own which truths, which call chains carry which data, where transactions and append-only boundaries sit, where failures bubble up. It is the operational mental model — the answer a senior engineer needs to predict the blast radius of any change.

**This doc IS:**

- A layer map grouping existing packages by runtime role (§1).
- A flow catalogue enumerating production call chains with their domain handoffs (§2).
- The canonical pointer to truth ownership (§3 → DOMAINS.md §2.4).
- An explicit register of append-only invariants and current violations (§4).
- The failure-surface map: how recoverable errors propagate and where humans get notified (§5).
- A CI enforcement index — every invariant has a grep gate in `scripts/check_topology.sh` (§6).

**This doc is NOT:**

- A new package layout. No `core/`, `runtime/`, `platform/`, `foundation/` directories appear from this work — the layers below are a *facet* over the existing spatial structure, not a re-organisation.
- A doctrine or governance document. The only durable enforcement is the grep gates in §6. Prose without enforcement decays.
- A premature platformisation plan. Stage 4 (reusable workflows / multi-platform abstraction) is **deferred** until a second platform (Taobao / 1688 / TikTok per memory) is actually in production — until then, abstraction is over-engineering.

---

## 1. Layer map

Same packages as [DOMAINS.md §1](../internal/store/DOMAINS.md#1-quick-navigation), grouped by runtime role. Arrows = "reads from / depends on". Lower layers must not import upper layers (L1 invariant; enforced by `scripts/check_topology.sh` §6.1).

```
┌────────────────────────────────────────────────────────────────────┐
│ CONTROL PLANE                                                      │
│   internal/server/observability/        ExecutionRealityView (FE) │
│   internal/server/org/superadmin.go     diagnostic + reset-risk   │
│   slog event taxonomy                                              │
│   Role: read-only over every layer below. No writes.               │
└─────────────────────────────────┬──────────────────────────────────┘
                                  │ reads
┌─────────────────────────────────▼──────────────────────────────────┐
│ ENGAGEMENT LAYER                                                   │
│   internal/store/leads/         lead pipeline + engagement view   │
│   internal/store/threads/       conversation state                 │
│   Role: business-outcome projection. Owns badge state.             │
│   Truth: derived from EXECUTION layer; never authoritative.        │
└─────────────────────────────────┬──────────────────────────────────┘
                                  │ projects from
┌─────────────────────────────────▼──────────────────────────────────┐
│ EXECUTION LAYER                                                    │
│   internal/store/outbound/      queue/claim/finalize state machine │
│   internal/store/coordination/  action_ledger + execution_attempts │
│                                 + behaviour_profile (append-only   │
│                                 truth substrate)                   │
│   Role: "what the platform did and was it verified".               │
│   Truth: coordination is canonical; outbound is the state machine. │
└─────────────────────────────────┬──────────────────────────────────┘
                                  │ written by
┌─────────────────────────────────▼──────────────────────────────────┐
│ INTELLIGENCE LAYER                                                 │
│   internal/store/prompts/       prompt logs + ai_memory + routing  │
│   internal/store/knowledge/     KOS: sources/assets/embeddings     │
│   internal/ai/                  classifier + message generator     │
│   Role: decisions + context. Feeds INGESTION (classify) and        │
│   EXECUTION (message generation).                                  │
└─────────────────────────────────┬──────────────────────────────────┘
                                  │ feeds
┌─────────────────────────────────▼──────────────────────────────────┐
│ INGESTION LAYER                                                    │
│   internal/store/crawl/         intents + groups + posts           │
│   internal/store/connectors/    Chrome-extension command bus       │
│   internal/leadingest/          shared crawl→lead pipeline         │
│   Role: input collection. Source of every lead and post.           │
└─────────────────────────────────┬──────────────────────────────────┘
                                  │ identity-scoped by
┌─────────────────────────────────▼──────────────────────────────────┐
│ BROWSER RUNTIME LAYER                                              │
│   internal/store/identities/    FB accounts (encrypted creds)     │
│   internal/store/app/sessions   (AppStore wrapper — browser sess.) │
│   internal/store/app/identities (browser fingerprints)             │
│   internal/browsergateway       transport contracts                │
│   internal/session/             session lifecycle + checkpoint     │
│   Role: per-account browser state. Mediates all FB I/O.            │
└─────────────────────────────────┬──────────────────────────────────┘
                                  │ tenant-bounded by
┌─────────────────────────────────▼──────────────────────────────────┐
│ FOUNDATION LAYER                                                   │
│   internal/store/  users.go + organization.go + store.go +        │
│                    schema.go + migrator.go + backup.go             │
│   internal/store/dbutil/        cross-cutting helpers              │
│   internal/store/storetest/     test infra (TEST-ONLY)             │
│   Role: tenant root + infra. Everything imports dbutil.            │
└────────────────────────────────────────────────────────────────────┘
```

**Layer boundary rules** (each is a grep gate in §6):

- Lower layers MUST NOT import upper layers (cycle-prevention; Go compiler also catches).
- Same-layer cross-imports are restricted: e.g. coordination + outbound are both EXECUTION but coordination does not import outbound (per [[feedback_no_bidirectional_domain_knowledge]]). outbound calls coordination via the Hooks closure pattern wired at boot.
- CONTROL PLANE may read any layer; never writes.

---

## 2. Flow catalogue

Every production-visible feature traces to exactly one flow below. Adding a feature = mapping it onto one of these (or, rarely, justifying a new flow). When that mapping is hard, the feature is probably crossing a boundary the topology doesn't yet name — that's a topology bug, not a feature bug.

### FLOW 2.1 — Crawl → Lead (ingestion → engagement)

```
INGESTION ──▶ INTELLIGENCE ──▶ ENGAGEMENT (via append to EXECUTION's action_ledger? no — this flow doesn't touch action_ledger)
```

```
1. Chrome extension scrolls a FB feed → POSTs crawl chunk to
   POST /api/agent/connectors/crawl-result/:id
   (internal/server/agent/crawl.go)
2. handler calls leadingest.IngestPost (internal/leadingest/ingest.go)
   for each item — shared between extension + worker paths.
3. IngestPost runs:
   - URL repair (canonical permalink synthesis from PostFBID + GroupFBID)
   - deterministic scoring
   - AI classify (when business profile + UserPrompt present)
   - thread_role inference
4. Persists:
   - task_leads     via deps.AppStore.InsertLead     (BROWSER RUNTIME)
   - legacy leads   via deps.LegacyDB.Leads().InsertLead (ENGAGEMENT)
   - thread row     via deps.LegacyDB.Threads().SeedThreadForOrg
     (ENGAGEMENT — substrate for ConversationGate to see "first-touch"
      from message #1 instead of post-send)
5. Cursor advance on the crawl intent (if recurring run)
```

**Sync/async**: sync within the HTTP request. Failure non-fatal — single lead skip never rolls back the batch.

**Tx boundary**: each lead = independent `s.db.ExecContext` calls. No multi-row tx. Acceptable because partial ingest is tolerable; durability is per-lead.

**Crosses**: 5 packages (extension HTTP boundary → leadingest → scoring → ai → leads + threads).

---

### FLOW 2.2 — Queue → Execute → Verify

The execution-verification flow. This is the load-bearing flow of the entire platform. Every behavioural rule (cooldowns, caps, risk_score, badge derivation) ultimately reduces to one of these steps producing a particular outcome.

```
INTELLIGENCE ──▶ EXECUTION ──(extension)──▶ EXECUTION (verified) ──▶ ENGAGEMENT (projection)
```

```
A. ENQUEUE  (single tx)
   skill handler (cmd/scraper/outbound_actions.go::queueLeadOutreach)
     → db.QueueOutboundForOrg (deprecated wrapper → outbound.Store.Queue)
       └── tx.Begin()
           ├── outbound.Store.canQueueOutboundTx (dedup index check)
           ├── HOOK BehaviourCheck → coordination.CheckCapsTx(tx, accountID, msgType)
           ├── HOOK ConversationGate → conversationGateForOutbound(ctx, orgID, targetURL, profileURL, cooldown)
           │       └── store.Threads().GetThreadByProfileForOrg(orgID, profileURL)
           ├── INSERT INTO outbound_messages (execution_state='planned')
           ├── HOOK RecordActionLedger → coordination.RecordLedgerTx(tx, ..., outcome='queued')
           ├── HOOK IncrementCounter  → coordination.IncrementCounterTx(tx, accountID, msgType)
           └── COMMIT

B. CLAIM    (single tx)
   GET  /api/agent/outbox  (internal/server/agent/outbox_agent.go::agentGetOutbox)
     → outbound.Store.Claim (CAS execution_state: planned → executing)
     → returns rows with fresh execution_id + lease_expiry to the extension

C. EXECUTE  (in browser)
   extension content script: identity gates → typeText → submit → DOM verify
   → POST /api/agent/connectors/outbox/:id/sent or /failed
     with ExtensionExecutionReport (proof.js → JSON body)

D. VERIFY + FINALIZE  (TWO distinct tx)

   D1. (tx 1 — outbound state machine)
       outbound.Store.Finalize (CAS execution_state on execution_id)
       → atomic flip executing → finished
       → HOOK RecordTransition → coordination.RecordTransitionTx(ctx, tx1, ...)
         INSERT INTO execution_attempts (canonical writer)

   D2. (tx 2 — implicit; best-effort)
       coordination.MarkActionLedgerOutcomeByOutbound (UPDATE action_ledger)
         ⚠ KNOWN APPEND-ONLY VIOLATION (see §4)
       coordination.ApplyRiskSignal (UPDATE account_runtime_state)

E. PROJECT
   leads.GetLeadEngagement reads coordination's tables (action_ledger
   + execution_attempts) → derives badge state on every dashboard read.
   No push notification; pure projection.
```

**Sync/async**: A through D are synchronous within their HTTP handler. C is bounded by the extension's network latency. E is on-demand read.

**Tx boundaries**: A is one tx (the queue tx — atomic). D1 is one tx (finalize CAS + execution_attempts append). D2 is a separate best-effort tx — failure logged, not propagated, because the outbound CAS already committed and reverting would create a worse split-brain.

**Append-only invariant**:
- `execution_attempts` writes are INSERT-only at the row level — D1's INSERT is the canonical write. Internal lifecycle (`AdvanceStatus` UPDATE) is intra-row state and predates the append-only mandate.
- `action_ledger` SHOULD be append-only; D2's UPDATE breaks the invariant. Logged in §4 as carry-over debt.

**Failure modes**: see §5.

**Crosses**: 7 packages (cmd/scraper → outbound → coordination → server/agent → runtime → identities + connectors).

---

### FLOW 2.3 (sketch) — Reply Loop

```
extension detects new inbound message → threads.MarkInboundReceivedForOrg
   → leads engagement projection re-derives badge: 'lead_replied'
   → next ConversationGate query returns Allowed=true, Reason='lead_replied'
     (replied lead is allowed through cooldown)
```

**Tempo**: async, poll-based. Currently no push notification — operator sees the new state on next dashboard refresh.

To-fill: the exact code path from extension polling → handler → MarkInboundReceivedForOrg is not yet traced in detail.

---

### FLOW 2.4 (sketch) — Reconciliation

```
cron / admin trigger
   → coordination.ReconcileEngagement(orgID)
   → SELECT action_ledger LEFT JOIN execution_attempts.outcome
   → if ledger.outcome='succeeded' but attempt.outcome ∈ failure-set:
       UPDATE action_ledger SET outcome='failed', reason='reconciled:...'
       ⚠ APPEND-ONLY VIOLATION (should emit engagement_revoked)
```

**Tempo**: scheduled background. Idempotent — second run on same data is a no-op.

Known violation tracked in §4.

---

### FLOW 2.5 (sketch) — Diagnostic

```
founder hits GET /api/superadmin/accounts/:id/diagnostic
   → identities.GetAccount
   + coordination.GetAccountRuntimeState
   + coordination.ListAttemptsForOutbound
   + coordination.ListActionLedger
   → JSON merged + parsed evidence_json returned for human inspection
```

**Tempo**: pure read; no write side-effects ever.

**Pairs with**: POST /api/superadmin/accounts/:id/reset-risk (the one mutation in the diagnostic surface — admin-only, audited).

---

### To enumerate

Five more flows known to exist but not yet traced:

- **AI Generation** (skill resolves lead → prompts.GetRelevantMemories + knowledge retrieval → ai.MessageGenerator → outbound queue)
- **Crawl Intent Scheduling** (cmd/scraper crawl_scheduler → CrawlIntent table → recurring submission)
- **Browser Watchdog** (RestartController → session.CheckpointVerifier → human_required signal)
- **Skill Registration** (boot path: cmd/scraper/skills_register → skills.Registry → org_skills overrides)
- **Login Session** (server/auth/login_session → connector identity bridge → identities.SetAccountFacebookIdentity)

Add as the topology stabilises. Each new flow that doesn't map onto an existing one is a topology delta worth a doc PR.

---

## 3. Truth ownership

Authoritative table lives in [DOMAINS.md §2.4](../internal/store/DOMAINS.md#24-truth-ownership-matrix-locked-2026-05-21). That table is the SOURCE; this section is the POINTER plus completions discovered during Phase 5B–8b extractions.

### Completions noticed during the wave

| Truth | Owner | Notes |
|-------|-------|-------|
| conversation thread state | `conversation_threads` → threads | seeded at lead-ingest with `last_outbound_at = NULL` (Phase B substrate) |
| lead engagement projection | derived → leads | `leads.Store` holds a `*threads.Store` handle for the cross-domain read |
| chrome extension session | `connector_screenshots` → connectors | reclassified from identities domain in Phase 7 |
| prompt routing decision | `prompt_logs.routing_decision_json` → prompts | Watchpoint B observability lives in `internal/server/observability/prompt_routing.go` |
| schema bootstrap markers | `_schema_bootstrap_marker` → infra | written last by `migrate()`; gates fast-path skip |

### Rules implied (binding)

1. Coordination is canonical writer for action_ledger + execution_attempts. Any other writer is a violation (§6.4, §6.5 gates).
2. Engagement projection consumes coordination's tables; coordination never reads back ([[feedback_no_bidirectional_domain_knowledge]]).
3. Cross-domain SQL projections (leads → outbound EXISTS subquery, leads.lead_engagement → action_ledger JOIN) are accepted with `// tenant-ok` annotation. Cross-domain *writes* require an explicit Hook struct.
4. Reconciliation MUST emit correction events into action_ledger, NEVER UPDATE existing rows. Current code violates this — §4.

---

## 4. Append-only boundaries

The system depends on certain tables being append-only so projections can re-derive state by replay. Drift from append-only = drift from replayability = silent corruption.

### Append-only by design

| Table | Why append-only matters |
|-------|--------------------------|
| `execution_attempts` | Each row is one verifier classification; retries APPEND with `attempt = N+1`. Internal lifecycle UPDATE (`AdvanceStatus`) is intra-row state and does not violate the row-level append-only intent. |
| `prompt_logs` | Routing decisions never mutate; replay across the routing tape is how Watchpoint B audits work. |
| `engagement_events` (proposed) | Reconciliation correction events go here per [[feedback_append_only_correction_events]]. Not yet implemented. |
| `connector_commands` | Each command is one issued instruction; completion writes a new row, not an UPDATE. (Audit pending.) |

### Append-only-aspirational with current violations

| Table | Intended writer | Current violations |
|-------|-----------------|---------------------|
| `action_ledger` | coordination only, INSERT-only | 3 UPDATE statements: `MarkActionLedgerOutcome`, `MarkActionLedgerOutcomeByOutbound`, `ReconcileEngagement`. The first two are part of the queue→execute lifecycle (outcome flips from 'queued' to 'succeeded'/'failed' on finalize). The third is reconciliation — explicitly should emit `engagement_revoked` events instead. |

**Why we ship the wave with these violations carried forward**: Phase 5B mandate from the user was "preserve byte-for-byte". Fixing the append-only invariant is a semantic change (flip from row-mutation to event-emission) that deserves its own design PR — `specs/APPEND_ONLY_LEDGER_MIGRATION.md` should propose:

1. Add `engagement_revoked` row type (action_type = 'engagement_revoked', target_outbound_id = N).
2. `MarkActionLedgerOutcomeByOutbound` becomes `RecordOutcomeForOutbound` which INSERTs a `outcome_classified` event.
3. `ReconcileEngagement` emits `engagement_revoked` instead of UPDATE.
4. Engagement projection logic re-derives state from event sequence (most-recent-wins by `performed_at`).

That migration is **Stage 3.5** — earned after the topology doc + L2 CI enforcement land. Not blocking on the topology PR.

**Detection**: §6.4 grep gate tracks `UPDATE action_ledger | DELETE FROM action_ledger` count against a baseline of 3. Any new violation fails CI; lowering the count requires manually dropping the baseline.

---

## 5. Failure surface

Where does each failure mode bubble up, and who gets notified?

| Failure mode | Detected at | Recovery path | Operator-visible? |
|--------------|-------------|---------------|--------------------|
| Browser session crash | `RestartController` (watchdog) | auto-restart with 30s cooldown | Browser view shows restart event |
| FB checkpoint | `session.CheckpointVerifier` | `ErrCheckpointStillActive` → HTTP 409 `CHECKPOINT_STILL_ACTIVE` | Yes — dashboard banner + extension proof banner |
| Captcha | extension proof → `execution_attempts.outcome='captcha'` | account paused; risk_score bumped | Yes — account_health diagnostic surfaces concentration |
| Account banned / soft-restricted | accumulated `risk_score` → trust ladder demotion → cooldown | hard pause until cooldown expiry | Yes — account_health + diagnostic |
| Crawl context drift | extension verifier → `outcome='context_drift'` or `redirected_feed` | logged in `execution_attempts.evidence_json` | Yes — Stuck-state observation panel (PR-E) |
| Hallucinated success (ledger says succeeded, attempt says blocked) | nightly `ReconcileEngagement` (currently UPDATE; targets engagement_revoked emission) | ledger correction; risk re-derived | Partially — visible after reconcile run |
| Tx commit failure on outbound queue | `outbound.Store.Queue` returns error | retry-safe (no partial state) | Skill response surfaces the error string |
| Best-effort hook failure (coordination ledger/counter) | logged via `slog.WarnContext` | none — failure is intentional; outbound CAS holds | Logs only; no dashboard signal yet |
| Cross-account concurrent first-inbox slip | NOT DETECTED today (substrate exists via PR-B seed; gate-rule fix deferred) | none today | No — would surface as duplicate inbox sent |

**Gap surfaced by this map**: best-effort hook failures (coordination ledger updates that silently fail) have no operator surface. A future Control Plane addition should aggregate `event=execution.verified` slog records and surface failure-class counts. Tracked in [[project_runtime_control_plane]] EXP-1 instrumentation.

---

## 6. CI enforcement index

Every binding invariant in this doc maps to a check in `scripts/check_topology.sh`. CI failure = the topology doc no longer describes reality.

| # | Invariant | Gate | Current status |
|---|-----------|------|----------------|
| 1 | L1 dependency direction — no subpackage imports parent store | grep for `"internal/store"` in subpackage dirs | PASS |
| 2 | Coordination has no peer-domain (outbound/leads/threads) imports | grep for those imports in `coordination/` | PASS |
| 3 | Outbound has no leads/threads peer imports | grep in `outbound/` | PASS |
| 4 | action_ledger INSERTs only in coordination/ | grep `INSERT INTO action_ledger` outside coordination/ | PASS |
| 5 | execution_attempts INSERTs only in coordination/ | grep `INSERT INTO execution_attempts` outside coordination/ | PASS |
| 6 | action_ledger append-only (baselined) | count of `UPDATE action_ledger \| DELETE FROM action_ledger` | EXPECTED-FAIL (3 carry-over; baseline 3) |
| 7 | No downstream business reads of legacy `outbound_messages.status` | grep heuristic | deferred (needs schema-aware check; current heuristic too noisy) |
| 8 | Typed event taxonomy — no raw `"event"` string literals outside `internal/runtime/events/` | grep for `"event"\s*,\s*"<string>"` outside the events package | PASS |
| 9 | L2 wrapper count tracking | count of `// Deprecated:` markers in `outbound_aliases.go` | INFO (28 — track only) |

Run: `bash scripts/check_topology.sh`. Wired into CI in `.github/workflows/ci.yml` as the `topology` job — runs before tests so topology regressions surface fast.

To raise a check from EXPECTED-FAIL to PASS, the underlying violation must be fixed; lowering the baseline number is a deliberate act recorded in the script's commit history.

---

## 7. Stages of the architecture roadmap

For shared vocabulary about where we are and what comes next. Adapted from the user's framing 2026-05-22 with one explicit deferral on Stage 4.

| Stage | Goal | THG state |
|-------|------|-----------|
| 1 — Monolith | Make it work | done (pre-2026-05) |
| 2 — Extraction | Isolate ownership | **done 2026-05-22** (Phases 0–8b shipped; 11 subpackages) |
| 3 — Topology | Define runtime composition | **in progress** (this doc + check_topology.sh + L2 enforcement) |
| 4 — Platformisation | Reusable workflows / multi-platform services | **deferred** — earned by shipping a 2nd platform (Taobao / 1688 / TikTok per memory). Premature abstraction here is the next entropy bomb. |

**Stage 3 deliverables** (current work — in priority order):

1. Runtime stabilisation — close stuck-state surfaces (PR-E shipped); maintain green tests through topology PR.
2. L2 CI enforcement — `check_topology.sh` (this PR) + wire into ci.yml.
3. Runtime topology doc — this file.
4. Replay / Control Plane foundations:
   - 4a. **Typed slog event taxonomy** — `internal/runtime/events/` package (DONE 2026-05-22). 13 event constants, 2 emit helpers (Info + Warn), retrofit of 3 existing typed-event sites, new emission at the "best-effort hook failure" gap from §5. CI gate §6.8 enforces no raw event-name literals outside the package.
   - 4b. **`runtime_events` table + dual-write** (DONE 2026-05-22). Persistence via `coordination.RecordRuntimeEvent` + `events.SetSink` boot-time registration. Schema lives in `internal/store/coordination/runtime_events.go`. Function-typed sink — no interface (per L4).
   - 4c. **`GET /api/observability/runtime-feed`** (DONE 2026-05-22). Paginated read backing a live event tail panel. Supports `?hours=1`, `?limit=100`, `?level=warn`, `?event=outbound.queued` filters. Org-scoped + system-wide rows visible.
5. Semantic cleanup — append-only migration on action_ledger (§4); cross-domain SQL annotations.

Stage 4 will be earned, not designed in advance.

---

## 8. How to use this doc

- **Adding a feature**: trace it onto one of §2's flows. If it doesn't fit, the topology is missing a flow — open a doc PR first.
- **Changing a write path**: check §3 (ownership) and §4 (append-only). If the change crosses a boundary, ensure the relevant §6 gate still passes.
- **Onboarding a new engineer**: read §1 (5 min) + §2 flow 2.2 (10 min) to understand the load-bearing path. Everything else is reference.
- **Reviewing a PR that touches multiple domains**: run `scripts/check_topology.sh` locally; surface §6 results in the PR description.

Pointer hubs: [DOMAINS.md](../internal/store/DOMAINS.md), [STORE_SUBPACKAGE_REFACTOR.md](STORE_SUBPACKAGE_REFACTOR.md), [project_runtime_control_plane](../) memory.
