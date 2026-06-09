var THGOutbox = globalThis.THGOutbox || (() => {
  let processing = null;

  // tabExecutionLocks enforces the "1 tab = 1 active outbound execution"
  // invariant at the background layer. Keyed by Chrome tab id; the
  // value is the in-flight Promise so a second caller can either await
  // it or short-circuit. The map is module-scoped on purpose — there
  // is exactly one background service worker per browser, so this
  // singleton owns the tab-execution truth for the whole extension.
  //
  // Why per-tab instead of per-account: Chrome tabs are the
  // composition unit for FB sessions. Two tabs CAN belong to the same
  // FB account (user opens FB in two windows) but they have separate
  // DOM, separate React state, separate composer focus. Locking by
  // account would forbid legitimate parallelism on two tabs of the
  // same account; locking by tab forbids the actual race we care
  // about — two outbound commands mutating the same DOM tree.
  //
  // Why this is NOT redundant with `processing`: the module-level
  // `processing` lock serialises calls to processOnce (outbox batch).
  // But other code paths can also invoke chrome.tabs.sendMessage on
  // the same content script — heartbeat (thg_collect_meta), crawl
  // (thg_execute_command). Per-tab lock guarantees no two MUTATING
  // commands execute concurrently regardless of code path.
  const tabExecutionLocks = new Map();

  // acquireTabExecutionLock returns a release function when the lock
  // is granted, OR null when another execution is already in flight
  // on the same tab. The caller MUST call release() in a finally so
  // the lock cannot leak across an exception path.
  function acquireTabExecutionLock(tabId) {
    if (!tabId || typeof tabId !== 'number') return null;
    if (tabExecutionLocks.has(tabId)) return null;
    let release;
    const promise = new Promise(resolve => { release = resolve; });
    tabExecutionLocks.set(tabId, promise);
    return () => {
      tabExecutionLocks.delete(tabId);
      release();
    };
  }

  async function fetchApprovedOutbox() {
    const res = await THGApi.agentFetch('/api/connectors/outbox?limit=1');
    if (!res.ok) return [];
    const payload = await res.json().catch(() => ({}));
    return Array.isArray(payload.messages) ? payload.messages : [];
  }

  // completeOutbox reports the click outcome to the backend.
  //
  // Step 3b — the body is now an ExtensionExecutionReport (see Go side at
  // internal/runtime/verifier.go). When a `proof` object is supplied by
  // the content-script executor, it ships verbatim so the backend's
  // ClassifyExtensionReport can reach `dom_verified` (instead of the
  // legacy `optimistic_success` no-proof fallback). The legacy `error`
  // field is preserved in `failure_reason` for backward compatibility
  // with older server builds during rollout.
  // completeOutbox ships the terminal callback for one queued message.
  //
  // executionId MUST be forwarded explicitly (not derived from proof
  // alone): when the content script fails to load or throws before it
  // can build a proof object, proof is null but the row's execution_id
  // is still meaningful. Without it the body would omit execution_id
  // and the backend's CAS would reject the report as stale — turning
  // an honest network error into a phantom "stale token" failure.
  async function completeOutbox(id, ok, error = '', proof = null, executionId = '') {
    const path = ok ? 'sent' : 'failed';
    const exec = (proof && proof.execution_id) || executionId || '';
    const body = proof
      ? { ...proof, success: !!ok, failure_reason: proof.failure_reason || (ok ? '' : error || ''), execution_id: exec }
      : { success: !!ok, failure_reason: ok ? '' : error || '', error, execution_id: exec };
    await THGApi.agentFetch(`/api/connectors/outbox/${id}/${path}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });
  }

  function targetUrlForMessage(message) {
    const typ = String(message.type || '').toLowerCase();
    const raw = String(message.target_url || message.targetUrl || '').trim();
    if (raw) return raw;
    if (typ === 'profile_post') return THGShared.FACEBOOK_HOME;
    return '';
  }

  function isCommentableFacebookPostUrl(raw) {
    try {
      const url = new URL(String(raw || '').trim());
      const host = url.hostname.toLowerCase();
      if (host !== 'fb.watch' && !host.endsWith('.fb.watch') &&
        host !== 'facebook.com' && !host.endsWith('.facebook.com')) {
        return false;
      }
      const path = url.pathname.toLowerCase();
      if ((host === 'fb.watch' || host.endsWith('.fb.watch')) && path.replace(/^\/+|\/+$/g, '')) return true;
      const query = url.searchParams;
      if (query.get('story_fbid') || query.get('multi_permalinks')) return true;
      if (path.includes('/posts/') || path.includes('/permalink/') ||
        path.includes('/videos/') || path.includes('/reel/') ||
        path.includes('/watch/') || path.includes('/share/')) {
        return true;
      }
      return path.endsWith('/photo.php') && Boolean(query.get('fbid'));
    } catch {
      return false;
    }
  }

  // extractGroupHomeFromPostUrl returns "/groups/<g>/" for a group-post
  // URL, or "" for non-group targets. Used to gate the Path 2 group-feed
  // flow: only group posts route through it. Profile posts / /watch /
  // /reel / fb.watch / photo permalinks keep the direct-nav crawler
  // pattern (executeInFacebookTab below) since FB's redirect-on-deep-link
  // behaviour is specific to /groups/<g>/posts/<p>/ targets.
  function extractGroupHomeFromPostUrl(targetUrl) {
    try {
      const url = new URL(String(targetUrl || '').trim());
      const m = url.pathname.match(/^\/groups\/([^/]+)\/(posts|permalink)\//);
      if (!m) return '';
      return url.origin + '/groups/' + m[1] + '/';
    } catch {
      return '';
    }
  }

  function extractPostIdFromTargetUrl(targetUrl) {
    try {
      const url = new URL(String(targetUrl || '').trim());
      const m = url.pathname.match(/\/(?:posts|permalink)\/(\d+)/);
      return m ? m[1] : '';
    } catch {
      return '';
    }
  }

  // executeInGroupFeed implements Path 2 (commit pending 2026-06-01):
  // navigate to /groups/<g>/ (group home — proven non-redirected) and
  // dispatch the new content-script command 'thg_comment_in_group_feed'.
  // The content-script handler (THGContentOutbound.executeCommentInFeed)
  // scrolls the feed to find the target article by post_id, then comments
  // from feed context. We never load /groups/<g>/posts/<p>/.
  //
  // Why a separate function from executeInFacebookTab: outbox.js stays
  // a thin router. Per-flow setup (target_url → group_home derivation,
  // post_id extraction, group_feed message type) lives here; tab
  // lifecycle (lock, sendMessage, cleanup) mirrors the sibling exactly.
  //
  // If Path 2 still fails while operating entirely from group feed
  // context, that's strong evidence FB is detecting content-script
  // execution itself regardless of nav surface — at which point we'd
  // escalate to GraphQL API as the next investigation track.
  async function executeInGroupFeed(message, targetUrl, groupHomeUrl) {
    const postId = extractPostIdFromTargetUrl(targetUrl);
    let crawlInfo;
    try {
      crawlInfo = await THGCommands.navigateAndVerify(groupHomeUrl, { reuseTab: true });
    } catch (err) {
      return {
        ok: false,
        error: 'navigation_redirected',
        proof: {
          success: false,
          failure_reason: 'redirected_feed',
          page_url_after: '',
          notes: 'path2.group_home_nav_failed: target_group=' + groupHomeUrl +
                 ' err=' + (err && err.message ? err.message : String(err)) +
                 ' (crawler-pattern navigateAndVerify exhausted 3 retries to group home — ' +
                 'if this triggers consistently, even the proven crawler nav surface is restricted for this account)',
          execution_id: String(message.execution_id || ''),
        },
      };
    }
    const tabId = crawlInfo.tab && crawlInfo.tab.id;
    if (!tabId) throw new Error('Facebook tab is not ready after navigateAndVerify');

    const releaseTabLock = acquireTabExecutionLock(tabId);
    if (!releaseTabLock) {
      // Window Respect (PR-2): another execution owns this tab — don't close it
      // (would disrupt the other run / the user's tab). Back off; the poller retries.
      if (THGWindowPolicy.shouldCloseTabAfterExecution()) {
        await chrome.tabs.remove(tabId).catch(() => {});
      }
      throw new Error('tab_busy_executing');
    }

    const payload = { ...message, post_id: postId };
    let result;
    try {
      try {
        result = await chrome.tabs.sendMessage(tabId, { type: 'thg_comment_in_group_feed', message: payload });
      } catch {
        await THGShared.injectContentScripts(tabId);
        result = await chrome.tabs.sendMessage(tabId, { type: 'thg_comment_in_group_feed', message: payload });
      }
    } finally {
      releaseTabLock();
      // Window Respect (PR-2): the user-owned window is sacred. By default we LEAVE
      // the Facebook tab open on its final page so the user can inspect the comment
      // (success or failure) — no auto-close, no minimize. The verifier already ran
      // inside the content script. Only a debug build flips these policy flags.
      if (THGWindowPolicy.shouldCloseTabAfterExecution()) {
        await chrome.tabs.remove(tabId).catch(() => {});
      }
      if (THGWindowPolicy.shouldMinimizeAfterExecution() && crawlInfo.shouldReminimize && crawlInfo.crawlWinId) {
        await chrome.windows.update(crawlInfo.crawlWinId, { state: 'minimized' }).catch(() => {});
      }
    }
    return result;
  }

  // ─── STAGE 0 PROBE helpers (specs plan: Locate→Execute→Verify rebuild) ───
  // isHomeOrFeedUrl detects the redirect-to-home/feed landing that is the
  // signature of FB rejecting our automated navigation.
  function isHomeOrFeedUrl(raw) {
    try {
      const u = new URL(String(raw || ''));
      const p = u.pathname.replace(/\/+$/, '');
      if (p === '' || p === '/home.php' || p === '/feed') return true;
      return false;
    } catch {
      return false;
    }
  }
  // isLoginOrCheckpointUrl detects a login wall / identity checkpoint — these
  // map to human_required, not a navigation failure.
  function isLoginOrCheckpointUrl(raw) {
    const s = String(raw || '').toLowerCase();
    return s.includes('/login') || s.includes('checkpoint') ||
      s.includes('two_step') || s.includes('/recover');
  }

  // classifyLandingBg is the BACKGROUND-side mirror of content/navreport.js
  // classifyLanding (the content module is not in scope in the service worker).
  // Returns the same closed vocabulary as internal/models/nav_diagnostic.go
  // RedirectClass* so the nav-failure path reports a precise cause.
  function classifyLandingBg(raw) {
    const s = String(raw || '');
    if (!s) return 'unknown';
    const low = s.toLowerCase();
    if (/\/login(\/|\.php|$|\?)/.test(low) || low.includes('login.facebook.com')) return 'login';
    if (low.includes('/checkpoint') || low.includes('two_step') || low.includes('/recover')) return 'checkpoint';
    let url;
    try { url = new URL(s); } catch { return 'unknown'; }
    const host = url.hostname.toLowerCase();
    const isFB = host === 'facebook.com' || host.endsWith('.facebook.com') ||
      host === 'fb.watch' || host.endsWith('.fb.watch');
    if (!isFB) return 'unknown';
    const path = url.pathname.replace(/\/+$/, '');
    if (path === '' || path === '/') return 'home';
    if (path === '/home.php' || path === '/feed' || path === '/feed.php') return 'feed';
    if (isCommentableFacebookPostUrl(s)) return 'permalink';
    if (path.startsWith('/photo') || path.startsWith('/marketplace') ||
      path.startsWith('/events') || path.startsWith('/profile.php') ||
      path.startsWith('/story.php') || /^\/groups\/[^/]+$/.test(path)) {
      return 'unsupported_target';
    }
    return 'unknown';
  }

  // deliverCommentViaPermalink implements STAGE 1 — Option C (the TargetLocator):
  // navigate the user's visible, logged-in FB tab IN-SESSION directly to the
  // post permalink (the post IS the page), then dispatch the existing
  // permalink-page executor (executeComment via thg_execute_outbound), which
  // locates the target article, runs its identity checkpoints (the last-moment
  // re-assertion), types, and submits. No fresh-tab, no group-home, no
  // feed-scroll rediscovery.
  //
  // The probe (Stage 0) confirmed Rung 1 works: focused in-session nav lands on
  // the permalink (not redirected to /). This is that nav, now followed by
  // delivery instead of a probe-only report.
  //
  // Locator identity gate (classify landed state before acting):
  //   login/checkpoint → checkpoint (human required, session held)
  //   feed/home (/)     → redirected_feed (Rung 1 bounced — escalate to Rung 2)
  //   permalink         → dispatch the executor
  async function deliverCommentViaPermalink(message, targetUrl) {
    const execId = String(message.execution_id || '');
    const fail = (failure_reason, notes) => ({
      ok: false,
      error: failure_reason,
      proof: { success: false, failure_reason, page_url_after: '', notes, execution_id: execId },
    });

    // Establish/reuse a logged-in FB tab at home (in-session base — never
    // fresh-tab straight to a deep URL; that is the redirect trigger).
    const home = await THGFacebookState.ensureFacebookTabVisible(THGShared.FACEBOOK_HOME, { focus: true });
    if (!home || !home.fbUserId) {
      return fail('soft_fail', 'c.locate.not_logged_in: no c_user — open Facebook and log in, then retry');
    }
    // Account safety: if the work item carries the target account's FB uid, it
    // MUST match the logged-in session, or we would comment from the WRONG
    // account. Dormant until the backend adds account_fb_user_id to the work
    // item (Stage 1b); harmless no-op when absent.
    const wantUid = String(message.account_fb_user_id || message.fb_user_id || '').trim();
    if (wantUid && wantUid !== String(home.fbUserId)) {
      return fail('soft_fail',
        'c.locate.wrong_account: logged-in c_user=' + home.fbUserId +
        ' != action account fb_user_id=' + wantUid + ' (switch the Facebook tab to the correct account)');
    }

    // Navigate that SAME established tab in-session to the post permalink.
    const liveState = await THGFacebookState.ensureFacebookTabVisible(targetUrl, { focus: true });
    const tabId = liveState && liveState.tab && liveState.tab.id;
    if (!tabId) return fail('soft_fail', 'c.locate.no_tab_after_nav: target=' + targetUrl);
    let landed = (liveState && liveState.currentUrl) || '';
    try {
      const t = await chrome.tabs.get(tabId);
      if (t && t.url) landed = t.url;
    } catch { /* keep currentUrl */ }

    if (isLoginOrCheckpointUrl(landed)) {
      return fail('checkpoint', 'c.locate.login_or_checkpoint: target=' + targetUrl + ' landed_at=' + landed);
    }
    if (isHomeOrFeedUrl(landed)) {
      return fail('redirected_feed',
        'c.locate.nav_redirected_permalink: target=' + targetUrl + ' landed_at=' + landed +
        ' (Rung1 focused in-session nav bounced to feed — escalate to Rung 2 in-SPA nav)');
    }

    // Landed on a (non-feed) permalink page → hand off to the executor.
    // Per-tab lock guards against a concurrent mutating command on this tab.
    const releaseTabLock = acquireTabExecutionLock(tabId);
    if (!releaseTabLock) {
      throw new Error('tab_busy_executing');
    }
    let result;
    try {
      try {
        result = await chrome.tabs.sendMessage(tabId, { type: 'thg_execute_outbound', message });
      } catch {
        await THGShared.injectContentScripts(tabId);
        result = await chrome.tabs.sendMessage(tabId, { type: 'thg_execute_outbound', message });
      }
    } finally {
      releaseTabLock();
      // Option C: this is the user's visible tab. Do NOT close or minimize it —
      // observability (north star) + it stays ready for the next action.
    }
    return result;
  }

  // sendMutationWithTimeout dispatches a content-script command but NEVER waits
  // forever. MV3 can silently drop the response (service-worker recycle, or the
  // content script's message channel closing mid-navigation — the "message
  // channel closed before a response was received" error). Without a bound, the
  // background `await chrome.tabs.sendMessage` hangs indefinitely, which freezes
  // the per-tab execution lock AND the outbox (no completeOutbox ever fires —
  // the 7-minutes-of-silence symptom). Promise.race guarantees the caller always
  // reaches a terminal so completeOutbox runs and the row stays retryable.
  function sendMutationWithTimeout(tabId, payload, timeoutMs) {
    return Promise.race([
      chrome.tabs.sendMessage(tabId, payload),
      new Promise((_, reject) => setTimeout(
        () => reject(new Error('content_script_no_response_' + timeoutMs + 'ms')), timeoutMs)),
    ]);
  }

  // deliverCommentViaRung2 — REAL delivery on the confirmed Rung-2 navigation.
  // Establish a stable, logged-in home tab (the in-SPA click base), then hand
  // the whole nav+comment to the content script (the click MUST happen in-page).
  // The user's visible tab is reused and never closed (observability).
  async function deliverCommentViaRung2(message) {
    const execId = String(message.execution_id || '');
    const fail = (failure_reason, notes) => ({
      ok: false,
      error: failure_reason,
      proof: { success: false, failure_reason, page_url_after: '', notes, execution_id: execId },
    });

    // Reuse an EXISTING logged-in FB tab WITHOUT forcing a reload. A full
    // top-level navigation (the old ensureFacebookTabVisible→HOME) destroys the
    // tab's main frame mid-operation → "Frame with ID 0 was removed" → the
    // comment never lands. Only create a tab if none exists. Then bring it
    // foreground (human-flow) by activating/focusing — NOT navigating it.
    let state = await THGFacebookState.collectFacebookState();
    if (!state || !state.tab || !THGShared.isFacebookUrl(state.tab.url)) {
      state = await THGFacebookState.ensureFacebookTabVisible(THGShared.FACEBOOK_HOME, { focus: true });
    }
    if (!state || !state.fbUserId) {
      return fail('soft_fail', 'c.locate.not_logged_in: open Facebook and log in, then retry');
    }
    // Account safety (dormant until backend adds account_fb_user_id; no-op when absent).
    const wantUid = String(message.account_fb_user_id || message.fb_user_id || '').trim();
    if (wantUid && wantUid !== String(state.fbUserId)) {
      return fail('soft_fail',
        'c.locate.wrong_account: logged-in c_user=' + state.fbUserId +
        ' != action account fb_user_id=' + wantUid + ' (switch the Facebook tab to the correct account)');
    }
    const tabId = state.tab && state.tab.id;
    if (!tabId) return fail('soft_fail', 'c.locate.no_fb_tab');
    // Foreground the tab/window WITHOUT navigating (no reload → no frame churn) and
    // WITHOUT resizing — Window Respect (PR-2): focus only, never force state:'normal'
    // over the user's maximized/fullscreen window unless window management is enabled.
    try { if (state.tab.windowId) await chrome.windows.update(state.tab.windowId, THGWindowPolicy.focusUpdate()); } catch { /* ignore */ }
    try { await chrome.tabs.update(tabId, { active: true }); } catch { /* ignore */ }
    await THGShared.delay(500);

    // The content script does: Rung-2 click-nav → wait for permalink → executeComment.
    const releaseTabLock = acquireTabExecutionLock(tabId);
    if (!releaseTabLock) {
      throw new Error('tab_busy_executing');
    }
    let result;
    try {
      try {
        result = await sendMutationWithTimeout(tabId, { type: 'thg_comment_via_rung2', message }, 75000);
      } catch (e1) {
        // Re-inject + resend ONLY when the content script genuinely isn't loaded.
        // NOT on timeout: a blind resend would click-navigate a SECOND time
        // (double comment / modal churn). On timeout or any other error, fall
        // through to a terminal proof below.
        const m1 = (e1 && e1.message) || String(e1);
        if (/Receiving end does not exist|Could not establish connection/i.test(m1)) {
          await THGShared.injectContentScripts(tabId);
          result = await sendMutationWithTimeout(tabId, { type: 'thg_comment_via_rung2', message }, 75000);
        } else {
          throw e1;
        }
      }
    } catch (e2) {
      // Never hang the outbox: turn a lost/timed-out response into a terminal,
      // retryable, no-risk outcome with a diagnostic note (surfaced in chat).
      const m2 = (e2 && e2.message) || String(e2);
      result = {
        ok: false,
        error: 'rung2_no_terminal',
        proof: {
          success: false,
          failure_reason: 'soft_fail',
          page_url_after: '',
          notes: 'c.rung2.no_terminal_from_content_script: ' + m2 +
                 ' (content script hung or message channel closed before responding; outbox freed, retryable)',
          execution_id: execId,
        },
      };
    } finally {
      releaseTabLock();
      // Visible tab — do NOT close or minimize (observability + ready for next action).
    }
    return result;
  }

  // probeRung2 (background half) — does the Rung-2 in-SPA click navigation
  // reach AND HOLD on the permalink? This catches the LATE redirect that
  // fooled the Stage-0 background-read probe (which measured during the brief
  // landing, before FB bounced the tab to /). From a stable logged-in home
  // tab, it asks the content script to click-navigate to the permalink (a
  // genuine click FB's router can turn into in-SPA pushState), then samples
  // the tab URL over ~4s FROM THE BACKGROUND (survives a top-load unload that
  // would kill a content-script timer). No comment is typed.
  // failure_reason='soft_fail' → no risk penalty, retryable.
  async function probeRung2(message, targetUrl) {
    const execId = String(message.execution_id || '');
    const targetId = extractPostIdFromTargetUrl(targetUrl);
    const out = (notes) => ({
      ok: false,
      error: 'rung2_probe',
      proof: { success: false, failure_reason: 'soft_fail', page_url_after: '', notes, execution_id: execId },
    });

    const home = await THGFacebookState.ensureFacebookTabVisible(THGShared.FACEBOOK_HOME, { focus: true });
    if (!home || !home.fbUserId) return out('c.rung2_probe: not_logged_in — open Facebook and log in, then retry');
    const tabId = home.tab && home.tab.id;
    if (!tabId) return out('c.rung2_probe: no_fb_tab');

    // Ask the content script (on the stable home page) to click-navigate.
    let clickInfo = null;
    let clickError = '';
    try {
      clickInfo = await chrome.tabs.sendMessage(tabId, { type: 'thg_nav_probe_rung2', message: { target_url: targetUrl } });
    } catch {
      try {
        await THGShared.injectContentScripts(tabId);
        clickInfo = await chrome.tabs.sendMessage(tabId, { type: 'thg_nav_probe_rung2', message: { target_url: targetUrl } });
      } catch (e2) {
        clickError = (e2 && e2.message) || String(e2);
      }
    }

    // Measure the post-click URL trajectory from the background.
    const sampleUrl = async () => {
      try { const t = await chrome.tabs.get(tabId); return (t && t.url) || ''; } catch { return ''; }
    };
    const traj = [];
    traj.push('t0=' + (await sampleUrl()));
    await THGShared.delay(1500); traj.push('t1.5=' + (await sampleUrl()));
    await THGShared.delay(2500); traj.push('t4=' + (await sampleUrl()));

    const landed = await sampleUrl();
    const reachedHeld = !!targetId && landed.indexOf(targetId) !== -1 && !isHomeOrFeedUrl(landed);
    const method = (clickInfo && clickInfo.method) || (clickError ? 'unknown(click_msg_failed)' : 'unknown');
    return out(
      'c.rung2_probe: target_id=' + (targetId || '?') +
      ' method=' + method +
      ' reached_and_held=' + reachedHeld +
      (clickError ? ' click_msg_error=' + clickError : '') +
      ' trajectory=[' + traj.join(' | ') + ']'
    );
  }

  // forensicPatchMain is injected into the COMMENT tab's MAIN world (page
  // context) via chrome.scripting.executeScript before the content script
  // handoff. PR8C-Forensics: it patches history.pushState/replaceState so when
  // Facebook resets the tab to the home feed we capture the EXACT timestamp +
  // FB's own stack trace at the call site, and forwards it to the isolated
  // content script (content/forensics.js) via window.postMessage. Must be a
  // self-contained function (it is serialised by executeScript — no closure
  // refs). Idempotent: guards against double-patch. Passive: always calls the
  // original; never changes navigation behaviour.
  function forensicPatchMain() {
    try {
      if (window.__thgForensicPatched) return;
      window.__thgForensicPatched = true;
      const post = (method, url) => {
        try {
          window.postMessage({
            source: 'THG_FORENSIC_PUSHSTATE',
            method,
            url: String(url || location.href),
            ts: Date.now(),
            stack: String((new Error()).stack || '').slice(0, 900),
          }, '*');
        } catch (_) {}
      };
      const op = history.pushState;
      history.pushState = function (s, t, url) { post('pushState', url); return op.apply(this, arguments); };
      const or = history.replaceState;
      history.replaceState = function (s, t, url) { post('replaceState', url); return or.apply(this, arguments); };
      window.addEventListener('popstate', () => post('popstate', location.href), true);
    } catch (_) {}
  }

  // executeInFacebookTab navigates the FB tab to the target URL and
  // dispatches the outbound command into the page-context content script.
  //
  // Navigation strategy (2026-06-01): REUSE the crawler's
  // `THGCommands.navigateAndVerify` helper instead of bespoke
  // group-click logic.
  //
  // Background — investigation log in specs/AUTOCOMMENT_REDIRECT_INVESTIGATION.md:
  //   f2645e6 + 63def8a shipped a "group home + click post anchor" detour
  //   to bypass FB's deep-link redirect (H1). Diagnostic on 2026-05-31
  //   showed the click never fired because target post anchors weren't
  //   present in the freshly-rendered group home DOM (modern FB defers
  //   permalink anchor mount until hover). Strategy was a dead end.
  //
  //   The crawler (commands.js::navigateAndVerify) has been navigating
  //   reliably for months WITHOUT redirect. The key difference: crawler
  //   uses `chrome.tabs.create` (fresh tab with URL — fingerprinted as
  //   user-clicked link), outbox was using `chrome.tabs.update` on the
  //   existing FB tab (background-nav fingerprint that FB's anti-bot
  //   stack flags and redirects to /). Switching outbox to the same
  //   helper inherits the crawler's proven nav profile: 3 retries,
  //   5000ms SPA settle, URL verification, close-tab-on-fail.
  //
  // Tab lifecycle (PR-2 + PR-2.1): comment execution reuses ONE persistent
  // automation tab per connector (reuseTab:true → THGAutomationTab). A 10-comment
  // batch runs on a single tab the user can watch; the tab is NOT closed/minimized
  // after execution (Window Respect). We still acquire the per-tab execution lock
  // before dispatching so two comments never run in the same tab concurrently.
  async function executeInFacebookTab(message) {
    const targetUrl = targetUrlForMessage(message);
    if (!targetUrl) throw new Error('outbox target URL is empty');
    const msgType = String(message.type || '').toLowerCase();
    if (msgType === 'comment' && !isCommentableFacebookPostUrl(targetUrl)) {
      throw new Error('comment_target_not_post_permalink');
    }

    // ─── COMMENT DELIVERY — SINGLE PATH: navigate directly to the post ──
    // Comments fall straight through to the generic crawler-nav path below:
    // navigateAndVerify(targetUrl, { reuseTab:true }) navigates the PERSISTENT
    // automation tab to the post permalink (reusing it across the batch; creating
    // one only if none is alive), using the crawler's proven retry + settle flow,
    // then thg_execute_outbound runs executeComment on that loaded page — the
    // content script only types/submits, it never navigates. That avoids BOTH
    // dead ends we have now ruled out by telemetry:
    //   - rung2 `no_terminal_from_content_script`: the content script navigated
    //     while holding the response channel open → channel closed mid-flow.
    //   - Path 2 `article_not_found_in_feed`: nav landed on the home feed
    //     (nav_at_entry=https://www.facebook.com/), not the group, so scrolling
    //     the feed for the post was a dead end (found 2 articles, never the
    //     target). Scrolling a feed to rediscover a known post is the wrong
    //     model — go straight to the permalink instead.

    // PR8A.1: mark when we begin opening the comment tab, so THGNavWatch can
    // hand back exactly the navigation events that fired on this tab during
    // the attempt — that trace NAMES who redirected it to home.
    const navWatchStart = Date.now();
    let crawlInfo;
    try {
      // PR8B-Redirect: short settle for the comment path. ROOT_CAUSE_REPORT
      // (2026-06-04) proved the post is stable ~t+2.9s but FB's SPA router
      // resets to home ~t+8.4s; the crawler's 5000ms settle handed off at that
      // exact edge. 800ms hands off inside the stable window so gate-1 types
      // before the reset. (Crawl keeps the 5000ms default.)
      crawlInfo = await THGCommands.navigateAndVerify(targetUrl, { settleMs: 800, reuseTab: true });
    } catch (err) {
      // PR8A: navigateAndVerify exhausted 3 retries — the post permalink never
      // held (FB redirected every attempt). This is target_not_reached, not the
      // generic redirected_feed: nothing was ever typed, it is retryable, and it
      // must not poison the account's risk_score. Classify the last landed URL
      // so the operator sees the precise cause (feed/home/login/checkpoint).
      const navTrace = (err && err.navTrace) || {};
      const landed = navTrace.landed_url || '';
      const rc = classifyLandingBg(landed);
      return {
        ok: false,
        error: 'target_not_reached',
        proof: {
          success: false,
          failure_reason: 'target_not_reached',
          page_url_after: landed,
          notes: 'outbox.crawler_nav_failed (target_not_reached): redirect_class=' + rc +
                 ' attempts=' + (navTrace.attempts || 3) + ' duration_ms=' + (navTrace.duration_ms || 0) +
                 ' · ' + (err && err.message ? err.message : String(err)),
          nav_diagnostic: {
            nav_to_url: navTrace.to_url || targetUrl,
            nav_duration_ms: navTrace.duration_ms || 0,
            nav_attempts: navTrace.attempts || 3,
            landed_url: landed,
            doc_title: '',
            article_found: false,
            permalink_found: false,
            comment_button_found: false,
            target_post_id: extractPostIdFromTargetUrl(targetUrl),
            account_id: Number(message.account_id || message.accountId || 0) || 0,
            redirect_class: rc,
            stage: 'background_nav_failed',
          },
          execution_id: String(message.execution_id || ''),
        },
      };
    }
    const tabId = crawlInfo.tab && crawlInfo.tab.id;
    if (!tabId) throw new Error('Facebook tab is not ready after navigateAndVerify');
    // PR8A: hand the background navigation trace to the content script so the
    // NavDiagnostic it builds at each gate carries from/to/duration/attempts.
    message.nav_trace = crawlInfo.navTrace || null;

    const releaseTabLock = acquireTabExecutionLock(tabId);
    if (!releaseTabLock) {
      // Race with another mutate-class caller on the just-created tab is implausible
      // (the tab is brand new). Window Respect (PR-2): don't close by default; back
      // off and let the outbox poller retry next cycle.
      if (THGWindowPolicy.shouldCloseTabAfterExecution()) {
        await chrome.tabs.remove(tabId).catch(() => {});
      }
      throw new Error('tab_busy_executing');
    }

    // PR8C-Forensics: install the MAIN-world history.pushState interceptor
    // BEFORE handing off to the content script, so the FB reset that fires
    // ~100ms after our content-script activity is captured with its timestamp +
    // stack trace. Best-effort (needs Chrome 111+ for world:'MAIN'); a failure
    // here must never block delivery.
    try {
      await chrome.scripting.executeScript({ target: { tabId }, world: 'MAIN', func: forensicPatchMain });
    } catch (_) { /* forensics injection is best-effort */ }

    let result;
    try {
      try {
        result = await chrome.tabs.sendMessage(tabId, { type: 'thg_execute_outbound', message });
      } catch {
        await THGShared.injectContentScripts(tabId);
        result = await chrome.tabs.sendMessage(tabId, { type: 'thg_execute_outbound', message });
      }
    } finally {
      releaseTabLock();
      // PR8A.1: BEFORE removing the tab, fold the webNavigation trace for this
      // tab into the proof's nav_diagnostic. The ring buffer already holds the
      // events (they were recorded as they fired), so this is just a read —
      // but we do it pre-remove so tabId is unambiguous. This is the trace that
      // names whether home came from FB (server/client_redirect, SPA history)
      // or our own chrome.tabs code (typed/auto_toplevel, no redirect qualifier).
      try {
        if (result && result.proof && globalThis.THGNavWatch) {
          const evs = THGNavWatch.eventsFor(tabId, navWatchStart);
          if (evs.length) {
            result.proof.nav_diagnostic = result.proof.nav_diagnostic || {};
            result.proof.nav_diagnostic.nav_events = evs;
          }
        }
      } catch (_) { /* telemetry must never break delivery */ }
      // PR8A evidence pack: on FAILURE, capture the failing tab WHILE IT IS STILL
      // OPEN (and still active in its window) so the operator sees the exact
      // feed/login/post state. Out-of-band: the raw JPEG rides proof
      // .evidence_screenshot_b64 (NOT nav_diagnostic) → the server writes it to
      // disk and records only the path. Telemetry-only; never breaks delivery.
      try {
        if (result && result.proof && result.ok === false) {
          const winId = (crawlInfo.tab && crawlInfo.tab.windowId) || crawlInfo.crawlWinId || undefined;
          const shot = await chrome.tabs.captureVisibleTab(winId, { format: 'jpeg', quality: 40 }).catch(() => '');
          if (shot) result.proof.evidence_screenshot_b64 = shot;
        }
      } catch (_) { /* screenshot is best-effort evidence, never load-bearing */ }
      // Window Respect (PR-2): by default LEAVE the tab open on its final page so
      // the user can inspect the comment result, and never minimize the window.
      // (Per-connector tab REUSE is the future approach to avoid accumulation — not
      // in this PR; the founder chose "don't close" over auto-close.) Debug only.
      if (THGWindowPolicy.shouldCloseTabAfterExecution()) {
        await chrome.tabs.remove(tabId).catch(() => {});
      }
      if (THGWindowPolicy.shouldMinimizeAfterExecution() && crawlInfo.shouldReminimize && crawlInfo.crawlWinId) {
        await chrome.windows.update(crawlInfo.crawlWinId, { state: 'minimized' }).catch(() => {});
      }
    }
    return result;
  }


  async function processOnce(target, state) {
    if (!target || !state.fbUserId) return;
    const messages = await fetchApprovedOutbox();
    if (!messages.length) return;
    for (const message of messages) {
      let ok = false;
      let error = '';
      let proof = null;
      try {
        const result = await executeInFacebookTab(message);
        ok = Boolean(result?.ok);
        proof = result?.proof || null;
        if (!ok) error = result?.error || result?.detail || 'outbound action failed';
      } catch (err) {
        error = err?.message || String(err);
      }
      // PR8D.1: a TAB-LIFECYCLE error ("No tab with id", "No window with id",
      // "Frame ... was removed", "message channel closed") means the tab/frame
      // vanished BEFORE the content script could act — nothing was typed or
      // submitted. With no proof the server would classify this as
      // shadow_rejected (a non-retryable "we tried and FB silently rejected"),
      // stranding the lead. It is actually transient (SW recycle / window
      // minimized → tab discarded / tab closed mid-flow), so report it as
      // soft_fail with a proof so the row stays RETRYABLE next poll cycle.
      if (!ok && !proof && error && /No tab with id|No window with id|Frame .*was removed|message channel closed|Receiving end does not exist/i.test(error)) {
        proof = {
          success: false,
          failure_reason: 'soft_fail',
          page_url_after: '',
          notes: 'tab_lifecycle (retryable, nothing attempted): ' + error,
          execution_id: String(message.execution_id || ''),
        };
      }
      if (error) {
        await THGShared.storageSet({ lastOutboxError: error, lastError: error }).catch(() => {});
      }
      await completeOutbox(message.id, ok, error, proof, message.execution_id || '').catch(() => {});
    }
  }

  async function process(target, state) {
    if (processing) return processing;
    processing = processOnce(target, state).finally(() => {
      processing = null;
    });
    return processing;
  }

  return { process };
})();
globalThis.THGOutbox = THGOutbox;
