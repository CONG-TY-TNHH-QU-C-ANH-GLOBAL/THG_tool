# SaaS UX Hardening Track — Review + Fix Plan

Status: DESIGN APPROVED (founder decisions applied 2026-06-12: explicit invite
acceptance flow with in-app notifications; clickable-link policy flag removed —
canonical URL always used when website configured; PR order re-prioritized)
Scope: customer-facing product issues across invite/membership, RBAC, extension
version gating, contact identity, canonical link output, catalog policy gate,
notifications.
Out of scope (explicit): Taobao/1688, Vision fallback, Telegram /comment execution,
Facebook composer hot path (except safe assignment gating / payload input),
AI provider BYOK implementation (roadmap only, §8).

This document is the authoritative plan. Each PR below is a separate, independently
shippable change, sliced by architecture track (track separation rule). No PR mixes
tracks. Every PR: no new source file > 200 lines, tenant isolation preserved.

---

## 0. Audit Summary (verified against HEAD)

| # | Area | Verdict | Root cause / gap |
|---|------|---------|------------------|
| 1 | Invite → workspace visibility | BROKEN UX | JWT embeds `org_id`+`role` statically (`internal/auth/auth.go:28-35`); accept handler issues a fresh JWT (`internal/server/auth/onboarding.go:499-500`) but the app never re-bootstraps; stale cookie can win after reload (`authService.ts:74-95`); no membership endpoint; no toast; no tests. |
| 2 | Admin/Sales RBAC for FB accounts | MOSTLY SOUND, 1 CRITICAL LEAK | Ownership model (PR0–PR3 of ORGANIC_SALES_NETWORK) is correct. **CRITICAL: `/admin/accounts` serializes decrypted `CookiesJSON`, `ProxyURL`, `UserAgent`** (`internal/server/org/superadmin.go:12-18`, `models.go:197-199`). Missing ownership check on input commands (`local_connector.go:~120`). No admin pause/resume primitive. UI views missing. |
| 3 | Extension version gating | PARTIAL | Single hardcoded `MinExtensionVersion = "0.5.26"` (`internal/store/connectors/readiness.go:17`). Crawl is gated via `PickReadyConnector`; **comment/post/inbox outbound path has NO version gate**. No policy storage, no 4-state model, no `blocked_by_extension_version` audit. |
| 4 | Contact identity | WORKSPACE-ONLY | Single `BusinessProfile.OfficialContact`; no per-staff contact profile anywhere. `@Moonzzz03` appears only in test fixtures, not prod code. Comment generation cannot vary contact per assigned staff. |
| 5 | Clickable URLs | PARTIAL | Normalize-for-match + screening (≤1 URL, grounded-only) + repair exist (`comment_contacts.go`, `comment_quality.go:76-116`). Missing: canonical output normalization, spaced-domain obfuscation detection. (Product decision: NO link policy flag — website configured ⇒ always canonical clickable URL; website empty ⇒ no website mention.) |
| 6 | Catalog grounding | STRONG CORE, NO POLICY GATE | Retrieval (hybrid pgvector/keyword, K=6) + `GroundSelection` (invented items dropped, role-guarded) + confidence recalibration + `knowledge_gap` all live with tests (`comment_decision.go`). Missing: P2d Policy Gate, `allow_price_in_comments`, `allow_product_link_in_comments`, `min_confidence_to_quote_price` flags. |
| 7 | Notifications | TELEGRAM MATURE, NO IN-APP | Telegram event pipeline + delivery tracking + audit live; `EventTypes` allow-list exists but invite/extension events not wired. SMTP invite email works. No persistent in-app notifications. Rate-limit pattern exists (`system/notifications.go:15-50`). |

Security review highlights (must fix first):
- **S1 (CRITICAL)**: decrypted FB cookies/proxy/UA exposed over HTTP in `/admin/accounts` (superadmin). Any handler marshalling `models.Account` leaks. Fix = redaction projection (PR-2).
- **S2 (MEDIUM)**: sales user can queue connector input commands against another staff's account (org check passes, ownership check missing) — `local_connector.go` input command handler.
- **S3 (MEDIUM)**: middleware trusts JWT `org_id` for its full TTL; membership change does not take effect until token refresh (15 min) or logout. Compounded with single-org "move" semantics this enables acting in the *old* org post-accept.

