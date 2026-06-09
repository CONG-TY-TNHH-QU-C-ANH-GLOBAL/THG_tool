# Facebook Automation Reliability Track

> **STATUS: ACTIVE (opened 2026-06-08, founder-directed).** A reliability layer
> for ALL Facebook automation (crawl, comment, inbox, posting, future campaign).
> NOT mixed with the Comment Intelligence / P2c / media track. Small PRs, each
> with tests + green build. No big bang.

## Foundational invariant

**No action runs unless the system knows EXACTLY:**
1. Who initiated it.
2. Which account was chosen to run it.
3. Which connector will run it.
4. The ACTUAL Facebook identity currently logged in.
5. Whether that account/connector is eligible for that action.

If any is missing → **fail early, typed reason code, actionable message, NO silent
fallback to another account, NEVER auto-pick "first ready account."**

## Confirmed bug that motivated the track (CAUSE A)
`org_crawl_intents` 7 & 9 have `account_id=0` (user created a mission without
choosing an account) → backend fell back to `pickReadyFacebookAccountIDForCrawl`
→ only connector online was account #50 → every account-less mission piled onto
#50; missions for #40/#29 failed (extension offline). This is a DESIGN flaw
(allowing account-less missions + silent fallback), not a dispatch race. The
earlier "missing account_id filter on the command claim" diagnosis was WRONG —
`enqueueConnectorCrawlCommand` already targets a specific per-account `agent_id`
([crawl_runtime.go:290](../cmd/scraper/crawl_runtime.go#L290)); `agent_id` is the
partition key.

---

## PRs (ordered — foundation first)

### PR-A — Mission Preflight / crawl bug fix (FIRST)
**Backend:** reject `account_id=0` for user-created crawl missions; remove/disable
the `pickReadyFacebookAccountIDForCrawl` fallback for user-created missions;
validate at create time (owned by org/user · connector online · connector logged
into the right `fb_user_id` · extension new enough · not `actor_mismatch_blocked`);
do not create on fail; typed reason codes:
`account_not_selected · account_not_owned · connector_offline ·
actor_identity_unknown · actor_mismatch_blocked · extension_version_outdated`.
Actionable messages.
**Frontend:** `CreateMissionForm` requires an account (no submit when empty;
preselect when exactly one ready; always send `account_id`); show per-account
status (ready / connector offline / identity unknown / actor blocked / extension
outdated).
**Tests:** account_id=0 rejected · offline rejected · not-owned rejected · ready
succeeds · no fallback-to-first-ready path remains for user-created missions.

### PR-B — Facebook Identity Accuracy Hardening
`fb_user_id`/`c_user` = source of truth; `fb_display_name` = display metadata only,
never used to decide identity. Heartbeat sends `fb_user_id, fb_display_name,
fb_profile_url, identity_confidence, identity_extraction_method,
identity_last_verified_at`. "Cover photo" hardening: ignore UI labels (Cover
photo / Profile picture / Ảnh bìa / Ảnh đại diện / See more / Menu / …) as the FB
name. Prefer: (1) `c_user` cookie for `fb_user_id`, (2) profile/canonical URL,
(3) verified DOM selector. Display-name-only (no `c_user`) → `identity_unknown`.
Backend: account ready ONLY with a valid `fb_user_id`. Verified-Actor gate
unchanged (expected `accounts.fb_user_id` vs live `c_user`; mismatch blocks the
account + notifies owner; unknown never auto-executes; verified does not
auto-clear an operator block). Tests cover all.

**PR-B part 1 SHIPPED (ext 0.5.30):** B2 — `content/proof.js currentFBUserID`
reads the `c_user` cookie FIRST (same source as the heartbeat → Verified-Actor
compares apples-to-apples), HTML `USER_ID`/`data-userid` as fallback. B1 —
`content/meta.js` rejects UI-affordance labels (Cover photo / Ảnh bìa / Profile
picture / See more / Menu / …, diacritic-insensitive) and keeps scanning for a
real name. Golden smoke `local-connector-extension/test/identity.test.js`.
**PR-B part 2 (NEXT):** B3 heartbeat metadata `identity_confidence /
identity_extraction_method / identity_last_verified_at` (src/facebook-state.js +
src/api.js + backend + migration); B4 confirm backend treats missing `fb_user_id`
as identity_unknown (mostly via `connectors.PickReadyConnector` ConnIdentityUnknown).

### PR-C — Connector Registry / multi-account per device
Model: one device runs MANY Facebook accounts — each Chrome PROFILE is its OWN
connector. No "1 machine = 1 account." Connector metadata: `connector_id,
machine_id, browser_profile_id|profile_label, account_id, fb_user_id,
extension_version, last_seen_at, capabilities`. Many connectors may share a
`machine_id`. Each connector binds one account by FB identity. UI shows N
connectors on one machine. Audit pairing: two profiles never bind the same
account; rebind only by `fb_user_id`; no-steal preserved; no cross-member theft.

### PR-D — Readiness Matrix ✅ SHIPPED (66a0a1d + hardening)
Read-only projection/API `GET /api/accounts/readiness`: per account, per capability
(`crawl/comment/inbox/post`) `{can, reasons[]}` + `connector_id, extension_version,
required_action`. **Canonical typed reasons** (one name each — UI must map to these
EXACT strings): `connector_offline · actor_identity_unknown · actor_mismatch_blocked
· extension_version_outdated · account_cooldown_active · risk_ceiling_exceeded ·
daily_limit_exceeded`. (NOTE: it is `account_cooldown_active`, NOT `cooldown_active`
— the existing gate code + this matrix share the one name; do not introduce a
second.) Implementation hardening (PR-D.1):
- **Shared truth:** `coordination.DecideCaps(now, …)` is PURE (clock injected, no
  flake) and is the single cap decision used by the queue gate (`CheckCapsTx`, with
  decay) and the read-only matrix (`EvaluateCaps`, no decay).
- **crawl policy:** crawl is read-only → outbound pacing (cooldown/risk/daily) does
  NOT apply, but the hard `actor_mismatch_blocked` (denies ALL execution) DOES.
- **post mapping:** `post` → `group_post` cap (the live action). `profile_post` is a
  scaffold; a separate capability is deferred until it ships (documented so the
  group_post daily cap is never silently missed).
- **RBAC:** scoped via `GetAllAccounts` + `models.CanViewAccountDevice` — a member
  sees only their own accounts; an admin also sees unassigned org accounts; no other
  member's `fb_user_id` leaks (same privacy rule as the connector status board).
Every mission/action UI consumes this instead of guessing. (`missing_default_account
/ not_owned` are create-time concerns handled by the mission preflight, not the
matrix.)

### PR-E — Account Health Board
Actionable table (replaces the screenshot stream as the customer's main
understanding surface): Account · FB Identity · Connector · Chrome Profile ·
Status · Last Seen · Capabilities · Block Reason · Action. Actions: reconnect
guide · clear actor block (admin) · set default account · view last failure
evidence · extension version · offline duration.
**MUST consume `GET /api/accounts/readiness` (PR-D) as the source of per-capability
status + typed reasons + required_action — do NOT re-derive readiness from the old
`stream_status`/`online` flags in the FE.** Group rows by `machine_label` (PR-C).

### PR-F — Stream cleanup
Remove the continuous screenshot stream from the customer's primary UX ONLY AFTER
PR-D/PR-E ship. Keep evidence-on-FAILURE (screenshot, final_url, fb_user_id,
connector_id, account_id, extension_version, phase, reason_code, nav_events).
Audit `LocalChromeViewer`/remote-control deps first; if still used for debug,
keep behind a flag / superadmin-only.

---

## Priority: A → B → C → D → E → F.

## Definition of Done
No silent `account_id=0` fallback · user can't accidentally run the wrong account
· system never auto-picks the first online account for the user · FB identity by
`fb_user_id`, no "Cover photo" · one machine / many Chrome profiles / many FB
accounts modeled clearly · UI shows which account is ready / broken + what to do ·
stream is no longer the primary understanding tool · every failure carries a typed
reason code + operator action · the SaaS has a clean reliability floor to scale.
