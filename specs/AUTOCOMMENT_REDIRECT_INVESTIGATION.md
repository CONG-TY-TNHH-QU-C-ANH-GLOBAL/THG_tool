# Auto-Comment `redirected_feed` Investigation

**Status**: 2026-06-02 — **Real root cause of "auto-comment posts nothing" was NOT navigation/redirect — it was AI text generation failing.** Operator clarified: comments route to the correct post fine, but the AI never composes the comment text. Found the deterministic bug: `OPENAI_COMMENT_MODEL` defaults to `gpt-5.4` (a reasoning model), and `MessageGenerator.callOpenAI` sent `temperature: 0.7`, which gpt-5*/o-series reject with HTTP 400 → every comment/inbox/post generation returned `generation_failed` and the lead was skipped with empty content. Classification kept working because `UniversalClassify` uses the no-temperature `callOpenAIStrictJSON` path. Fix (commit pending): `callOpenAI` now builds the body via `chatCompletionBody(model, prompt)` — omits `temperature` for reasoning models, raises `max_completion_tokens` 400 → 2000, and errors on empty content instead of silently returning "". Unit test `msggen_model_test.go` guards it. The navigation/redirect work below remains a SEPARATE, still-open concern for the rung2 execution path (the `no_terminal` symptom) — left untouched by this fix; revisit only if a comment with real text reaches the post and still fails to type.

**Prior status**: 2026-06-01 (afternoon) — Crawler-pattern nav (a2d022c) verified post DID load briefly but FB silently redirects between navigateAndVerify success and content-script handler entry. Switched to **Path 2: comment in group feed** — open `/groups/<g>/` (proven non-redirected), locate target article in feed DOM by post_id, comment inline. Never navigate `/posts/`. Manifest 0.5.8.1000. Awaiting verification cycle.

**Owner**: Operator (founder). Claude cannot satisfy the verification gate from current context.

**Date opened**: 2026-05-21.

---

## Symptom (verified)

When the operator runs `comment_all_leads` in their workspace's copilot:

```
✅ comment_all_leads → queued_comment=1 skipped=9 mode=approved_auto
                       reasons=map[missing_post_permalink:9]
SYSTEM ▸ [Trợ lý THG] Facebook comment #53 trạng thái:
        finished/context_drift. Đối tượng: Anonymous participant.
        Chi tiet: redirected_feed
```

Operator confirmed via direct browser inspection:
- All lead URLs open to the correct FB post when navigated manually (Test A baseline).
- Comments do NOT appear on the target posts (Test A negative).
- Account #49 IS a verified member of the affected group `1312868109620530` (Test B).
- Extension visibly never types or submits during the automation flow.

---

## Definitively fixed in this session

### 1. URL routing bug — `missing_post_permalink` for photo-format leads

- **Commits**: `1083e4c` (pre-session), `47b669c` (this session)
- **Mechanism**: Extension crawler's `postPermalink()` Tier 3 regex `[?&]fbid=\d+` was matching photo viewer anchors (`/photo/?fbid=PHOTO_FBID`) before the legitimate `/posts/<p>/` Tier 4 anchor. Photo URLs went into `lead.source_url`, but FB's photo viewer fbid is the photo's id, not the parent post's. Server's `isCommentableFacebookPostURL` correctly rejects `/photo/?fbid=` (only accepts the legacy `/photo.php?fbid=`), producing the `missing_post_permalink` skip.
- **Fix**: Three changes in `crawl.js` — `postPermalink::isCandidate` excludes `/photo/` and `/photo.php`; `extractPostFBID` skips `?fbid=` extraction on photo paths; new clause extracts the real parent post fbid from `set=gm.<post_fbid>` and the group fbid from `idorvanity=<group_fbid>`. Photo-anchor-only articles now still surface a real canonical post URL.
- **Verification path**: re-crawl, query `leads.source_url` — should show `/groups/<g>/posts/<p>/` or `/groups/<g>/permalink/<p>/`, never `/photo/`.

### 2. Sales role missing browser tab

