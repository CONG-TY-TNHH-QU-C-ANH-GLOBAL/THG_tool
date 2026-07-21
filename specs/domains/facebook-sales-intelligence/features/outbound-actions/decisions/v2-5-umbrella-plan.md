# Verified-State-Centric Architecture: Outbound Invariant → Engagement Ledger → Templates → Telegram → Copilot

**Phiên bản**: v2.5 — bổ sung Account Reputation + append-only correction events + Reconciliation Replay UI + Knowledge Governance link
**Trạng thái**: APPROVED — bắt đầu implement PR-1.
**Ngày**: 2026-05-20
**Liên kết**: [[project_autonomous_operational_system]] · [[feedback_shared_battlefield_not_crm]] · [[project_distributed_coordination]] · [[project_execution_verification]] · [[feedback-verified-state-centric]]

---

## 0. Triết lý kiến trúc (binding cho mọi PR)

> **System trưởng thành = chuyển từ "execution-centric" sang "verified-state-centric".**

Hệ thống hiện tại đang đứng giữa 2 thế giới:
- **Execution layer**: outbound_messages, browser automation, extension proof.
- **Business truth layer**: lead engagement, AI planner, scoring, cooldown, retry suppression.

### 7 nguyên tắc bất biến (binding cho mọi PR)

1. **Supreme Invariant**: Engagement ledger là **single source of truth**. CHỈ verifier (runtime/verifier.go `FinalizeOutboundAttempt`) được phép emit `engagement_verified` events. Planner, admin tool, reconciliation service đều KHÔNG được fabricate hay forge verified success. Hard-coded ở mặt API.

2. **Immutable State Over Mutable Columns**: KHÔNG dùng `lead.touched: boolean`, `lead.engaged_count: int`, hay bất kỳ mutable counter nào trên entity. Lưu append-only events trong `engagement_events`, derive state qua projection / materialized view. Boolean / counter cho phép data corruption silently tích lũy.

3. **Append-Only + Correction Events**: KHÔNG dùng `UPDATE` / `DELETE` để fix historical state. Khi reconciliation phát hiện inconsistency → emit `engagement_revoked` correction event (event_type='engagement_revoked', references original engagement_id). Projection compute từ full log. Lịch sử không bao giờ rewrite.

4. **Strict Dependency Isolation**: Planner và external service KHÔNG được query `outbound_messages` để infer success. Phải đọc `engagement_events` exclusively. Grep audit gate trong CI block PR nào violate.

5. **Safe Reconciliation**: Default DRY-RUN mode. Reconciliation phải log `reconciliation_batches` + `reconciliation_audit_logs`. Operator review diff visually trước khi force apply. Không bao giờ là "black box mutation engine".

6. **Deterministic Templates over AI Entropy**: AI KHÔNG được rewrite template content tại runtime. Template = deterministic variable substitution. Pull "Strict Facts" (price, URL, product spec) từ `knowledge_assets` (Compliance Center), không invent.

7. **Broadcast Safeguards**: Broadcast phải có pacing rules chống heat concentration (cùng acc nhiều target cluster), temporal patterns (regular interval = bot detection signal), và **Account Reputation gate**. Không có Reputation System = burn acc pool.

---

## 1. PR order — REVISED v2.5

