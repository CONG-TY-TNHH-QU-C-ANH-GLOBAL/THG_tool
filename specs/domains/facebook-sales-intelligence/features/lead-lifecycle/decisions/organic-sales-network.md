# Organic Sales Network v1 — Founder Architecture Directive

> Design doc cho hướng kiến trúc multi-member ownership / execution / attribution / coordination /
> campaign. Đây là nguồn chân lý cho các PR0–PR8 (xem Rollout order). Implementation theo từng PR nhỏ.

## 0. Vision & Final Rule (nền tảng, không thương lượng)
**Organic Sales Network / Organic Distribution Platform** — nhiều member, nhiều FB account, nhiều browser
agents cùng tạo hiện diện thương hiệu & hỗ trợ bán hàng organic. **KHÔNG phải CRM.**

**Final Rule — 4 khái niệm KHÔNG được trộn lẫn trong implementation (tách module rõ):**

| Resource | Bản chất | Domain code | Tầng |
|---|---|---|---|
| **Lead** | SHARED (org) | `leads/` (read-only theo nghĩa ownership) | tài nguyên chung |
| **Execution** | OWNED (`created_by` bất biến) | `outbound/` + `coordination/` | tài sản thực thi |
| **Contribution** | DERIVED | module attribution (đọc Interaction Ledger) | projection |
| **Campaign** | ORCHESTRATION | `campaigns/` (tương lai, additive) | điều phối |

**Lead luôn shared:** không bao giờ có `lead.owner_id` / `assigned_user_id` / territory / lock / exclusivity.

## Cross-cutting: Event-Sourced Core (hợp nhất Ledger + Domain Events)
**Interaction Event Ledger = append-only Domain-Event store.** Một sự thật bất biến duy nhất; mọi thứ
"derived" (Attribution, Champion, KPI, Coordination-view, Leaderboard) là **PROJECTION rebuildable** từ
event stream — KHÔNG lưu trạng thái sở hữu/aggregate riêng có thể lệch. Hệ quả:
- Ghi execution → **append** một event (`ExecutionCompleted`/`CommentPosted`/...), bất biến.
- Projection cập nhật bằng cách **subscribe** event (không gọi chéo domain). Có thể **rebuild lại từ đầu**.
- Khớp append-only ledger hiện có (`internal/store/coordination/action_ledger.go`,
  `specs/domains/facebook-sales-intelligence/features/outbound-actions/implementation/append-only-ledger.md`) — mở rộng tự nhiên, không phải khái niệm mới.

## Engineering principles (áp cho mọi PR)
Clean architecture · single responsibility · deterministic · explicit ownership · additive migrations ·
backward compatible · reusable abstractions · **observable failures + typed reason codes** · no hidden
magic · **no heuristic routing** · **event-sourced (append-only, projections rebuildable)**. Mục tiêu:
ít coupling, dễ test, dễ mở rộng, ít bug. Không quick-fix làm tăng tech debt. **Mỗi PR ship kèm unit test
store-level** (`internal/store/storetest`).

---

## 1. Ownership Layer (INVARIANT) — chỉ ở tài sản thực thi
**Account** — 1 FB identity = 1 account; thuộc 1 member; member chỉ thấy/dùng account của mình; no-steal.
- `UNIQUE(org_id, fb_user_id)` (partial, sau dedup). `ResolveOrCreateAccountForFacebookIdentity(orgID,
  ownerUserID, fbUserID, meta, email)`. Login FB người khác → 409 + audit + typed reason `ownership_conflict`
  surface ra UX. Auto-create khi pair `account_id=0`.
- **Audit & event chỉ phát khi ĐỔI STATE** (create / rebind / conflict), KHÔNG mỗi heartbeat.
- Files: `internal/store/identities/accounts.go`, `internal/server/agent/heartbeat.go:110`, `internal/server/org/identity.go`.

**Connector** — thuộc member tạo (`created_by`); bind đúng account theo FB identity (rebind qua
`AssignAgentAccount`); RBAC tuyệt đối. Files: `internal/store/connectors/`.