- **Commit**: `e2243eb`
- **Mechanism**: `FacebookWorkspaceApp::STAFF_TABS` did not include `browser`, so sales-role users could not initialize their own browser workspace despite owning their own FB accounts (per `AccountOwnerAllowed`).
- **Fix**: Added `browser` to `STAFF_TABS`. Backend ownership checks already enforce that sales users can only action their own accounts. Per `feedback_shared_battlefield_not_crm`: sales sees all accounts as battlefield context, actions only their own.

---

## 2026-06-01 (afternoon) — Path 2: comment in group feed (no permalink nav at all)

The crawler-pattern fix (a2d022c) addressed the chrome.tabs.update→create distinction but failed in a NEW way:

```
outbox#78  redirected_feed
  notes: identity_gate_1_no_article_or_unstable
         target id=2031780854395915
         landed_at=https://www.facebook.com/
         nav_at_entry=https://www.facebook.com/
         did not settle within 8s
```

Operator observed the tab DID load the post briefly (visual confirmation), then content script's `nav_at_entry` reported `/`. Sequence:

```
T=0    chrome.tabs.create(post_url)
T=1-3s post page renders, operator sees it visually
T=5s   navigateAndVerify URL check → matches post → OK
T=5.x  chrome.tabs.sendMessage fires
T=5.x  FB silent-redirects tab to /
T=5.x  content-script handler entry: location.href = /
T=13.x gate-1 polls 8s on / → fails → emits redirected_feed
```

Operator confirmed account #49 has NO Facebook-side restriction (manual commenting works normally), which rules out H2 (account shadow). H1 manifests differently than expected — FB allows the navigation but kicks the tab back to `/` during the ~hundreds-of-ms window between URL verify and content-script handler entry.

**Path 2 hypothesis**: FB's anti-bot heuristic detects content-script execution at the permalink page surface specifically. Group home (`/groups/<g>/`) has been navigated reliably by the crawler for months with content-script always injected — that surface does NOT trigger the redirect. By finding the target article IN the feed DOM (the article body IS rendered on group home for fresh posts) and commenting from there, we avoid the permalink surface entirely.

**Shipped (commit pending)**:

- [local-connector-extension/content/outbound.js](../local-connector-extension/content/outbound.js) — added `executeCommentInFeed(message)` as sibling to existing `executeComment`. Reuses ALL existing utilities (`waitUntilTargetArticleStable`, `findTargetArticle`, `extractArticleCanonicalEntityId`, comment-button finder, `findCommentEditor`, gate-2 + gate-3 identity checks, `setEditableText`, `findSubmitButtons`, `commentResult`). Only new behaviour: scroll-then-search loop (up to 8 scrolls × 2.5s waitUntilTargetArticleStable each = ~25s max) to handle articles below the initial fold.
- [local-connector-extension/content/bridge.js](../local-connector-extension/content/bridge.js) — added `thg_comment_in_group_feed` to MUTATING_COMMAND_TYPES + onMessage routing.
- [local-connector-extension/src/outbox.js](../local-connector-extension/src/outbox.js) — added `extractGroupHomeFromPostUrl`, `extractPostIdFromTargetUrl`, `executeInGroupFeed(message, targetUrl, groupHome)`. Branch in `executeInFacebookTab`: comments to `/groups/<g>/posts/<p>/` route through Path 2; non-group surfaces (/watch, /reel, profile posts, fb.watch, photo permalinks) keep the direct-nav crawler pattern.
- [local-connector-extension/manifest.json](../local-connector-extension/manifest.json) — 0.5.7 → 0.5.8.

Diagnostic taxonomy (operator-facing notes prefix):

| Note prefix | Stage | Next action if it appears |
|---|---|---|
| `path2.group_home_nav_failed` | navigateAndVerify to /groups/<g>/ failed 3× | Even crawler-surface restricted. Strong escalation signal — GraphQL becomes next track. |
| `path2.article_not_found_in_feed` | gate-1 timed out across 8 scrolls | Post is too old/deep in feed. Tune scroll budget or add group search-box fallback. |
| `path2.article_found_but_comment_button_missing` | article found but no Comment button in scope | DOM shape changed OR account restricted from commenting on this post specifically |
| `path2.identity_gate_2_post_click_swap` | post-click modal opened on different post | FB's lazy click resolution swap — rare race. |
| `path2.identity_gate_3_editor_drift` | editor's enclosing scope canonical id mismatched at pre-type | Same as gate-3 on permalink path. |
| `path2.article_found_comment_opened_submit_failed` | typed OK, submit click + enter both failed to clear editor | FB blocking submit specifically — strong fingerprint signal. |
| `path2.comment_success` | terminal success | ✅ Close investigation. |