| Order | PR | Pháp lý / Lý do |
|-------|----|-----------------|
| **PR-1** | **Outbound taxonomy split** (`execution_state` ⊥ `verification_outcome`) + cleanup legacy draft/approve | Domain invariant nền. Phải có 2-trục state trước khi build ledger trên đó. |
| **PR-2** | **Engagement ledger (append-only + correction events)** + derived touched state + Reconciliation Replay UI + planner isolation | Fix false touched + business truth layer. **User yêu cầu NGAY tiếp theo.** |
| **PR-2.5** | **Account Reputation System** projection từ engagement_events + DOM outcomes (shadow_rejected, rate_limited) → trust_level + cooldown adjust | Missing link. Broadcast planner sẽ burn acc pool nếu không có. |
| **PR-3** | **Template schema + render engine** + behavioral metadata (risk/persona/cooldown_profile) + **Knowledge governance link** (pull strict facts from `knowledge_assets`) | Core capability. Templates = deterministic substitution + Compliance Center binding. |
| **PR-4** | **Template UI + broadcast campaigns** với aggregation (campaigns + targets) + **Reputation gate** + pacing rules | Visible feature trên top of safe foundation + acc protection. |
| **PR-5** | **Telegram multi-tenant** với BotRuntime lifecycle + **per-org failure isolation** (one org's broken bot không crash supervisor cho org khác) | Infra security + multi-tenant isolation. |
| **PR-6** | **Telegram ↔ Copilot bridge** với session_id explicit + channel bindings | Integration sau khi multi-tenant đứng vững. |
| **PR-7** | **Copilot image attach** với AI vision wiring | Enhancement cuối cùng. |

**Lý do thứ tự**: 
- "Build thêm feature trên top of corrupted state" là cách startup automation fail.
- Telegram/Copilot/Template đều **consume** engagement state → nếu state nhiễu thì mọi feature bị nhiễu theo.
- Fix invariant → fix data integrity → build Reputation projection → build feature mới.
- Account Reputation phải có TRƯỚC template+broadcast — nếu không, 1 broadcast aggressive sẽ burn pool.

---

## 2. PR-1: Outbound Taxonomy Split + Cleanup Legacy

### 2.1 Vấn đề hiện tại

Status hiện tại trộn 2 chiều vào 1 column:
```
planned, executing, verified_success, verified_failure,
context_drift, blocked, rate_limited, expired
```

Đây là **mixed dimension**:
- `planned/executing/expired` = transport lifecycle
- `verified_success/context_drift/blocked` = verification result

Khi query analytics: "bao nhiêu attempt bị context_drift trong tuần?" → phải filter cả `status IN (...)` + cross-check `execution_attempts` table → fragile.

### 2.2 Schema thay đổi

```sql
-- Migration: bump schemaBootstrapVersion = 4
ALTER TABLE outbound_messages ADD COLUMN execution_state TEXT NOT NULL DEFAULT 'planned';
ALTER TABLE outbound_messages ADD COLUMN verification_outcome TEXT;  -- nullable: chưa có outcome khi planned/executing

-- Backfill từ status hiện tại
UPDATE outbound_messages SET execution_state = 'planned'  WHERE status = 'approved';
UPDATE outbound_messages SET execution_state = 'executing' WHERE status = 'sending';
UPDATE outbound_messages SET execution_state = 'finished'  WHERE status = 'sent';
UPDATE outbound_messages SET execution_state = 'finished',  verification_outcome = 'verified_success' WHERE status = 'sent';
UPDATE outbound_messages SET execution_state = 'finished',  verification_outcome = 'execution_failed' WHERE status = 'failed';
UPDATE outbound_messages SET execution_state = 'expired'   WHERE status = 'expired';

-- Indexes
CREATE INDEX idx_outbound_execstate ON outbound_messages(org_id, execution_state);
CREATE INDEX idx_outbound_verify    ON outbound_messages(org_id, verification_outcome);
```

### 2.3 Enum

```go
// internal/models/outbound_state.go (NEW)

type ExecutionState string
const (
    ExecPlanned   ExecutionState = "planned"
    ExecExecuting ExecutionState = "executing"
    ExecFinished  ExecutionState = "finished"   // verification_outcome quyết định ý nghĩa
    ExecExpired   ExecutionState = "expired"     // lease/timeout, không thực thi
)

type VerificationOutcome string
const (
    VerifVerifiedSuccess VerificationOutcome = "verified_success"
    VerifContextDrift    VerificationOutcome = "context_drift"
    VerifRateLimited     VerificationOutcome = "rate_limited"
    VerifBlocked         VerificationOutcome = "blocked"
    VerifCaptcha         VerificationOutcome = "captcha"
    VerifShadowRejected  VerificationOutcome = "shadow_rejected"  // FB silently filtered comment
    VerifExecutionFailed VerificationOutcome = "execution_failed"
)
```

### 2.4 Decision matrix

| execution_state | verification_outcome | Meaning |
|-----------------|---------------------|---------|
| planned   | null               | Chờ executor claim |
| executing | null               | Có executor đang chạy |
| finished  | verified_success   | OK — emit engagement event |
| finished  | context_drift      | Wrong target — KHÔNG emit |
| finished  | rate_limited       | FB cap — defer retry |
| finished  | blocked            | Acc bị limit — pause acc |
| finished  | captcha            | Human required |
| finished  | shadow_rejected    | FB ate comment — pause acc + investigation |
| finished  | execution_failed   | Crash/timeout |
| expired   | null               | Lease timeout, never executed |

### 2.5 Cleanup legacy

Cùng PR — không split để tránh callsite trùng:
1. Xoá `OutboundDraft`, `OutboundRejected`, `OutboundApproved`, `OutboundSending`, `OutboundSent`, `OutboundFailed` từ `internal/models/models.go`. Replace bằng helper `outboundStatusFromState(state, outcome) string` cho FE compat.
2. Xoá routes `/outbox/:id/approve`, `/outbox/:id/reject` khỏi backend.
3. Xoá `approveOutbox` / `rejectOutbox` từ [outboxService.ts](../../../../../../frontend/src/modules/autoflow/services/outboxService.ts).
4. Drop key `outbound_mode` khỏi `org_settings`.
5. Update `agent_brain` prompts xoá "DRAFT mode".
6. Migrate FE filters: `CommentingView/PostingView` filter theo `(execution_state, verification_outcome)` thay vì status string.

### 2.6 Files
**Backend**:
- `internal/store/schema.go` — migration v4
- `internal/models/outbound_state.go` NEW — enum
- `internal/models/models.go` — remove legacy constants, add execution_state + verification_outcome fields
- `internal/store/outbound.go` — `ClaimApprovedOutboundForOrg` set state=executing; `FinalizeOutboundAttempt(execID, execState, verifOutcome)` thay vì status terminal.
- `internal/server/agent/outbox_agent.go` — return new shape
- `internal/runtime/verifier.go` — emit (execState, verifOutcome) pair thay vì single status

**Frontend**:
- [outboxService.ts](../../../../../../frontend/src/modules/autoflow/services/outboxService.ts) — drop dead exports + new types
- [CommentingView.tsx](../../../../../../frontend/src/modules/autoflow/components/views/CommentingView.tsx) — filter 2-trục
- [PostingView.tsx](../../../../../../frontend/src/modules/autoflow/components/views/PostingView.tsx) — filter 2-trục
- [DataPrivateView.tsx](../../../../../../frontend/src/modules/autoflow/components/views/DataPrivateView.tsx) — đã xoá OutboundPolicyPanel, double-check

### 2.7 Verification

```powershell
go test ./internal/store/... ./internal/runtime/... ./internal/server/agent/...
go vet ./...
npm --prefix frontend run build

# Smoke
# 1. SQL: SELECT execution_state, verification_outcome, COUNT(*) FROM outbound_messages GROUP BY 1,2;
#    → phải thấy historical rows mapped đúng
# 2. Tạo 1 comment → planned/null → executing/null → finished/verified_success
# 3. Lực drift: post 1 comment lên acc khác post → finished/context_drift
# 4. Approve/reject routes return 404 (đã xoá)
```

---

## 3. PR-2: Engagement Ledger + Derived Touched + Reconciliation + Planner Isolation

### 3.1 Vấn đề

Hiện tại "đã chạm" badge derive từ `action_ledger.outcome = 'succeeded'`. Đây vẫn là **execution layer** trá hình — chỉ filter outcome chứ chưa tách thành business event.

User yêu cầu rõ:
> **ALL downstream systems consume engagement ledger, NOT outbound row status directly.**

### 3.2 Schema — APPEND-ONLY + correction events

```sql
-- Migration: bump schemaBootstrapVersion = 5

CREATE TABLE engagement_events (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  org_id            INTEGER NOT NULL,
  lead_id           INTEGER,                  -- nullable: post-only events không có lead
  outbound_id       INTEGER NOT NULL,         -- audit trail back to source
  event_type        TEXT NOT NULL,            -- 'comment_verified' | 'inbox_verified' | 'post_verified' | 'reaction_verified' | 'engagement_revoked'
  revokes_event_id  INTEGER,                  -- nullable: nếu event_type='engagement_revoked' thì point tới event bị revoke
  revoke_reason     TEXT,                     -- 'reconciliation:no_dom_proof' | 'reconciliation:cross_tenant_drift' | etc
  channel           TEXT NOT NULL,            -- 'facebook' | future: 'instagram'
  actor_account_id  INTEGER NOT NULL,
  target_entity_id  TEXT NOT NULL,            -- canonical post_id / thread_id / profile_id
  target_url        TEXT NOT NULL,
  verified_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  proof_ref         TEXT,                     -- execution_attempts.id hoặc proof.execution_id
  emitted_by        TEXT NOT NULL,            -- 'runtime_verifier' | 'reconciliation:batch_id=X' — provenance
  
  -- Verifier-emitted events: 1 outbound → at most 1 verified event
  -- Reconciliation can append engagement_revoked events referencing the original
  UNIQUE(outbound_id, event_type) ON CONFLICT IGNORE
);

CREATE INDEX idx_engagement_lead     ON engagement_events(org_id, lead_id);
CREATE INDEX idx_engagement_target   ON engagement_events(org_id, target_entity_id);
CREATE INDEX idx_engagement_account  ON engagement_events(org_id, actor_account_id, verified_at);
CREATE INDEX idx_engagement_type     ON engagement_events(org_id, event_type, verified_at);
CREATE INDEX idx_engagement_revokes  ON engagement_events(revokes_event_id) WHERE revokes_event_id IS NOT NULL;
```

**Projection logic** (derived state):

```sql
-- Lead "đã chạm" = có verified event chưa bị revoke
SELECT EXISTS (
  SELECT 1 FROM engagement_events ev
  WHERE ev.org_id = ? AND ev.lead_id = ?
    AND ev.event_type IN ('comment_verified', 'inbox_verified', 'post_verified', 'reaction_verified')
    AND NOT EXISTS (
      SELECT 1 FROM engagement_events rev
      WHERE rev.revokes_event_id = ev.id AND rev.event_type = 'engagement_revoked'
    )
);
```

Lưu ý: KHÔNG bao giờ `DELETE FROM engagement_events`. Lịch sử immutable.

### 3.3 Emission rule (HARD INVARIANT)

```go
// internal/store/engagement.go (NEW)

// EmitEngagementEvent is the ONLY function that writes to engagement_events.
// Called EXCLUSIVELY from FinalizeOutboundAttempt when:
//   1. verification_outcome == VerifVerifiedSuccess
//   2. EnforceTargetIdentity passed
//   3. proof.execution_id matches outbound_messages.execution_id (CAS)
//
// Any other code path that tries to emit is a BUG.
func EmitEngagementEvent(ctx context.Context, tx *sql.Tx, ev EngagementEvent) error {
    if ev.OutboundID == 0 { return errors.New("engagement: outbound_id required") }
    if ev.OrgID == 0      { return errors.New("engagement: org_id required") }
    if ev.TargetEntityID == "" { return errors.New("engagement: target_entity_id required") }
    // INSERT OR IGNORE — idempotent on UNIQUE(outbound_id)
    ...
}
```

### 3.4 Derived touched

```go
// internal/store/lead_engagement.go (REWRITE)

// LeadEngagementBadge derived from engagement_events ONLY.
// No reading from outbound_messages.status, no reading from action_ledger.outcome.
type LeadEngagementBadge struct {
    LeadID         int64
    HasEngagement  bool       // EXISTS(verified event)
    LastEventType  string     // 'comment_verified' etc
    LastEventAt    time.Time
    EventCount     int        // total verified events
    Accounts       []int64    // which accs touched
}

func DeriveLeadBadge(ctx context.Context, db *sql.DB, orgID, leadID int64) (*LeadEngagementBadge, error) {
    // SELECT FROM engagement_events WHERE org_id=? AND lead_id=? ORDER BY verified_at DESC
    ...
}
```

**FE projection**:
- `LeadCard.tsx` — "ĐÃ CHẠM" badge chỉ render khi `lead.engagement.has_engagement === true`.
- Remove any code path reading `lead.last_outbound_status` for badge.

### 3.5 Reconciliation — Dry-run first + Operator Replay UI

**Hard rule**: Reconciliation KHÔNG `DELETE FROM engagement_events`. Phải emit `engagement_revoked` events.

```sql
-- Audit trail của reconciliation
CREATE TABLE reconciliation_batches (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  org_id          INTEGER NOT NULL,
  triggered_by    INTEGER NOT NULL,    -- user_id
  mode            TEXT NOT NULL,        -- 'dry_run' | 'force_apply'
  status          TEXT NOT NULL,        -- 'pending_review' | 'approved' | 'rejected' | 'applied' | 'partially_applied'
  proposal_count  INTEGER NOT NULL DEFAULT 0,
  applied_count   INTEGER NOT NULL DEFAULT 0,
  
  created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
  reviewed_at     DATETIME,
  reviewed_by     INTEGER,
  applied_at      DATETIME
);

CREATE TABLE reconciliation_audit_logs (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  batch_id        INTEGER NOT NULL,
  proposal_type   TEXT NOT NULL,         -- 'emit_engagement' | 'revoke_engagement' | 'mark_action_ledger_failed'
  target_outbound_id INTEGER,
  target_event_id INTEGER,
  reason          TEXT NOT NULL,
  before_state    TEXT,                  -- JSON snapshot
  after_state     TEXT,                  -- JSON proposal
  decision        TEXT,                  -- 'pending' | 'approved' | 'rejected'
  applied         INTEGER NOT NULL DEFAULT 0,
  
  created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

```go
// internal/store/engagement_reconcile.go (REWRITE)

type ProposalType string
const (
    PropEmitEngagement       ProposalType = "emit_engagement"        // outbound verified but no event
    PropRevokeEngagement     ProposalType = "revoke_engagement"      // event exists but outbound contradicts
    PropMarkLedgerFailed     ProposalType = "mark_ledger_failed"     // action_ledger succeeded but no verified
)

type Proposal struct {
    Type              ProposalType
    TargetOutboundID  int64
    TargetEventID     int64
    Reason            string
    BeforeStateJSON   string
    AfterStateJSON    string
}

// ReconcileEngagementState: DEFAULT mode='dry_run'.
// Returns batch_id; UI surfaces proposals for operator review.
// Apply happens via separate ApproveReconciliationBatch call.
func ReconcileEngagementState(ctx context.Context, db *sql.DB, orgID, userID int64, mode string) (batchID int64, proposals []Proposal, err error) {
    // Step 1: Open reconciliation_batches row (mode=dry_run unless explicitly force_apply by admin).
    // Step 2: Scan outbound where verification_outcome='verified_success' AND no engagement_events row exists
    //         → propose emit_engagement.
    // Step 3: Scan engagement_events where event_type IN (verified_*) AND outbound.verification_outcome != 'verified_success'
    //         → propose revoke_engagement (will emit engagement_revoked, never DELETE).
    // Step 4: Scan action_ledger outcome='succeeded' where no matching verified engagement_events
    //         → propose mark_ledger_failed.
    // Step 5: INSERT each proposal as reconciliation_audit_logs row (decision='pending').
    // Step 6: If mode='force_apply', batch through review automatically (admin escape hatch).
    // Return batch + proposals for UI.
}

// ApplyReconciliationBatch: after operator review.
func ApplyReconciliationBatch(ctx context.Context, db *sql.DB, batchID, userID int64) (appliedCount int, err error) {
    // For each audit_log with decision='approved':
    //   if proposal=emit_engagement   → INSERT engagement_events (event_type=comment_verified|...) emitted_by='reconciliation:batch=N'
    //   if proposal=revoke_engagement → INSERT engagement_events (event_type=engagement_revoked, revokes_event_id=X, reason=...) emitted_by='reconciliation:batch=N'
    //   if proposal=mark_ledger_failed → UPDATE action_ledger SET outcome='failed' (action_ledger không phải authoritative, OK update)
    // Mark batch status='applied'.
}
```

**Endpoints**:
- `POST /api/admin/reconcile-engagement` — start dry run, return batch_id + proposals
- `GET /api/admin/reconcile-batches/:id` — fetch detail
- `POST /api/admin/reconcile-batches/:id/decisions` — body `{decisions: [{audit_log_id, decision}]}`
- `POST /api/admin/reconcile-batches/:id/apply` — apply approved proposals (cannot undo, idempotent on re-call)

**Operator Replay UI** (PR-2):
- `frontend/src/modules/autoflow/components/admin/ReconciliationReplayView.tsx` NEW
- List batches → click batch → table proposals với before/after JSON diff
- Per-row Approve/Reject buttons + Apply All Approved button
- Trust building: "AI cannot secretly rewrite its own history."

### 3.6 Planner isolation

```go
// internal/ai/planner.go — REWRITE consumer

// BEFORE (forbidden):
//   recent := db.Query("SELECT * FROM outbound_messages WHERE lead_id=? AND status='sent'")
//
// AFTER (mandatory):
//   recent := db.Query("SELECT * FROM engagement_events WHERE lead_id=? ORDER BY verified_at DESC")
//
// Planner reads ONLY engagement_events. Outbound status is execution-layer noise.

func (p *Planner) HasRecentEngagement(ctx context.Context, leadID int64, window time.Duration) (bool, error) {
    // SELECT EXISTS(... engagement_events WHERE lead_id=? AND verified_at > NOW() - ?)
    ...
}
```

Audit grep: 
```powershell
# After PR-2 ship, this command should return 0 hits in internal/ai/, internal/server/agent/, internal/runtime/planner:
grep -r "outbound_messages.status" --include="*.go" internal/ai/ internal/server/agent/ internal/runtime/
```

### 3.7 Cooldown / rate limit / behaviour profile

Same change — all consume engagement_events, never outbound status:
- `BehaviourProfileRuntime.ShouldBackoff(accountID, leadID)` → count engagement events last N hours.
- `AntiSpamGate.RecentEngagementWithTarget(targetEntityID, days)` → query engagement_events.
- Retry suppressor: nếu lead có engagement event trong 7 ngày → suppress new outbound đến cùng lead.

### 3.8 Files

**Backend NEW**:
- `internal/store/engagement.go` — `EmitEngagementEvent`, `ListEngagementEvents`, `CountEngagementEventsForLead`
- `internal/store/engagement_reconcile.go` — REWRITE existing
- `internal/server/admin/reconcile_routes.go` NEW — POST `/api/admin/reconcile-engagement`

**Backend MODIFY**:
- `internal/store/outbound.go` — `FinalizeOutboundAttempt` calls `EmitEngagementEvent` in same tx when verified_success
- `internal/store/lead_engagement.go` — rewrite, drop `action_ledger.outcome='succeeded'` filter, query engagement_events
- `internal/ai/planner.go` — refactor all status reads to engagement reads
- `internal/runtime/behaviour_profile.go` — same
- `internal/server/leads/list.go` — derive badge from engagement_events

**Frontend MODIFY**:
- `LeadCard.tsx` — badge from `engagement.has_engagement`
- `frontend/src/modules/autoflow/services/leadsService.ts` — LeadEngagement type matches new shape

### 3.9 Verification

```powershell
go test ./internal/store/... ./internal/ai/...
go vet ./...

# Smoke
# 1. Tạo outbound, finish với verified_success → SELECT * FROM engagement_events; thấy 1 row
# 2. Tạo outbound, finish với context_drift → engagement_events vẫn 0 rows
# 3. Bảo ĐồnG case: SELECT * FROM action_ledger WHERE outcome='succeeded' AND outbound_id NOT IN (SELECT outbound_id FROM engagement_events);
#    → trước reconcile: nhiều rows. Chạy POST /api/admin/reconcile-engagement → expect 0 rows.
# 4. Grep audit: no callsite đọc outbound_messages.status từ planner/ai/behaviour.
# 5. Lead badge: "ĐÃ CHẠM" chỉ hiện khi có engagement_events row matching lead_id.
```

---

## 3a. PR-2.5: Account Reputation System (projection)

### 3a.1 Tại sao tách thành PR riêng

User identified: "Without an engine to track if an account is warm/risky/recently blocked, a powerful broadcast planner will burn through the account pool."

Reputation = **projection** từ event log, không phải mutable state. Computed from:
- `engagement_events` (verified_success rate)
- `outbound_messages.verification_outcome` (rate_limited, blocked, shadow_rejected frequency)
- `execution_attempts` (DOM failures)
- Account age, manual flags

### 3a.2 Schema

```sql
-- Migration v5b (cùng PR với engagement, hoặc tách v6)

CREATE TABLE account_reputation_snapshots (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  org_id            INTEGER NOT NULL,
  account_id        INTEGER NOT NULL,
  
  -- Computed metrics (point-in-time)
  trust_level       TEXT NOT NULL,        -- 'warm' | 'standard' | 'risky' | 'cooling' | 'frozen'
  verified_count_24h    INTEGER NOT NULL,
  shadow_rejected_24h   INTEGER NOT NULL,
  rate_limited_24h      INTEGER NOT NULL,
  blocked_24h           INTEGER NOT NULL,
  context_drift_24h     INTEGER NOT NULL,
  age_days              INTEGER NOT NULL,
  
  -- Derived recommendations
  recommended_cooldown_seconds INTEGER NOT NULL,
  recommended_max_per_day      INTEGER NOT NULL,
  
  -- Provenance
  computed_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
  computed_by       TEXT NOT NULL,        -- 'cron:reputation_v1' | 'manual'
  
  -- Snapshot pattern: append-only, never UPDATE old rows
  UNIQUE(org_id, account_id, computed_at)
);

CREATE INDEX idx_reputation_account ON account_reputation_snapshots(org_id, account_id, computed_at DESC);
CREATE INDEX idx_reputation_trust   ON account_reputation_snapshots(org_id, trust_level);
```

### 3a.3 Trust level rules

```go
// internal/reputation/score.go (NEW)

type TrustLevel string
const (
    TrustWarm     TrustLevel = "warm"     // healthy, low activity, ageing
    TrustStandard TrustLevel = "standard" // normal acc, can broadcast moderately
    TrustRisky    TrustLevel = "risky"    // recent rate_limit/drift signals, slow down
    TrustCooling  TrustLevel = "cooling"  // recent blocked/shadow_rejected, freeze broadcast 24h
    TrustFrozen   TrustLevel = "frozen"   // captcha + login wall recently, no outbound until human
)

// Deterministic — same inputs → same level. No ML, no randomness.
func ComputeTrustLevel(m Metrics) TrustLevel {
    if m.AgeDays < 7                      { return TrustWarm }   // newly created acc
    if m.Blocked24h > 0 || m.ShadowRejected24h > 1 { return TrustCooling }
    if m.RateLimited24h > 3 || m.ContextDrift24h > 2 { return TrustRisky }
    if m.VerifiedCount24h > 0             { return TrustStandard }
    return TrustWarm
}

// Recommended cooldown matrix
var cooldownByTrust = map[TrustLevel]Recommendation{
    TrustWarm:     {CooldownSec: 21600, MaxPerDay: 2},   // 6h between sends, 2/day
    TrustStandard: {CooldownSec: 1800,  MaxPerDay: 8},   // 30 min, 8/day
    TrustRisky:    {CooldownSec: 7200,  MaxPerDay: 3},   // 2h, 3/day
    TrustCooling:  {CooldownSec: 86400, MaxPerDay: 0},   // 24h freeze
    TrustFrozen:   {CooldownSec: -1,    MaxPerDay: 0},   // human required
}
```

### 3a.4 Compute pipeline

Cron job (every 15 min) or trigger after each finalize:
```go
// internal/reputation/compute.go
func ComputeForAccount(ctx, orgID, accountID) (*Snapshot, error) {
    // Aggregate last 24h metrics from engagement_events + outbound_messages.verification_outcome
    // SELECT COUNT(*) FILTER (WHERE verification_outcome='rate_limited' AND finished_at > now-24h) ...
    // Insert snapshot row (append-only)
    // Return snapshot for planner consumption
}
```

### 3a.5 Planner & Broadcast integration

Broadcast worker MUST check reputation before enqueue:
```go
// internal/server/templates/broadcast.go
func planNextTarget(target BroadcastTarget) Decision {
    rep := reputation.LatestSnapshot(orgID, target.AccountID)
    if rep.TrustLevel == TrustFrozen { return Skip("acc_frozen_human_required") }
    if rep.TrustLevel == TrustCooling { return Defer(24 * time.Hour) }
    if timeSinceLastSend(accountID) < rep.RecommendedCooldownSec { return Defer(...) }
    if accountDailySendCount(accountID, today) >= rep.RecommendedMaxPerDay { return DeferUntilMidnight() }
    return EnqueueNow()
}
```

### 3a.6 UI

`frontend/src/modules/autoflow/components/admin/AccountReputationView.tsx` NEW:
- Grid: account · trust_level (color badge) · recent metrics · recommended cooldown · last computed
- Filter by trust_level
- Manual override flag (e.g., "frozen → standard" requires admin action + reason logged)

### 3a.7 Files

**Backend NEW**:
- `internal/reputation/score.go` — deterministic trust calc
- `internal/reputation/compute.go` — aggregate + snapshot writer
- `internal/store/reputation.go` — snapshot CRUD (insert + latest-by-account)
- `internal/server/admin/reputation_routes.go` — list, manual override

**Backend MODIFY**:
- `internal/server/templates/broadcast.go` (PR-4 will be aware of this gate)
- `cmd/scraper/main.go` — start reputation compute cron (15 min interval)

**Frontend NEW**:
- `frontend/src/modules/autoflow/components/admin/AccountReputationView.tsx`

### 3a.8 Verification

```powershell
# Smoke
# 1. Acc với 3 rate_limited trong 24h → snapshot row trust_level='risky', cooldown 2h
# 2. Acc với 1 blocked → trust_level='cooling', recommended_max_per_day=0
# 3. Broadcast planner pause khi gặp cooling acc; skip + log skip_reason='acc_cooling'
# 4. Append-only check: 24h tick sau → snapshot mới row, old row vẫn còn
```

---

## 4. PR-3: Template Schema + Render Engine (Behavioral + Knowledge-Bound)

### 4.1 Templates KHÔNG chỉ là text

Phản hồi user: template là **behavioral entity**, không phải string lưu trữ. Khi AI broadcast template aggressive lên mọi nhóm → ban acc. Cần metadata để planner gating.

### 4.2 Schema

```sql
-- Migration: bump schemaBootstrapVersion = 6

CREATE TABLE saved_templates (
  id                  INTEGER PRIMARY KEY AUTOINCREMENT,
  org_id              INTEGER NOT NULL,
  kind                TEXT NOT NULL,         -- 'comment' | 'post' | 'inbox'
  title               TEXT NOT NULL,
  body                TEXT NOT NULL,
  image_path          TEXT,
  
  -- Behavioral metadata (gating planner)
  risk_level          TEXT NOT NULL DEFAULT 'medium',  -- 'low' | 'medium' | 'high'
  persona             TEXT,                  -- 'sourcing_buyer' | 'customer_inquirer' | 'praise_generic' | free-form
  style               TEXT,                  -- 'casual' | 'professional' | 'friendly' | 'urgent'
  cooldown_profile    TEXT,                  -- 'warmup' | 'cautious' | 'standard' | 'aggressive'
  channel_constraints TEXT,                  -- JSON: {"max_per_account_per_day":3,"min_account_age_days":30,"forbidden_groups":[...]}
  
  -- Standard fields
  tags                TEXT,                  -- JSON array
  language            TEXT DEFAULT 'auto',
  source_outbound_id  INTEGER,               -- "save from existing"
  created_by_user_id  INTEGER,
  created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at          DATETIME,
  use_count           INTEGER DEFAULT 0,
  last_used_at        DATETIME,
  archived            INTEGER DEFAULT 0,
  
  UNIQUE(org_id, kind, title)
);

CREATE INDEX idx_template_risk_kind ON saved_templates(org_id, kind, risk_level, archived);
```

### 4.3 Cooldown profiles

```go
// internal/templates/profiles.go

var cooldownProfiles = map[string]CooldownProfile{
    "warmup":     {MaxPerAccountPerDay: 1, MinDelayBetween: 6*time.Hour,  MinAccountAgeDays: 7},
    "cautious":   {MaxPerAccountPerDay: 3, MinDelayBetween: 90*time.Minute, MinAccountAgeDays: 14},
    "standard":   {MaxPerAccountPerDay: 8, MinDelayBetween: 30*time.Minute, MinAccountAgeDays: 0},
    "aggressive": {MaxPerAccountPerDay: 20, MinDelayBetween: 5*time.Minute,  MinAccountAgeDays: 0},
}
```

**Planner gate**: Trước khi enqueue outbound từ template, planner kiểm tra:
- `behaviour_profile_runtime.daily_send_count[account] < cooldownProfile.MaxPerAccountPerDay`
- `behaviour_profile_runtime.last_send_at[account] + MinDelayBetween < now`
- `account.created_at + MinAccountAgeDays < now`

Nếu không pass → defer outbound row với `next_run_at` future, không pollute planned queue.

### 4.4 Placeholder render — Deterministic + Knowledge-bound

**Hard rule**: NO AI rewriting at render time. AI = entropy engine when placed in render path. Deterministic substitution only.

```go
// internal/templates/render.go
// Supported placeholders:
//   {{lead_name}}        — lead.name (runtime context)
//   {{post_excerpt}}     — first 80 chars of source post (runtime context)
//   {{custom:foo}}       — caller-provided kv map
//
// Knowledge placeholders (PULL from knowledge_assets — Compliance Center):
//   {{knowledge:product.price}}      — locked price for SKU
//   {{knowledge:product.url}}        — verified product link
//   {{knowledge:product.short_name}} — SKU short label
//   {{knowledge:warranty.terms}}     — current warranty text
//   {{knowledge:promo.active}}       — active promo text (null if none)
//
// If knowledge placeholder unresolvable → render FAILS with explicit error.
// Never silently swallow — broadcast batch rolls back, operator alerted.

type RenderContext struct {
    OrgID         int64
    LeadName      string
    PostExcerpt   string
    KnowledgeRef  KnowledgeResolver  // injected, pulls from knowledge_assets
    Custom        map[string]string
}

type KnowledgeResolver interface {
    Resolve(orgID int64, path string) (string, error)  // path = "product.price" etc
}

var (
    runtimeRe   = regexp.MustCompile(`\{\{(lead_name|post_excerpt|custom:[a-z_]+)\}\}`)
    knowledgeRe = regexp.MustCompile(`\{\{knowledge:([a-z_.]+)\}\}`)
)

func Render(body string, ctx RenderContext) (string, error) {
    out := body
    
    // Phase 1: deterministic runtime substitution
    out = runtimeRe.ReplaceAllStringFunc(out, func(m string) string {
        // ... simple lookups
    })
    
    // Phase 2: knowledge resolution (FAIL on missing)
    var resolveErr error
    out = knowledgeRe.ReplaceAllStringFunc(out, func(m string) string {
        path := extractKnowledgePath(m)
        val, err := ctx.KnowledgeRef.Resolve(ctx.OrgID, path)
        if err != nil {
            resolveErr = fmt.Errorf("knowledge path %q unresolvable: %w", path, err)
            return m
        }
        return val
    })
    if resolveErr != nil { return "", resolveErr }
    
    return out, nil
}
```

### 4.4a Knowledge Governance link

Template phải declare `required_knowledge_paths` JSON column:
```
required_knowledge_paths: ["product.price", "product.url"]
```

Khi user save template, validator extract `{{knowledge:*}}` từ body, persist set. Khi `knowledge_assets` đổi (price update, URL deprecated), Compliance Center có thể audit "templates using deprecated path X" and proactively warn.

Trust property: comments dispatched by template **100% human-authored structure + Compliance Center-verified facts**. No AI hallucination in pricing/URL/spec.

### 4.5 CRUD endpoints

- `POST /api/templates` — create
- `GET /api/templates?kind=comment&risk=low` — list filtered
- `GET /api/templates/:id` — detail
- `PATCH /api/templates/:id` — update
- `DELETE /api/templates/:id` — archive (soft)
- `POST /api/outbox/:id/save-as-template` — body `{title, tags, risk_level, persona, style, cooldown_profile}`

### 4.6 Files

**Backend NEW**:
- `internal/store/templates.go` — CRUD
- `internal/templates/render.go` — placeholder engine
- `internal/templates/profiles.go` — cooldown profiles registry
- `internal/server/templates/routes.go` — REST

**Frontend NEW**:
- `frontend/src/modules/autoflow/services/templateService.ts` — typed client

### 4.7 Verification
- Create template → list → render with mock context → assert output.
- Cooldown gate: tạo template `aggressive` → mock account vừa send 20 lần hôm nay → planner gate refuse.

---

## 5. PR-4: Template UI + Broadcast Campaigns

### 5.1 Aggregation entities

Phản hồi user: spawn N rows raw không đủ. Cần campaign object để pause/resume/retry/analytics.

```sql
-- Migration v7

CREATE TABLE broadcast_campaigns (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  org_id            INTEGER NOT NULL,
  template_id       INTEGER NOT NULL,
  created_by_user_id INTEGER NOT NULL,
  
  throttle_seconds  INTEGER NOT NULL DEFAULT 60,
  max_targets       INTEGER NOT NULL DEFAULT 100,   -- safety cap
  
  status            TEXT NOT NULL DEFAULT 'running',  -- 'running' | 'paused' | 'completed' | 'cancelled'
  total_targets     INTEGER NOT NULL DEFAULT 0,
  
  created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
  completed_at      DATETIME
);

CREATE TABLE broadcast_targets (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  campaign_id   INTEGER NOT NULL,
  lead_id       INTEGER,
  target_url    TEXT NOT NULL,
  outbound_id   INTEGER,                     -- nullable until enqueued
  
  status        TEXT NOT NULL DEFAULT 'queued',   -- 'queued' | 'enqueued' | 'completed' | 'skipped' | 'failed'
  skip_reason   TEXT,                        -- vd: 'recent_engagement_dedup', 'account_cooldown', 'rate_limit'
  
  scheduled_at  DATETIME,
  enqueued_at   DATETIME,
  
  UNIQUE(campaign_id, target_url)            -- dedup within campaign
);

CREATE INDEX idx_bcast_status ON broadcast_campaigns(org_id, status);
CREATE INDEX idx_btarget_camp ON broadcast_targets(campaign_id, status);
```

### 5.2 Broadcast lifecycle

```go
// internal/server/templates/broadcast.go

func CreateBroadcast(orgID int64, req CreateBroadcastReq) (*Campaign, error) {
    // 1. Resolve template; assert risk_level + cooldown_profile vs current org behaviour
    // 2. Insert broadcast_campaigns row (status=running)
    // 3. For each target, insert broadcast_targets row (status=queued)
    // 4. Dedup check: skip targets with recent engagement_events
    // 5. Don't enqueue outbound yet — broadcast_worker poll
    return campaign, nil
}

// Background worker: poll campaigns where status='running'
// For each campaign, peek N targets where scheduled_at <= now, status='queued'
// Spawn outbound row (planned), update target.outbound_id + status=enqueued
// Respect cooldown_profile gates (cooldown timeout → target.scheduled_at += delay)
func BroadcastWorker(ctx context.Context) { ... }
```

### 5.3 Anti-abuse layered (5 gates + pacing)

1. **Per-broadcast cap**: `max_targets` ≤ 100 default, config `BROADCAST_MAX_TARGETS_HARD_CAP=200`.
2. **Throttle min**: ≥ 30s, hard cap `BROADCAST_MIN_THROTTLE=30`.
3. **Template cooldown profile gate** (PR-3): respect template's cooldown_profile.
4. **Account Reputation gate** (PR-2.5): refuse enqueue if trust_level IN ('cooling', 'frozen'); defer if 'risky'.
5. **Engagement dedup**: skip target nếu `engagement_events` đã có verified row chưa bị revoke với cùng `target_entity_id` trong 7 ngày → skip_reason='recent_engagement_dedup'.
6. **Per-account daily quota** (from reputation snapshot's recommended_max_per_day).

**Pacing rules** (chống bot pattern detection):
- **Jitter**: actual `next_run_at` = scheduled + random(0, throttle * 0.3) seconds. Linear interval = bot signal.
- **Heat dispersion**: không enqueue 2 outbound liên tiếp dùng cùng account. Round-robin qua acc pool.
- **Burst cap**: max 5 enqueues per minute toàn broadcast (regardless of throttle setting).
- **Hour-of-day window**: default chỉ enqueue 7am-11pm local. Config override.

### 5.4 Frontend

**New folder** `frontend/src/modules/autoflow/components/templates/`:
- `TemplatePicker.tsx` — modal show by kind, filter by risk/persona/tag.
- `TemplateCard.tsx` — show body preview, risk badge (color: green=low, yellow=medium, red=high), use_count.
- `SaveAsTemplateDialog.tsx` — appear after `verified_success` outbound: title, tags, risk_level dropdown, cooldown_profile dropdown.
- `TemplatesView.tsx` (Settings sub-tab) — CRUD listing.

**New folder** `frontend/src/modules/autoflow/components/broadcast/`:
- `BroadcastView.tsx` — main view: list campaigns + create button.
- `CreateBroadcastFlow.tsx` — wizard: pick template → pick targets (lead filter UI) → throttle → review → confirm.
- `CampaignDetail.tsx` — show campaign with target list grid (queued/enqueued/completed/skipped + skip_reason).
- Pause / resume / cancel buttons on campaign.

### 5.5 Files

**Backend NEW**:
- `internal/store/broadcast.go` — campaign + target CRUD
- `internal/server/templates/broadcast.go` — handler + worker

**Backend MODIFY**:
- `internal/server/templates/routes.go` — wire broadcast routes
- `cmd/scraper/main.go` — start BroadcastWorker goroutine

**Frontend NEW**: templates/ + broadcast/ folders above.

**Frontend MODIFY**:
- [CommentingView.tsx](../../../../../../frontend/src/modules/autoflow/components/views/CommentingView.tsx) — "Chọn mẫu" button → TemplatePicker
- [PostingView.tsx](../../../../../../frontend/src/modules/autoflow/components/views/PostingView.tsx) — same
- [SettingsPage.tsx](../../../../../../frontend/src/modules/autoflow/components/SettingsPage.tsx) — + Templates tab
- [FacebookWorkspaceApp.tsx](../../../../../../frontend/src/modules/autoflow/components/FacebookWorkspaceApp.tsx) — + Broadcast nav entry

### 5.6 Verification

```powershell
# Smoke
# 1. Save 1 verified_success outbound as template, risk=medium, cooldown=standard.
# 2. Create broadcast: template + 5 leads, throttle=60s.
# 3. SQL: SELECT * FROM broadcast_targets WHERE campaign_id=?; → 5 queued rows
# 4. Wait worker tick: 1 row enqueued every 60s. Spy outbound_messages: 5 planned rows over 5 minutes.
# 5. Pause campaign mid-flight → worker stops enqueuing new ones. Resume → continues.
# 6. Lead with recent engagement: SELECT skip_reason from broadcast_targets WHERE ... → 'recent_engagement_dedup'.
# 7. Each outbound finishes verified_success → engagement_events row + template.use_count incremented.
```

---

## 6. PR-5: Telegram Multi-Tenant (BotRuntime Lifecycle)

### 6.1 Phản hồi user: bot isolation boundary

```
TelegramManager
  └── BotRuntime[orgID]
      ├── Context (per-bot ctx with cancel)
      ├── Lifecycle (started_at, last_update_at)
      ├── Health (heartbeat, error counters)
      ├── Backoff (exponential on connect failures)
      └── ShutdownHook (deregister webhook, drain queue, close)
```

Tránh `map[orgID]*Bot` raw process-lifetime.

### 6.2 Schema

```sql
-- Migration v8

CREATE TABLE org_telegram_bots (
  org_id          INTEGER PRIMARY KEY,
  bot_token       TEXT NOT NULL,
  bot_token_hash  TEXT NOT NULL UNIQUE,   -- sha256 hash for webhook URL routing
  bot_username    TEXT NOT NULL,
  admin_chat_id   INTEGER,
  enabled         INTEGER NOT NULL DEFAULT 1,
  
  -- Runtime state (read-only mirror)
  last_health_at  DATETIME,
  last_error      TEXT,
  
  created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at      DATETIME
);

ALTER TABLE users ADD COLUMN telegram_chat_id INTEGER;
ALTER TABLE users ADD COLUMN telegram_username TEXT;
CREATE UNIQUE INDEX idx_users_tele_chat ON users(telegram_chat_id) WHERE telegram_chat_id IS NOT NULL;

-- Linking flow
CREATE TABLE telegram_link_tokens (
  token        TEXT PRIMARY KEY,
  org_id       INTEGER NOT NULL,
  user_id      INTEGER NOT NULL,
  expires_at   DATETIME NOT NULL,
  used_at      DATETIME
);
```

### 6.2a Multi-tenant isolation — failure containment

**Hard rule**: 1 org's broken bot KHÔNG được crash supervisor cho org khác.

- Mỗi `BotRuntime` chạy goroutine riêng với `defer recover()`.
- Panic / fatal error → log + mark runtime unhealthy + Supervisor backoff schedule restart.
- Supervisor loop wraps each tick in `func(orgID) { defer recover(); ... }`.
- Bot token expired / rotated externally → status='auth_failed', emit org admin alert, KHÔNG retry indefinitely (max 3 backoff cycles thì hold and wait manual).
- Webhook handler bị nhiều invalid update (spam, malformed) → per-runtime rate limit (≤ 50 updates/sec), drop excess.

### 6.3 BotRuntime contract

```go
// internal/telegram/runtime.go (NEW)

type BotRuntime struct {
    orgID    int64
    bot      *tele.Bot
    ctx      context.Context
    cancel   context.CancelFunc
    
    health   *HealthMonitor
    backoff  *BackoffState
    
    deps     RuntimeDeps  // db, agentBus, broadcastBus
}

type HealthMonitor struct {
    StartedAt       time.Time
    LastUpdateAt    atomic.Value
    ErrorCount24h   atomic.Int64
    ConsecutiveErrs atomic.Int64
}

func (r *BotRuntime) Start() error          // attach webhook OR start polling, kick health tick
func (r *BotRuntime) Stop(ctx context.Context) error  // graceful: deregister webhook + drain + close
func (r *BotRuntime) HealthStatus() HealthStatus
func (r *BotRuntime) onError(err error)     // updates backoff, may trigger restart

// Manager-level
type Manager struct {
    runtimes sync.Map  // orgID → *BotRuntime
    deps     RuntimeDeps
    supervisor *Supervisor
}

func (m *Manager) RegisterOrgBot(orgID int64, cfg OrgBotConfig) error
func (m *Manager) UnregisterOrgBot(orgID int64) error
func (m *Manager) ReloadOrgBot(orgID int64) error  // token rotation
func (m *Manager) HealthSnapshot() []HealthStatus  // for admin dashboard

// Supervisor: tick every 30s, check each runtime health, restart unhealthy with backoff.
type Supervisor struct {
    interval time.Duration
    mgr      *Manager
}
```

### 6.4 Webhook routing (no polling for prod)

`POST /api/telegram/webhook/:token_hash`:
1. Extract `token_hash` from URL.
2. `db.LookupOrgByTokenHash(token_hash)` → orgID. 404 if not found.
3. Resolve `BotRuntime` from Manager → if not running, reject 503.
4. Dispatch update to runtime's handler queue.

Fallback dev: `TELEGRAM_USE_POLLING=1` → Supervisor spawns `tele.LongPoller` per runtime.

### 6.5 Linking flow

1. FE user clicks "Liên kết Telegram" in Settings → POST `/api/telegram/link-token`.
2. Backend generates token (32 random bytes hex), stores in `telegram_link_tokens` (expires 10 min).
3. Returns `t.me/<bot_username>?start=<token>`.
4. User taps link → Telegram opens bot → bot receives `/start <token>`.
5. Webhook handler: lookup token → mark `telegram_link_tokens.used_at`, INSERT `users.telegram_chat_id`.
6. Send confirmation: "✅ Đã liên kết với workspace {{org_name}}, user {{user_name}}".

### 6.6 Cross-tenant invariant

```go
// Inside webhook handler:
func dispatch(update tele.Update, runtime *BotRuntime) {
    orgID := runtime.orgID  // determined by token, immutable
    
    chatID := update.Message.Chat.ID
    user, ok := db.LookupUserByTelegramChat(chatID, orgID)
    if !ok {
        // Not linked yet. Only allow /start <token>.
        handleStartCommand(...)
        return
    }
    
    // Hard invariant: user.org_id MUST equal runtime.orgID.
    if user.OrgID != orgID {
        log.WithFields(...).Error("cross-tenant attempt")  // never delete this log
        return
    }
    
    // From here, all agent calls thread (orgID, userID).
    ctx := withOrgUser(context.Background(), orgID, user.ID)
    handleUpdate(ctx, update)
}
```

### 6.7 Migration

Boot path: nếu env có `TELEGRAM_BOT_TOKEN` cũ và `org_telegram_bots` rỗng → auto seed row org_id=1 với token đó. Log deprecation warning.

### 6.8 Files

**Backend NEW**:
- `internal/store/telegram_bots.go` — CRUD
- `internal/store/telegram_link_tokens.go` — link flow
- `internal/telegram/runtime.go` — BotRuntime + HealthMonitor
- `internal/telegram/manager.go` — Manager + Supervisor
- `internal/server/telegram/webhook.go` — HTTP handler
- `internal/server/telegram/admin_routes.go` — admin: configure bot, view health

**Backend MODIFY**:
- `internal/telegram/bot.go` — refactor into runtime-driven, drop hardcoded orgID
- `cmd/scraper/main.go` — boot Manager instead of single bot

**Frontend NEW**:
- `frontend/src/modules/autoflow/services/telegramService.ts`
- `frontend/src/modules/autoflow/components/telegram/TelegramBotSettings.tsx`
- `frontend/src/modules/autoflow/components/telegram/TelegramLinkButton.tsx`

**Frontend MODIFY**:
- [SettingsPage.tsx](../../../../../../frontend/src/modules/autoflow/components/SettingsPage.tsx) — + Telegram tab

### 6.9 Verification

```powershell
# Cross-tenant smoke
# 1. Create 2 orgs, each configures distinct bot token.
# 2. Webhook hits /api/telegram/webhook/<hash_org1> → handler resolves orgID=1.
# 3. Same chat tries to link to org 2 → blocked (UNIQUE telegram_chat_id).
# 4. Bot runtime unhealthy 3 ticks → supervisor restarts with backoff.
# 5. Stop org 1 bot → deregister webhook with Telegram → org 2 bot still alive.
# 6. Health endpoint: GET /api/telegram/admin/health → array of runtime statuses.
```

---

## 7. PR-6: Telegram ↔ Copilot Bridge (Session Authority)

### 7.1 Phản hồi user: session_id explicit

Sai: `(org_id, user_id) = 1 session` — vì user mở nhiều tab/task/context.

Đúng:
```
agent_session (explicit id, can have many per user)
   ↓ (1-N)
agent_session_channels (binding: session_id ↔ channel_type ↔ channel_ref)
```

### 7.2 Schema

```sql
-- Migration v9

CREATE TABLE agent_sessions (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  org_id          INTEGER NOT NULL,
  user_id         INTEGER NOT NULL,
  title           TEXT,                              -- user-rename-able, like ChatGPT threads
  active          INTEGER NOT NULL DEFAULT 1,
  last_seen_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
  created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_session_user ON agent_sessions(org_id, user_id, last_seen_at DESC);

CREATE TABLE agent_session_channels (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id    INTEGER NOT NULL,
  channel_type  TEXT NOT NULL,         -- 'web' | 'telegram'
  channel_ref   TEXT NOT NULL,         -- web: client_id (tab uuid). telegram: chat_id as string.
  bound_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
  
  UNIQUE(channel_type, channel_ref)    -- one tele chat ↔ one session at a time
);

CREATE TABLE agent_messages (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id    INTEGER NOT NULL,
  role          TEXT NOT NULL,         -- 'user' | 'assistant' | 'system'
  channel_type  TEXT NOT NULL,         -- which channel emitted (for "via Telegram" badge)
  content       TEXT NOT NULL,
  image_path    TEXT,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_messages_session ON agent_messages(session_id, created_at);
```

### 7.3 Session binding rules

**Web (FE)**:
- Tab gets `client_id` uuid (localStorage).
- Tab calls `POST /api/sessions/bind { client_id, session_id? }`. If `session_id` omitted → backend creates new session. Returns `session_id`.
- FE SSE: `GET /api/copilot/stream?session_id=X` (auth scoped to user.org_id).

**Telegram**:
- Default: each user has ONE "default tele session" (auto-created on link). Binding row exists since linking.
- Future: `/switch` command to bind tele chat to another session_id (defer).

**Same user, multiple tabs**:
- Tab A bound to session 1, Tab B bound to session 2 → independent histories.
- Tab A and Tab B both bound to session 1 → both see same messages (broadcast to all clients of session_id).

### 7.4 Message flow

```
User types "Hello" on Telegram chat (chat_id=999)
  ↓
Webhook handler resolves chat_id → session (via agent_session_channels)
  ↓
INSERT agent_messages(session_id, role=user, channel_type=telegram, content="Hello")
  ↓
Notify session bus: SessionMessageEmitted(session_id)
  ↓
[Parallel]
  ├── SSE subscribers (web tabs bound to session_id) push message
  └── Agent worker picks up: agent.Run(ctx{org,user,session}, "Hello") → response "Hi"
       ↓
       INSERT agent_messages(session_id, role=assistant, channel_type=web|telegram, content="Hi")
       ↓
       Broadcast to ALL bindings:
         ├── SSE → web tabs
         └── If tele binding exists → bot.Send(chat_id, "Hi")
```

### 7.5 Concurrency

- Per-session lock: at most 1 `agent.Run` per session in-flight. New incoming message queues.
- Drop messages from same session within 200ms (typo correction).
- SSE broadcasts ordered by `created_at`.

### 7.6 Files

**Backend NEW**:
- `internal/store/agent_sessions.go`
- `internal/store/agent_messages.go`
- `internal/server/agent/session_routes.go` — bind, list sessions, rename
- `internal/server/agent/copilot_stream.go` — SSE
- `internal/server/agent/copilot_send.go` — POST send text (image in PR-7)
- `internal/agent/session_bus.go` — pub/sub for cross-channel broadcast

**Backend MODIFY**:
- `internal/telegram/runtime.go` — text handler hooks into session_bus
- `internal/ai/agent.go` — accept session_id in ctx, log to agent_messages

**Frontend MODIFY**:
- [WorkspaceChatView.tsx](../../../../../../frontend/src/modules/autoflow/components/views/WorkspaceChatView.tsx) — EventSource subscribe, session picker dropdown
- `frontend/src/modules/autoflow/services/copilotService.ts` NEW
- Session list sidebar (like ChatGPT history)

### 7.7 Verification

```powershell
# Smoke
# 1. Mở 2 tab copilot, mỗi tab tạo session khác nhau. Gõ trên tab A không hiện trên tab B.
# 2. 1 user link tele → tele bot gắn vào "default session" của user. Gõ trên tele → hiện trên tab nào? → tab bound vào "default session".
# 3. Gõ trên copilot session 1 → tele bot echo (vì default session === session 1).
# 4. Switch tab session: tab A re-bind sang session 2 → từ giờ tab A nhận messages session 2.
# 5. Race: gõ tele + copilot cùng lúc → cả 2 message vào DB, agent_run lock đảm bảo 1 agent.Run tại 1 thời điểm.
```

---

## 8. PR-7: Copilot Image Attach

### 8.1 Scope

Sau khi bridge xong (PR-6), thêm image:
- FE upload UI
- BE multipart endpoint
- AI vision wiring
- Outbound `image_path` populated from session context

### 8.2 Files

**Backend MODIFY**:
- `internal/server/agent/copilot_send.go` — accept multipart, save image, INSERT agent_messages.image_path
- `internal/ai/agent.go` — when message has image_path, build OpenAI vision content blocks
- `internal/server/agent/images.go` — reuse existing storage

**Frontend MODIFY**:
- [WorkspaceChatView.tsx](../../../../../../frontend/src/modules/autoflow/components/views/WorkspaceChatView.tsx) — paperclip + file input + drag/drop + preview thumb
- Render image inline in message bubble

### 8.3 Limits
- 5MB max per image
- 1 image per message (multi-image defer to PR-8)
- MIME: png/jpg/webp/gif

### 8.4 Verification
- Upload product image + prompt "comment khoe sản phẩm" → agent vision reasoning logs image
- Outbound row spawned has `image_path` populated → comment posts with image
- Tele bot already supports image upload — verify bridge syncs both directions

---

## 9. Risks tổng (REVISED)

| Risk | Mức | Mitigation |
|------|-----|-----------|
| Reconcile job over-corrects → wipes legitimate engagement | **HIGH** | Dry-run mode first (report only, no write). Admin reviews diff. Then run for-real. |
| outbound_messages.status read by some downstream code we missed | **HIGH** | Grep audit gate in PR-2 CI. New rule: PR-2 ships with linter rule blocking string `outbound_messages.status` outside `internal/store/outbound*.go` |
| Template cooldown profile too aggressive → ban acc | High | Default new templates `cooldown=cautious`. UI shows risk badge prominently. |
| Broadcast worker race with planner (same lead 2 paths) | Medium | broadcast_targets UNIQUE constraint + engagement dedup at enqueue time |
| Telegram bot runtime restart loop | Medium | Supervisor backoff 30s→1min→5min→15min cap; circuit breaker after 10 consecutive fails |
| SSE connection scaling | Medium | Per-user max 5 concurrent SSE, idle timeout 5min, server-sent reconnect hint |
| Cross-tenant Telegram (existing bug) | **CRITICAL** | PR-5 ships fix |
| Image storage growth | Medium | Cron purge agent_messages images > 90d, outbound images > 30d post-verified |

---

## 10. Non-goals (rõ ràng OUT of scope)

- **KHÔNG** AI-generated templates (staff viết, AI dùng).
- **KHÔNG** template version history (chỉ updated_at).
- **KHÔNG** template marketplace nội bộ (defer).
- **KHÔNG** undo broadcast.
- **KHÔNG** multi-image trong copilot.
- **KHÔNG** voice/video.
- **KHÔNG** Slack/Discord bridge.
- **KHÔNG** mobile native app.
- **KHÔNG** thay đổi vision model (`gpt-4o-mini` standard).
- **KHÔNG** translation auto vi↔en cho template render.
- **KHÔNG** giữ legacy `OutboundDraft/OutboundRejected` (xoá hoàn toàn ở PR-1).

---

## 11. Câu hỏi cần user xác nhận

### 11.1 Template placeholder
- **(A) Simple string replace** `{{name}}` — recommended, no dep.
- (B) Mustache lib.
- (C) AI-only rewrite.

### 11.2 Broadcast caps
- Hard max targets per campaign: **100** OK? (config env override)
- Min throttle: **30s** OK?

### 11.3 Reconciliation strategy
- **(A) Dry-run first** (PR-2 ships with report-only mode, admin reviews, then `force=true` flag). Recommended.
- (B) Auto-run on boot.
- (C) Manual-only (no auto).

### 11.4 Telegram dev mode
- **(A) Hybrid**: webhook prod + polling dev (env switch). Recommended.
- (B) Webhook-only (need ngrok for dev).
- (C) Polling-only first (defer webhook to v2).

### 11.5 Session UX
- **(A) Auto-create default session per user**, FE doesn't expose UI to manage. Recommended for v1.
- (B) ChatGPT-style session list sidebar from day 1.
- (C) One session per user permanent (no multi-session).

---

## 12. File list tổng hợp (7 PRs)

### Backend new
```
internal/models/outbound_state.go                          # PR-1
internal/store/engagement.go                               # PR-2
internal/server/admin/reconcile_routes.go                  # PR-2
internal/store/templates.go                                # PR-3
internal/templates/render.go                               # PR-3
internal/templates/profiles.go                             # PR-3
internal/server/templates/routes.go                        # PR-3
internal/store/broadcast.go                                # PR-4
internal/server/templates/broadcast.go                     # PR-4
internal/store/telegram_bots.go                            # PR-5
internal/store/telegram_link_tokens.go                     # PR-5
internal/telegram/runtime.go                               # PR-5
internal/telegram/manager.go                               # PR-5
internal/server/telegram/webhook.go                        # PR-5
internal/server/telegram/admin_routes.go                   # PR-5
internal/store/agent_sessions.go                           # PR-6
internal/store/agent_messages.go                           # PR-6
internal/server/agent/session_routes.go                    # PR-6
internal/server/agent/copilot_stream.go                    # PR-6
internal/server/agent/copilot_send.go                      # PR-6
internal/agent/session_bus.go                              # PR-6
```

### Backend modify
```
internal/store/schema.go                  # bump v4-v9
internal/store/outbound.go                # PR-1: state+outcome; PR-2: emit engagement
internal/store/lead_engagement.go         # PR-2: derive from engagement_events
internal/store/engagement_reconcile.go    # PR-2: rewrite reconciler
internal/models/models.go                 # PR-1: drop legacy, add new fields
internal/ai/planner.go                    # PR-2: read engagement only
internal/runtime/behaviour_profile.go     # PR-2: read engagement only
internal/runtime/verifier.go              # PR-1: emit state pair
internal/server/agent/outbox_agent.go     # PR-1: new shape
internal/server/leads/list.go             # PR-2: badge from engagement
internal/telegram/bot.go                  # PR-5: runtime-driven
internal/ai/agent.go                      # PR-6: session ctx; PR-7: vision blocks
cmd/scraper/main.go                       # PR-4: BroadcastWorker; PR-5: Manager; PR-6: session bus
```

### Frontend new
```
frontend/src/modules/autoflow/services/templateService.ts          # PR-3
frontend/src/modules/autoflow/services/copilotService.ts           # PR-6
frontend/src/modules/autoflow/services/telegramService.ts          # PR-5
frontend/src/modules/autoflow/components/templates/TemplatePicker.tsx        # PR-4
frontend/src/modules/autoflow/components/templates/TemplateCard.tsx          # PR-4
frontend/src/modules/autoflow/components/templates/SaveAsTemplateDialog.tsx  # PR-4
frontend/src/modules/autoflow/components/templates/TemplatesView.tsx         # PR-4
frontend/src/modules/autoflow/components/broadcast/BroadcastView.tsx         # PR-4
frontend/src/modules/autoflow/components/broadcast/CreateBroadcastFlow.tsx   # PR-4
frontend/src/modules/autoflow/components/broadcast/CampaignDetail.tsx        # PR-4
frontend/src/modules/autoflow/components/telegram/TelegramBotSettings.tsx    # PR-5
frontend/src/modules/autoflow/components/telegram/TelegramLinkButton.tsx     # PR-5
```

### Frontend modify
```
frontend/src/modules/autoflow/services/outboxService.ts                                  # PR-1: drop dead
frontend/src/modules/autoflow/services/leadsService.ts                                   # PR-2: engagement shape
frontend/src/modules/autoflow/components/views/CommentingView.tsx                        # PR-1, PR-4
frontend/src/modules/autoflow/components/views/PostingView.tsx                           # PR-1, PR-4
frontend/src/modules/autoflow/components/views/DataPrivateView.tsx                       # PR-1
frontend/src/modules/autoflow/components/views/WorkspaceChatView.tsx                     # PR-6, PR-7
frontend/src/modules/autoflow/components/leads/LeadCard.tsx                              # PR-2: badge source
frontend/src/modules/autoflow/components/SettingsPage.tsx                                # PR-4, PR-5
frontend/src/modules/autoflow/components/FacebookWorkspaceApp.tsx                        # PR-4
frontend/src/modules/autoflow/i18n/strings.ts                                            # all PRs
```

---

## 13. Follow-ups sau khi 7 PR ship

- **PR-8**: Multi-image messages, template analytics dashboard (conversion rate per template = verified_engagement_count / outbound_count).
- **PR-9**: AI suggest "save as template" tự động khi detect pattern lặp.
- **PR-10**: Discord/Slack bridge (same pattern as Telegram).
- **PR-11**: Template version history + A/B framework.
- **PR-12**: Cross-org template marketplace.

---

## 14. Tóm lược triết lý — đọc lại trước mỗi PR

> Hệ thống đang chuyển từ **"có hành động xảy ra"** sang **"có sự kiện được xác minh"**.
> 
> Outbound = hành động (intent + transport).
> Engagement = sự thật (verified outcome).
> 
> Mọi business logic — score, plan, cooldown, badge, retry — phải đứng trên engagement, không phải outbound.
> 
> Đây là khác biệt giữa "automation tool" và "operating system".