**Execution** — **`created_by` bất biến = nguồn chân lý duy nhất.** Đổi chủ account sau này, lịch sử vẫn
ghi đúng người thực hiện. KHÔNG suy ownership execution từ `account_id`.

## 2. Execution Layer (INVARIANT) — ownership bất biến + idempotency
Schema additive trong `migrate()` (`internal/store/schema.go`), bump version 7→8:
```sql
ALTER TABLE outbound_messages  ADD COLUMN created_by INTEGER NOT NULL DEFAULT 0;
ALTER TABLE action_ledger      ADD COLUMN created_by INTEGER NOT NULL DEFAULT 0;
ALTER TABLE execution_attempts ADD COLUMN created_by INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_action_ledger_member ON action_ledger(org_id, created_by, performed_at DESC);
```
`QueueOutboundForOrg` nhận `CreatedBy` → ghi xuống cả 3 bảng. Idempotency CAS (`execution_id`) giữ nguyên.

## 3. Deterministic ExecutionContext (INVARIANT) — campaign-ready abstraction
`ActionContext { OrgID; Source: manual|campaign|ai_agent|scheduled|workflow; ExecutionSourceID/SessionID;
InitiatorUserID(=created_by); AccountID; ConnectorID(0=connectorless); CampaignID }`.
- `ResolveUserActionContext(caller)` → ActionContext(Source=manual); tương lai `ResolveCampaignActionContext`
  cùng shape. Queue/execution chỉ phụ thuộc `ActionContext`.
- **Resolution deterministic (no heuristic, no auto-magic):** Explicit account_id → user default → (đúng 1
  account owned → dùng) → lỗi `execution_context_required` (khi ≥2 & chưa chọn). KHÔNG âm thầm set default.
- `user_execution_context(org_id, user_id, default_account_id)`; UI Settings Default Account/Connector picker.
- **Connector availability:** resolve xong, account không có connector online của caller → typed reason
  `connector_offline` (không queue treo).
- Typed reason codes (`execution_context_required`/`connector_offline`/`ownership_conflict`) surface ra
  copilot/UX + deep-link Settings.
- Sửa `cmd/scraper/outbound_actions.go:resolveCallerAccountID` thành producer của `ActionContext`.

## 4. Attribution Layer (DERIVED) — Interaction Events GENERIC (agent-agnostic)
Interaction Event Ledger = `action_ledger` tổng quát hoá (+`created_by`). 2 chiều **không Facebook-centric**:
`InteractionType` (COMMENT|REPLY|MESSAGE|FOLLOW_UP|SHARE|REACTION|INVITE|JOIN_GROUP|...) ×
`Channel` (FACEBOOK|EMAIL|TELEGRAM|ZALO|...). Thêm cột `channel TEXT NOT NULL DEFAULT 'facebook'` (additive).
- Attribution derive 100% từ (Interaction Event + `created_by`). KHÔNG hardcode role.
- `internal/store/leads/lead_engagement.go:261`: JOIN qua `created_by` (bất biến) thay `account.assigned_user_id`.
  Nối `staff_kpi` (trọng số `kpi_config`).
- Inbound (`ReplyReceived`/`LeadResponded`) attribute cho member của outbound gần nhất kích hoạt (thread linkage).
  action_ledger hiện chỉ outbound; inbound-event đầy đủ là việc tương lai (additive).
- `created_by=0` = system/unattributed → loại khỏi leaderboard/champion.

## 5. Champion Model (ANALYTICS-ONLY)
Champion = projection cho leaderboard/KPI. KHÔNG quyền/ưu tiên routing/execution/lead ownership.
`Ownership ⊥ Champion`. Không hàm execution/routing nào đọc champion để quyết định.