---

## PR-1 — Invite acceptance flow + in-app notifications + membership refresh (Track: Platform/Tenant)

Behavior-changing. Backend + frontend. **Invite acceptance is an explicit user
action on the website — never passive.** A user must never need logout/login to
see a workspace they were invited to.

Flow (product-decided):
1. Admin invites staff from Settings → Nhân viên.
2. Invitee is notified: email invite (if SMTP configured) + in-app notification
   (if the email already belongs to an account) + visible pending invite after login.
3. Invitee explicitly clicks «Đồng ý tham gia workspace».
4. On accept: backend grants membership → frontend refreshes `/auth/me` +
   memberships → workspace switcher updates immediately → user is routed
   directly to the invited workspace → toast «Bạn đã tham gia workspace <name>.»
5. If the invited email has no account, the email link routes through
   signup/login first, then lands on the accept screen.

Backend:
1. `GET /api/auth/me` already returns DB-fresh `org_id`/`role` — keep as source of truth.
2. Add `POST /api/auth/refresh-membership`: re-reads `users.org_id`/`role` from DB,
   issues fresh access token + cookies (reuse `GenerateAccessToken` + `setAuthCookies`
   from `onboarding.go:499-500`). Idempotent, deterministic.
3. Add `GET /api/user/memberships` → `[{ org_id, org_name, role, joined_at }]`.
   **Constraint documented: the model is single-org-per-user today** (`users.org_id`;
   accepting an invite MOVES the user's org via `UpdateUserOrg`). The endpoint
   returns one row now but its contract is a list — future-proofs multi-membership
   without overbuilding (no join table yet).
4. Accept response must return: org id, org name, role, fresh token/session.
   Emit audit `membership_granted` alongside existing `invite_accepted`
   (admin-visible audit event).
5. Invite status: `GET /api/org/invites` exposes derived state
   `pending|accepted|expired|revoked` (derive from `used_at`/`expires_at`; revoked
   currently DELETEs the row — switch to soft `revoked_at` column so admin sees status).
6. **In-app notification substrate** (moved into this PR from PR-8): table
   `notifications` (`id, org_id, user_id NULL=org-wide, type, title, body,
   payload_json, read, created_at`) + list/mark-read endpoints. Notification
   types this phase: `workspace_invite_received`, `workspace_invite_accepted`,
   `extension_update_required` (emitter for the last one lands in PR-4).
   On invite create: if invited email matches an existing user, write a
   `workspace_invite_received` notification for that user. On accept: write
   `workspace_invite_accepted` for the inviting admin.
7. Do not rely on stale JWT claims for workspace state anywhere — workspace
   list/context derives only from `/auth/me` + `/api/user/memberships`.

Frontend:
8. Notification bell + panel (feature folder `components/notifications/`,
   each file < 200 lines): unread badge, list, mark-read.
9. Invite card (in notification panel and on login landing when a pending
   invite exists):
   - Title: «Bạn được mời tham gia workspace»
   - Body: «<admin_name> đã mời bạn tham gia workspace <workspace_name> với vai trò <role>.»
   - Primary CTA: «Đồng ý tham gia» / Secondary CTA: «Từ chối» or «Để sau»
10. After accept (`JoinWorkspace.tsx:85-97`, `CreateFacebookWorkspace.tsx:44-58`,
    invite card): `setAuth(newToken, user)` → `await hydrate()` (re-run `/auth/me`)
    → refetch memberships → invalidate workspace-derived state → route directly
    to the invited workspace → toast «Bạn đã tham gia workspace <name>.»
11. New-user path: email link → signup/login → accept screen (same card).

Tests (Go handler tests + FE unit):
- invited existing user sees in-app `workspace_invite_received` notification
- invited new user can signup/login then accept via the same link
- accept → `/auth/me` + memberships immediately reflect new org/role (no logout)
- accept routes to the invited workspace; switcher updates
- role applied correctly (sales vs admin) after accept
- admin sees `accepted` status + `membership_granted` audit event
- expired invite → not acceptable; revoked invite hidden + not acceptable
- user cannot accept an invite addressed to a different email
- no cross-tenant leakage: memberships + notifications never return another
  org's/user's rows
- stale JWT does not keep old workspace state after accept (request with
  pre-accept token to org-scoped endpoint behaves safely — see PR-2 item 3)

## PR-2 — RBAC hardening + secret redaction (Track: Platform/Tenant)

Behavior-changing (security). Backend only. **Ship first or with PR-1.**

1. **Redaction projection** `internal/models/account_safe.go` (`AccountSafe` — no
   `CookiesJSON`/`ProxyURL`/`UserAgent`). Convert every HTTP handler that returns
   `models.Account` (start: `superadmin.go:12-18`, any list using
   `GetAllAccounts`/`GetAccountsForUser`). Store keeps returning full Account for
   internal workers — domain contract, not ORM row (per contracts-not-orm rule).
2. Ownership check in `createConnectorInputCommand` (`local_connector.go`):
   `models.IsAccountOwnerAllowed(acc, userID, role)` → explicit 403
   "you do not own this account".
3. Stale-JWT mitigation: on org-scoped *mutating* endpoints, validate JWT `org_id`
   against `users.org_id` (one indexed lookup) OR shorten the window by having the
   accept handler revoke outstanding refresh tokens. Decision: DB validation on
   mutations only (cheap, deterministic — branch on explicit field, no proxy state).
4. **Pause/resume primitive**: migration adds `accounts.assignment_paused`
   (INTEGER 0/1) + `PUT /api/admin/accounts/:id/pause|resume` (admin-only).
   `DecideCaps` (`behaviour_caps_check.go`) gains gate #0:
   `assignment_paused → reason "assignment_paused_by_admin"`. This is the safe
   assignment gating allowed to touch near the hot path — it is a pre-queue check,
   not a composer change.
5. Move connector list ownership filter from handler loop
   (`local_connector.go:62-74`) to store: `ListConnectorsForUser(orgID, userID)`.

Tests: redaction (response JSON contains no `cookies_json`/`proxy_url` keys),
input-command 403 for non-owner, pause blocks queueing with typed reason,
resume restores, sales cannot call pause endpoints, tenant isolation on all new endpoints.

## PR-3 — Admin connector table + staff "My Facebook connection" (Track: Platform/Tenant, FE)

Refactor-light, additive UI. Depends on PR-2 (safe projections).

Admin (Settings → Nhân viên — option A, expandable staff row; reuses existing
`/staff`, `/connectors/status`, `/accounts/readiness` data):
- columns: staff name/email/role, FB display name, connector online/offline,
  last_seen, extension version + version status, readiness, automation eligibility,
  comments/posts/inbox counts (from existing staff convs/cmts fields)
- actions: view details, pause/resume assignment (PR-2 endpoints), copy update instructions
- admin NEVER sees: pairing secrets, cookies, tokens, session data (PR-2 guarantees)

Staff ("My Facebook connection" panel, promoted from existing private connector list):
- FB display name, connector online/offline, current vs latest extension version,
  version status, last_seen, readiness, pair/reconnect instructions, update instructions
- `CanViewAccountDevice` already hides other members' devices — reuse, don't re-derive.

New components under `frontend/src/modules/autoflow/components/connectorAdmin/`
(feature folder: components/hooks/services/types), each file < 200 lines.

## PR-4 — Extension version policy + outbound gating (Track: Facebook Automation Reliability)

Behavior-changing. Backend + extension heartbeat + FE messages.

1. Migration `extension_policies` (platform-scoped row, org override optional):
   `latest_version, min_supported_version, min_required_version, release_channel,
   release_notes, update_url, update_instructions, force_update_after NULL`.
   Fallback to current constant if no row.
2. Version state evaluator (pure function, `internal/store/connectors/version_policy.go`):
   `EvaluateVersionState(reported, policy) → latest | update_available |
   update_required | unsupported` using existing `versionAtLeast`
   (`readiness.go:76-90`). latest/update_available ⇒ allowed;
   update_required/unsupported ⇒ blocked.
3. Heartbeat enrichment (`api.js stateBody`, `heartbeat.go`): add
   `build_number`, `release_channel`; keep `extension_version`; `last_seen`
   already updated per heartbeat.
4. **Gate the outbound path**: insert version check into the pre-queue readiness
   chain — extend the `BehaviourCheck` hook adapter (`internal/store/outbound_aliases.go:95-116`)
   or call `PickReadyConnector` in `queueLeadOutreach` (`cmd/scraper/outbound_actions.go:83`).
   Chosen: explicit `PickReadyConnector` call pre-queue (deterministic, mirrors
   crawl's `EvaluateCrawlAccountReadiness`). Composer hot path untouched — this is
   assignment gating only.
5. Typed reasons: add `extension_update_required`, `extension_unsupported`
   (alongside existing `extension_version_outdated`, which maps to update_required
   for back-compat). Record pre-queue blocks in `action_ledger` with
   `outcome='skipped', reason='blocked_by_extension_version'` via existing
   `RecordLedgerTx` — no silent failure.
6. FE messages (`reasonMessages.ts`):
   - update_available (soft): «Có bản cập nhật extension mới. Bạn vẫn có thể dùng, nhưng nên cập nhật để ổn định hơn.»
   - update_required: «Automation đang tạm dừng vì extension của bạn đã cũ. Cập nhật extension để tiếp tục nhận task.»
   - unsupported: «Phiên bản extension này không còn được hỗ trợ. Vui lòng cài phiên bản mới.»
   - admin view: "Automation paused until staff updates extension."
7. Assignment preflight order (all must pass before any FB task assignment):
   connector online → account ready → not paused/cooldown/checkpoint →
   version not update_required/unsupported → admin assignment not paused (PR-2).
8. On transition into update_required/unsupported: write an
   `extension_update_required` in-app notification (substrate from PR-1) for the
   connector owner + an org-wide admin one. Telegram alerting for the same event
   is PR-8 (rate-limited there).

Tests: state evaluator table-driven (all 4 states + unknown version ⇒ unsupported),
outbound queue blocked + ledger row written, crawl parity, soft state still allowed,
policy fallback when table empty.

## PR-5 — Staff contact profiles (Track: Comment Intelligence)

Behavior-changing. Backend + FE + generation wiring.

1. Migration `staff_contact_profiles`: `user_id PK, org_id, display_name,
   role_title, telegram, zalo, phone, email, preferred_cta, signature_text,
   visibility, active, updated_at`. org_id ownership-checked everywhere.
2. Company identity (existing `org:{id}:*` keys) stays brand/service truth;
   `official_contact` becomes documented **fallback only**.
3. Contact resolution (pure function `internal/ai/contact_resolution.go`):
   `ResolveContactIdentity(staffProfile, companyIdentity, policy)` →
   assigned staff contact if present+active; else company default **iff**
   `org:{id}:allow_company_contact_fallback` (default true); else omit contact
   line entirely (degrade honestly — never invent).
4. Wire into generation: the queue path already knows the executing account →
   `Account.AssignedUserID` → staff profile. Pass resolved contact into
   `CompanyIdentity.OfficialContact` slot consumed by `buildContactRule` /
   `buildCompanyBlock` (`comment_decision.go:358-388`) so screening/repair
   (`ScreenCommentContacts`) automatically grounds against the *per-staff* contact.
   No prompt-code hardcoding.
5. FE: staff edits own profile (Settings → My profile); admin can view/manage all
   profiles in workspace (role-gated).

Tests: two staff ⇒ two different contact lines; missing contact + fallback=true ⇒
company contact; fallback=false ⇒ no contact line; inactive profile ⇒ fallback;
screening rejects contacts not grounded in resolved identity; grep-gate: no
hardcoded handles in non-test prompt code.

## PR-6 — Canonical URL normalization + malformed link prevention (Track: Comment Intelligence)

Behavior-changing, small. **Product decision: NO link policy flag.** No
`allow_clickable_links_in_comments`, no per-mission/per-account link policy,
no "no-link fallback" copy. The rule is simple and unconditional:
website configured ⇒ canonical clickable URL; website empty ⇒ no website mention.

1. Canonical normalizer (`internal/ai/url_normalize.go`, pure):
   `CanonicalURL(raw, canonicalHost)` → always `https://` + configured canonical
   host. Examples (canonical host = `www.thgfulfill.com`):
   - `thgfulfill.com/vi` → `https://www.thgfulfill.com/vi`
   - `www.thgfulfill.com/vi` → `https://www.thgfulfill.com/vi`
   - `https://thgfulfill.com/vi` → `https://www.thgfulfill.com/vi`
   Single helper; the match-normalizer call sites
   (`comment_contacts.go:118-124`, `company_identity.go:33-39`) keep their
   compare-only role (DRY: extract shared strip logic).
2. Generated comments use the normalized canonical URL exactly as resolved from
   company identity — normalize at identity load/save AND at repair output, so
   the screening allowlist and emitted text always agree.
3. Obfuscation guard: extend screening to detect spaced/broken domains
   (`thgfulfill. com`, `thgfulfill com`, partial non-clickable domains) —
   repair to canonical, or reject if repair is unsafe.
4. Empty website field ⇒ no website mention ever (already enforced via
   `buildCompanyBlock`; add explicit test).

Tests:
- website normalizes to canonical clickable URL (all three input shapes above)
- no spaced-domain output; no malformed/partial URL output
- empty website ⇒ no website mention
- generated comment uses the official website exactly as the normalized
  company identity value

## PR-7 — Catalog policy gate (P2d) (Track: Comment Intelligence)

Behavior-changing. Implements the missing gate over the existing grounding core.

1. Policy flags (org-scoped `user_context` keys, loaded via new
   `ai.LoadOrgCommentPolicies`): `allow_price_in_comments` (default true),
   `allow_product_link_in_comments` (default false),
   `min_confidence_to_quote_price` (default 0.7),
   `fallback_when_no_catalog_match` (`generic_service_comment` default).
2. `internal/ai/policy_gate.go` (pure): `EvaluateGate(decision, policy) → GateVerdict`
   - confidence ≥ threshold + policy allows ⇒ product mention + price allowed
   - medium confidence ⇒ category/service mention, no exact price, inbox-for-quote CTA
   - `knowledge_gap` / low confidence ⇒ generic service comment (no product, no price)
   - product URL only if `allow_product_link_in_comments` ⇒ canonical-normalized (PR-6)
3. Gate verdict shapes the generation prompt (which grounded fields are exposed)
   AND post-screens output (price text present only when allowed) before
   `QueueOutboundForOrg` in `outbound_actions.go`. Retrieval already returns
   ranked candidates with price/SKU/score/source — no retrieval change needed;
   surface `source connector name` in the decision payload for the Inspector.
4. Stale/unhealthy catalog sources excluded at retrieval (respect existing source
   health flags; add filter if absent).

Tests: connected-but-no-match ⇒ no product/price hallucination (extends existing
`TestBuildDecision_NoOfferZeroConfidence`); high conf + allow ⇒ name+price;
high conf + price policy false ⇒ name only; URL policy on/off; multi-catalog
source attribution; unhealthy source excluded.

Note: `outbound_actions.go` (838 lines) and `msggen.go` (726) are legacy god files —
all new logic lands in new small files; the touch points are call-site insertions
only. Future extraction noted per guardrails.

## PR-8 — Telegram + email notification refinements (Track: Omnichannel/Telegram)

Behavior-changing, additive. The in-app notification substrate ships in PR-1;
this PR extends the same events to Telegram and email.

1. Telegram: add `invite_created`, `invite_accepted`, `extension_update_required`
   to `EventTypes` allow-list (`telegram/control/policy.go:104-113`); emit from
   invite create/accept handlers and from the version-state evaluator transition
   (PR-4). Renderers in `telegram/render`
   (e.g. «Một connector cần cập nhật extension trước khi tiếp tục automation.»).
   Rate-limit extension alerts with the crawl-progress throttle pattern
   (`system/notifications.go:15-50`): max 1 per account per 24h. Never per-heartbeat.
2. Email: `SendInviteAccepted` (mailer) → inviter/admin on accept. Invite email
   already exists.
3. Admin invite list shows pending/accepted/expired/revoked (PR-1 item 5 supplies states).

Tests: event emission on create/accept, rate-limit (second alert within 24h
suppressed), org-scoped destination isolation.

---

## 8. Roadmap only — AI provider keys + billing (NOT in any PR above)

Recommendation: **hybrid, managed-first.**

- **Default: THG-managed AI credits bundled in plans.** Best UX for SMB (target
  segment), no API-key literacy needed, THG controls model choice/quality/cost
  optimization (caching, batching, model routing). Charge subscription + metered
  credits. Cons: THG carries margin risk on provider pricing; needs per-org
  usage metering + hard caps to avoid one tenant draining the founder key.
- **Optional: BYOK for advanced/enterprise workspaces.** User pays provider
  directly; THG charges platform fee. Pros: removes THG cost exposure, satisfies
  data-governance buyers. Cons: support burden (key invalid/quota errors become
  THG tickets), uneven quality across providers.
- **Pricing dimensions** (meter from day one even before billing ships):
  connected FB accounts, missions, crawl volume, qualified leads, automation
  actions (comment/post/inbox), AI credits/tokens, Telegram integrations, seats.
- **Credential security model (when built):** per-org provider credentials
  encrypted at rest with the existing ENCRYPTION_KEY pattern (as
  `telegram_bot_credentials` does), never returned by any API after write
  (write-only + "configured ✓" status), masked in logs, scoped usage ledger per
  org. Founder key lives only in server env, never in DB, and is the global
  fallback — per-org metering + caps protect it until managed billing exists.
- **Do NOT build yet:** provider settings UI, billing engine, credit purchase,
  model picker. **Do now (cheap):** per-org token usage counter on every LLM call
  (one table + one increment) so future billing has history.

---

## 9. Acceptance criteria mapping (answered by the PRs)

- Invite: PR-1 (explicit «Đồng ý tham gia» acceptance; in-app notification bell +
  invite card; fresh memberships via `/auth/me` + `/api/user/memberships` +
  refresh-membership + re-hydrate; route to invited workspace; audit
  `invite_accepted`+`membership_granted`; invite states visible to admin;
  no logout/login ever required).
- RBAC: PR-2/PR-3 (admin sees operational status only via `AccountSafe`; sales
  sees only own connector via `CanViewAccountDevice`/store filter; admin cannot
  see cookies/tokens — redaction enforced + tested).
- Connector/version: PR-4 (policy in `extension_policies`; heartbeat reports
  version/build/channel; latest+update_available allow, update_required+unsupported
  block; ledger `skipped/blocked_by_extension_version`; in-app warning via PR-1
  substrate).
- Contact identity: PR-5 (company identity ⊥ staff contact; resolution order
  staff → company fallback → omit; per-staff tests).
- Links: PR-6 (central canonical normalizer; website configured ⇒ always
  canonical clickable URL, website empty ⇒ no mention; spaced/malformed outputs
  rejected. No link policy flag — removed by product decision).
- Catalog: PR-7 (existing ranked retrieval + new policy gate; price only above
  confidence threshold + policy; low confidence ⇒ generic comment; hallucination
  tests extend existing grounding suite).
- Roadmap: §8 (managed credits default, BYOK optional, metering now, billing later).

Shipping order (founder-decided): PR-2 (security hotfix: AccountSafe redaction)
→ PR-1 (invite acceptance + in-app notifications + membership refresh)
→ PR-4 (extension version gating) → PR-3 (admin connector visibility / staff
My Facebook connection) → PR-5 (staff contact profile) → PR-6 (canonical URL
normalization) → PR-7 (catalog policy gate) → PR-8 (Telegram/email notification
refinements). Invite UX is prioritized early because it directly affects
customer onboarding.