**Validation gate**: If Path 2 still fails consistently with `path2.article_found_but_comment_button_missing` or `path2.article_found_comment_opened_submit_failed`, we have strong evidence FB is detecting content-script execution itself regardless of nav surface → GraphQL API is the next investigation track. If Path 2 hits `comment_success`, the entire redirect-on-permalink class of bugs is sidestepped and we can close the investigation.

## 2026-06-01 — Group-click abandoned; comment outbox now uses crawler's navigation helper (later superseded by Path 2)

Group-click v1 (`f2645e6`) + v2 (`63def8a` wide selector + scroll-then-scan) shipped to bypass H1's deep-link redirect. Both failed in distinct ways:

```
v1: scanned=1 anchors          → narrow selector caught only nav, not post permalinks
v2: scanned=4-8 anchors after 5 scrolls
    post_id=2025115625062438 — never matched any anchor in DOM
```

Modern FB renders group-feed permalink anchors via tracking URLs (`__cft__`, `__tn__`) and defers permalink-href mounting until hover or focus. Selector-based matching is structurally unreliable on this surface.

**User insight that ended the detour**: the crawler navigates FB reliably without redirect. Comparing the two paths surfaced the real difference:

| | Crawler | Comment outbox (old) |
|---|---|---|
| Tab creation | `chrome.tabs.create({url, active:true})` | `chrome.tabs.update(tab.id, {url})` |
| Retry | 3× with close-on-fail | 0 |
| SPA settle | 5000 ms | 1200 ms |

Hypothesis: FB's anti-bot heuristic fingerprints `chrome.tabs.update` of an existing tab as background-nav (bot-like) and redirects to `/`; `chrome.tabs.create` of a fresh tab is fingerprinted as user-clicked link and is allowed through. The crawler's months of reliable navigation are direct evidence.

**Shipped fix (commit pending)**:

- [local-connector-extension/src/commands.js](../local-connector-extension/src/commands.js) — export `navigateAndVerify` from the THGCommands closure (1 line in the return statement).
- [local-connector-extension/src/outbox.js](../local-connector-extension/src/outbox.js) — delete the entire group-click implementation (`extractGroupHomeFromPostUrl`, `findAndClickPostAnchorInPage`, `navigateToPostViaGroupClick`, `urlsMatchSameDestination`) and replace `executeInFacebookTab` with a thin wrapper around `THGCommands.navigateAndVerify`. Mirror crawler's cleanup: close the temp tab after the command finishes. Net −240 LOC.
- [local-connector-extension/manifest.json](../local-connector-extension/manifest.json) — 0.5.6 → 0.5.7 so operator knows when reload is required.
- [cmd/scraper/outbound_actions.go](../cmd/scraper/outbound_actions.go) — orthogonal max_items wire so `/comment_all_leads với chỉ 1 lead` honours the count the LLM already extracts.

Verification path: operator reloads extension to 0.5.7, runs `/comment_all_leads với chỉ 1 lead`, observes a NEW tab opening with the post URL (not the existing FB tab navigating). Three outcomes:

| `attempts[0].notes` | Meaning | Next |
|---|---|---|
| empty + `outcome=dom_verified` | ✅ Fix works | Close investigation |
| `outbox.crawler_nav_failed: navigate verify failed: expected=<post>` | All 3 retries hit FB redirect even with `chrome.tabs.create` | Escalate to H2 (account #49 shadow-restricted) or H4 (fingerprint) |
| `identity_gate_1_no_article ... landed_at=<post URL>` (matches target) | Nav succeeded but gate-1 failed on the post page | Tune gate-1 stable-wait / composer detection |

## 2026-05-31 — H1 confirmed; group-click detour shipped (later abandoned)

Operator triggered `comment_all_leads` after the risk-decay circuit-breaker (commit `6f36dad`) unblocked account #49. 10 leads queued, 10 redirected. `execution_attempts.evidence_json.notes` carried:

```
2026-05-31 15:19:50  outbox.navigation_redirected:
                     target_url=.../groups/.../posts/2032847754289225/
                     actual=https://www.facebook.com/

2026-05-31 15:20:32  identity_gate_1: target id=2032616460979021
                     landed_at=https://www.facebook.com/
                     nav_at_entry=https://www.facebook.com/
                     did not settle within 8s
(repeats for 4 more attempts, same pattern)
```

Decision-table match: **H1 confirmed, candidate fixes insufficient.** FB still redirects deep-link nav even with foreground tab + window restore (b93b783) and URL matcher harmonization (8209178).

Shipped fix: `local-connector-extension/src/outbox.js::navigateToPostViaGroupClick`. For `/groups/<g>/posts/<p>/` comment targets the executor now:

1. Navigates to `/groups/<g>/` (group home — FB's anti-automation does NOT redirect this surface).
2. `waitForTabReady` + 1.5s SPA settle so the post list mounts.
3. `chrome.scripting.executeScript` injects `clickPostAnchorInPage(postPath)` — finds the anchor whose pathname matches, scrolls into view, dispatches `.click()` so FB's own SPA router (`history.pushState`) handles the in-tab navigation.
4. Polls tab URL for ≤10s until it matches the target post URL.
5. Proceeds to existing gate-1 (article stability + composer detection).

Non-group surfaces (profile posts, `/watch`, `/reel`, fb.watch, photo permalinks) keep the direct-nav flow — H1 has not been observed for those URL shapes.

Failure modes the new flow distinguishes (notes payload):

| Failure note prefix | Meaning | Next action if observed |
|---|---|---|
| `group_click.group_home_redirected` | Even `/groups/<g>/` got redirected. Account is under stronger restriction than H1 predicts. | Likely H2 (shadow restriction). Pause account, switch. |
| `group_click.post_anchor_not_found` | Group home loaded but the target post is not visible in the freshly-rendered DOM (lazy-load horizon or post out of feed window). | Tune: scroll N times before scan, OR use group search box to surface the post by id. |
| `group_click.no_navigation` | Click fired but FB SPA did not advance to the post URL. | FB intercepted the synthetic click. Try `dispatchEvent(new MouseEvent('click', {isTrusted:false, bubbles:true}))` variant; if still fails escalate to H4. |
| `group_click.inject_failed` | `chrome.scripting.executeScript` rejected. | Check manifest `scripting` permission + tab domain. |
| `group_click.landed` (ok=true) | Tab now on target post URL, gate-1 runs next. | Watch gate-1 outcome separately. |

Manifest version bumped 0.5.4 → 0.5.5 so the operator knows when to reload the extension.

---

## Root cause hypotheses for `redirected_feed` — UNCONFIRMED (historical, pre-2026-05-31)

The remaining bug produces `finished/context_drift, detail=redirected_feed` and matches the operator's observation that the extension never types. Four hypotheses; tracking which is true requires Step 1 data.

### H1 — FB redirects deep-link navigation on bot-fingerprint

The strongest hypothesis. Operator's `chrome.tabs.update` to `/groups/<g>/posts/<p>/` lands on `/groups/<g>/` or `/home.php` because Facebook's anti-automation heuristic flags the navigation (no user-gesture referrer, background/minimized window, etc.). Content script then runs on a feedish page, `waitUntilTargetArticleStable` polls for 8s without finding the target article, gate-1 fails, and `proof.js::isFeedishURL(page_url_after)` masks the gate-1 failure as `redirected_feed`.

**Code-side defense shipped against H1**:
- `c0ce159` — outbox post-navigation URL verification. Catches the redirect at ~5ms instead of wasting 8s in gate-1 timeout. Emits explicit `outbox.navigation_redirected: target_url=X actual=Y` note instead of the generic `redirected_feed`.
- `b93b783` — outbox now forces window restore + tab focus during navigation, mirroring the crawl path's already-production-proven pattern. Reduces probability that FB's "background tab" anti-bot signal fires.
- `8209178` — URL matcher harmonized with crawl-path `tabUrlMatchesExpected` (lenient subpath rule). Eliminates false-positive regression risk from `c0ce159`'s original strict matcher.

If H1 is true and these fixes are sufficient → `redirected_feed` goes away. If H1 is true and these fixes are insufficient → operator sees `outbox.navigation_redirected` notes in diagnostic — meaning fix needs to go deeper (e.g., navigate to group home first then click post link DOM to mimic human flow).

### H2 — Account #49 under FB shadow restriction on engagement

FB has soft-restricted account #49 specifically for engagement actions (comment/react/share) on group posts, while still allowing read/browse. Direct deep-link to a post URL → FB intercepts → redirects user to feed because "this account isn't allowed to comment right now."

**Distinguishing signal**: every recent comment attempt by account #49 fails with redirect, but other accounts in the same workspace succeed.

**No code-side fix** — would require account rotation or wait period for FB to clear the restriction. Diagnostic surface (`GET /api/superadmin/accounts/49/diagnostic`) shows pattern via the `attempts[]` history.

### H3 — Post-level audience restriction

Specific posts in the group have audience settings excluding account #49 (e.g., "Friends only", "Specific members", admin-only). Founder's personal browser sees the post (because founder is admin/friend); account #49 navigating to the same URL gets redirected because the post isn't visible to them.

**Distinguishing signal**: account #49 manually browsing to the post URL in extension's persistent Chrome profile gets the same redirect → confirms H3. Account #49 sees the post fine when manually browsing → refutes H3, points back to H1/H2/H4.

**No code-side fix** — UX consideration: maybe surface "post not viewable to this account" as a separate skip reason at server side before queueing.

### H4 — Chrome extension CDP/automation fingerprint detected

FB's anti-bot stack detects Chrome DevTools Protocol attachment, content script injection patterns, navigator.webdriver flags, or similar fingerprints. Once detected, FB applies session-level restrictions including deep-link engagement redirects.

**Distinguishing signal**: account #49 logged into a fresh Chrome profile (no extension attached) navigates to post URLs successfully and can comment manually; same account #49 inside extension's persistent profile fails.

**No surgical code-side fix** — full remediation requires deep extension hardening (stealth plugins, fingerprint normalization, navigation pacing). Significant scope.

---

## What user must execute to break the diagnostic loop

One sequence, ~2 minutes:

1. **Reload extension** in Chrome (`chrome://extensions/` → reload icon; full Chrome restart for safety).
2. **POST `/api/superadmin/accounts/49/reset-risk`** — clears risk_score/recent_failures/cooldown so behaviour caps don't block new attempts. Endpoint shipped in `ac7d642`.
3. **Trigger `comment_all_leads`** via copilot.
4. **GET `/api/superadmin/accounts/49/diagnostic`** — endpoint shipped in `ac7d642`. Returns parsed `attempts[]` with `notes`, `page_url_after`, `dom_snippet` extracted from `evidence_json`.
5. **Paste `attempts[0]` content** back into the investigation thread.

The instrumentation patches (`3c17f1a` content-script gates + `c0ce159` outbox layer + `ac7d642` server slog) ensure the diagnostic response carries the exact `landed_at` URL at the moment of failure. The pattern match table below gives deterministic next steps.

---

## Diagnostic decision table

| `attempts[0].notes` content | Outcome | Next code action |
|---|---|---|
| Empty + outcome=`dom_verified` + node_matched=true | ✅ **Bug fixed**. Comment posted successfully. | Close investigation. Verify with 2-3 more leads. Flip outbound_mode=auto via Settings UI (`aea772c`). |
| `outbox.navigation_redirected: target_url=<post> actual=<feed/group home>` | **H1 confirmed but candidate fixes insufficient.** FB redirect happens despite foreground tab + window. | Deeper fix: navigate to group home → wait for SPA → find + click the target post link DOM (human-flow simulation). Not direct `chrome.tabs.update` to post URL. |
| `identity_gate_1_no_article_or_unstable: ... landed_at=<post URL>` (matches target) | Page LOADED post correctly but article DOM never stable for 500ms within 8s. | Tune `waitUntilTargetArticleStable` — extend timeout to 15s, relax stableMs to 200ms, OR accept any-ready-once-in-window. |
| `identity_gate_2_post_click_swap` or `identity_gate_2b_scroll_swap` or `identity_gate_3_editor_drift`, with `landed_at` on target | Article matched initially, identity drifted mid-flow (React re-mount). | Stability handling tuneup in respective gate. |
| `landed_at=<post URL>` but `dom_snippet` contains "Content unavailable" / "Nội dung không có sẵn" | **H3 confirmed.** Post-level audience restriction for account #49. | Server-side: add `post_not_viewable` skip reason BEFORE queueing (preflight check). User-side: switch account for restricted-audience posts. |
| All recent attempts (last 20) show same failure regardless of post | **H2 confirmed.** Account-level shadow restriction. | Pause account #49 24-48h, verify FB account state manually, possibly switch accounts. |
| Notes contain mixed outcomes (some success, some `redirected_feed`) | Intermittent — likely H4 timing or FB rolling A/B test. | Investigate per-attempt timing patterns. Possibly H4 fingerprint remediation. |

---

## Code shipped this session (9 commits)

```
8209178 outbox: harmonize URL matcher with crawl-path tabUrlMatchesExpected
2060e45 Tests: extractEvidenceField parser (13 edge cases)
aea772c FE Settings: outbound automation mode toggle (Step 5 UI prep)
b93b783 outbox: foreground tab + window during navigation (H1 fix #2)
c0ce159 outbox: verify tab URL matches target after navigation (H1 fix #1)
ac7d642 Server diagnostic surface (Step 1 receiver + Step 3 endpoint + slog)
e2243eb FE: sales role browser tab (orthogonal fix)
3c17f1a Extension comment executor: landed_url instrumentation
47b669c Crawler: extract post fbid from set=gm.X (URL routing fix #2)
```

Plus `1083e4c` (pre-session) — crawler exclude photo URLs from candidate selection.

All commits independently defensible. None speculate beyond the H1 hypothesis tier. Step 5 UI ships regardless of bug outcome (user explicitly requested auto-mode toggle). Tests and harmonization shipped to reduce risk in already-landed candidates.

---

## What would need to ship next, depending on Step 1 outcome

### If H1 confirmed insufficient by candidate fixes

Replace direct `chrome.tabs.update` with human-flow simulation:
1. Navigate to `https://www.facebook.com/groups/<g>/` (group home)
2. `waitForTabReady` + settle
3. Inject content script that finds an anchor matching the target post URL, scrolls to it, dispatches `clickLikeUser` on the anchor
4. Wait for the post page to render in-tab (FB SPA's history.pushState path)
5. Proceed with existing gate-1 logic

Scope: ~80 LOC in extension. New content-script function `navigateToPostViaGroupClick`. Modifies `executeInFacebookTab` to use it instead of `chrome.tabs.update` for comment-type actions.

### If H3 confirmed

Add server-side preflight that queries FB graph/web for post visibility before queueing. Alternatively (lower cost): accept the failure mode, add `post_not_viewable_to_account` to the failure_reason taxonomy, suppress risk_score increment for this specific outcome (it's not the account's fault).

Scope: ~30 LOC server + extension proof shape update.

### If H4 confirmed

Larger initiative. Out of scope for this investigation thread. Track as separate epic.

---

## Locked discipline boundaries applied

- **`[[feedback_extraction_is_not_redesign]]`** (debug variant) — no speculative refactors during diagnosis. Why every candidate fix here is bounded to the strongest hypothesis (H1).
- **`[[project_runtime_control_plane]]`** — no CRUD-tab dashboard for execution_attempts. Why the diagnostic endpoint is single-purpose, not a tab.
- **`[[feedback_governance_self_limits]]`** — no proliferation of new memory files. Why this investigation lives as a specs/ doc, not 5 new memories.
- **`[[feedback_freeze_abstraction]]`** — no new interfaces or DI layers. Why the diagnostic endpoint hits concrete `*sql.DB`.

---

## Resume protocol

When operator returns with `attempts[0]` content from `GET /api/superadmin/accounts/49/diagnostic`:

1. Match the `notes` against the Diagnostic Decision Table above.
2. Identify confirmed hypothesis.
3. Ship the corresponding "Next code action" from the table.
4. POST `/api/superadmin/accounts/49/reset-risk`.
5. Re-trigger `comment_all_leads`, verify outcome shifts to `dom_verified`.
6. Founder flips outbound_mode=auto via Settings UI.
7. Close this investigation; archive doc to `specs/done/` or delete.

If 5+ days elapse without resume, recommend re-reading this doc end-to-end before continuing — context decays.