## 6. Coordination Layer (OBSERVABILITY + POLICY, KHÔNG invariant)
Mặc định: nhiều member/account cùng tương tác 1 lead = HỢP LỆ (amplification-friendly). KHÔNG hardcode
cross-member block.
- Observability: `executing_by[]`, `active_contributors[]`, `recent_interactions[]`, `champion`, `active_campaigns[]`.
- Policy qua `action_policies` (đã có): mọi skip phải typed reason code + explainable + observable. Giới hạn
  cross-member là opt-in qua chiều `coordination_scope` (additive, mặc định OFF).

## 7. Campaign Layer (ORCHESTRATION) — chưa build, để chỗ đúng
`Campaign → Accounts → Connectors → Leads → Actions`. Additive (thêm `campaigns/` + resolver).
> **INVARIANT: Campaign KHÔNG phụ thuộc Facebook.** Campaign là abstraction; Facebook chỉ là một channel.
> Điều phối theo `(InteractionType, Channel)` + `ActionContext`. Cấm `campaign = facebook_campaign`.

## 8. Long-term direction (ghi nhận, không code ngay)
**8a. Tách Identity → Account → Connector** (3 thực thể): Identity riêng, Account tham chiếu Identity,
Connector tham chiếu Account (1 FB ↔ nhiều browser/device). v1 vẫn 1 Identity≈1 Account; không chặn tách sau.
**8b. Domain Events / Event Bus:** business events first-class (`ExecutionCompleted/CommentPosted/
ReplyReceived/LeadResponded`); subscriber tương lai (Campaign/Analytics/KPI/Notifications/AI Agent); nền
`internal/events` + coordination runtime_events. **IMMUTABLE, APPEND-ONLY.** Decouple qua events, không gọi chéo domain.

## RBAC & Security (Ưu tiên #1)
- Vá `getAccounts` (`internal/server/workspace/handlers.go`) — lọc theo role.
- `createLocalConnectorPairingCode` (`internal/server/agent/local_connector.go`) ownership check.
- ExecutionContext PUT: member chỉ set default = account của mình. `created_by` → audit per-member.

## Comment delivery — Option C (Track ĐỘC LẬP)
> **Ownership Architecture Track ≠ Comment Delivery Track.** Plan ownership (PR0–PR7) cho biết chính xác
> account/connector/member nào chạy, NHƯNG comment vẫn có thể fail vì FB redirect — vấn đề RIÊNG.

`TargetLocator → CommentExecutor → DeliveryVerifier`; reuse focused visible tab → navigate permalink →
comment → verify. Feed rediscovery không phải default. (PR8.)

## Daily limit (giữ nguyên)
Reserve + Refund trên `comments_today` = business quota. Risk/cooldown/anti-spam ở `risk_score`/
`cooldown_until`/circuit-breaker. KHÔNG thêm `comment_attempts_today`.

---

## Rollout order
0. **PR0 — RBAC/Security**: getAccounts filter + pairing ownership.
1. **PR1 — Multi-account identity uniqueness**: dedup + `UNIQUE(org_id, fb_user_id)`. Trước PR2 (race-safe).
2. **PR2 — Account ownership**: ResolveOrCreate/rebind/no-steal/auto-create + audit.
3. **PR3 — Execution ownership**: `created_by` ×3 + populate + projection switch.
4. **PR4 — Deterministic ExecutionContext**: `ActionContext` + `user_execution_context` + resolver + UI.
5. **PR5 — Attribution**: Interaction-Event derive + champion/leaderboard.
6. **PR6 — Coordination observability**: derived view + typed reason codes; không cross-member block.
7. **PR7 — Campaign-ready abstractions**: chốt interfaces.
8. **PR8 — Comment delivery Option C** (track riêng).

## Verification
```powershell
go build ./...; go vet ./...; go test ./internal/store/... ./cmd/scraper/... ./internal/server/...
npm --prefix frontend run build
```
Kịch bản 2-member/nhiều-FB: account auto theo identity, không ghi đè; Default Account deterministic;
`connector_offline`/`ownership_conflict`/`execution_context_required` rõ ràng; amplification cho phép;
champion derived không ảnh hưởng routing; reassign account không đổi attribution lịch sử (created_by bất biến).
