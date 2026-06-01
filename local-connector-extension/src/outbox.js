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
      crawlInfo = await THGCommands.navigateAndVerify(groupHomeUrl);
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
      await chrome.tabs.remove(tabId).catch(() => {});
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
      await chrome.tabs.remove(tabId).catch(() => {});
      if (crawlInfo.shouldReminimize && crawlInfo.crawlWinId) {
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

  // probeCommentNavigation implements STAGE 0 (the gate): it tests Rung 1 of
  // the navigation ladder — does a focused, IN-SESSION navigation of an
  // ESTABLISHED logged-in FB tab to the post permalink survive FB's redirect?
  // It NEVER comments. It establishes a logged-in tab at home first (so we are
  // never fresh-tab-ing straight to a deep URL — that is the redirect trigger),
  // then navigates that same tab in-session to target_url (the permalink), and
  // reports where it landed via proof.notes (surfaced in operator chat by A1).
  //
  // failure_reason='soft_fail' on purpose: a probe must NOT poison the account
  // risk profile and MUST stay retryable so the operator can re-probe freely.
  async function probeCommentNavigation(message, targetUrl) {
    const execId = String(message.execution_id || '');
    const probe = (notes) => ({
      ok: false,
      error: 'nav_probe',
      proof: { success: false, failure_reason: 'soft_fail', page_url_after: '', notes, execution_id: execId },
    });

    // Step 1: establish/reuse a logged-in FB tab at home (in-session base).
    const home = await THGFacebookState.ensureFacebookTabVisible(THGShared.FACEBOOK_HOME, { focus: true });
    if (!home || !home.fbUserId) {
      return probe('c.nav_probe: not_logged_in (no c_user) — open Facebook and log in, then retry');
    }

    // Step 2: navigate that SAME established tab in-session to the permalink.
    const liveState = await THGFacebookState.ensureFacebookTabVisible(targetUrl, { focus: true });
    const tabId = liveState && liveState.tab && liveState.tab.id;
    let landed = (liveState && liveState.currentUrl) || '';
    try {
      const t = await chrome.tabs.get(tabId);
      if (t && t.url) landed = t.url;
    } catch { /* keep currentUrl */ }

    const targetId = extractPostIdFromTargetUrl(targetUrl);
    const redirected = isHomeOrFeedUrl(landed);
    const login = isLoginOrCheckpointUrl(landed);
    const identityOk = !redirected && !login && !!targetId && landed.indexOf(targetId) !== -1;
    return probe(
      'c.nav_probe: target=' + targetUrl +
      ' landed_at=' + landed +
      ' target_id=' + (targetId || '?') +
      ' identity_ok=' + identityOk +
      ' redirected=' + redirected +
      ' login_or_checkpoint=' + login +
      ' (Rung1 focused in-session nav to permalink; no comment typed)'
    );
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
  // Tab lifecycle: the tab created by navigateAndVerify is temporary.
  // We acquire the per-tab execution lock, dispatch the command, then
  // close the tab in finally so a 10-comment daily batch doesn't leak
  // 10 dangling tabs into the FB window.
  async function executeInFacebookTab(message) {
    const targetUrl = targetUrlForMessage(message);
    if (!targetUrl) throw new Error('outbox target URL is empty');
    const msgType = String(message.type || '').toLowerCase();
    if (msgType === 'comment' && !isCommentableFacebookPostUrl(targetUrl)) {
      throw new Error('comment_target_not_post_permalink');
    }

    // ─── STAGE 0 GATE ───────────────────────────────────────────────────
    // Until the navigation probe confirms Rung 1 works, ALL comment actions
    // run the probe (navigate the focused in-session tab to the permalink and
    // report where it lands) instead of attempting delivery. No comment is
    // typed. Read the result in chat via proof.notes (A1). Replace this block
    // with the real Locate→Execute→Verify path (Stage 1) once the probe lands
    // on the permalink. The Path-2 code below is dormant during Stage 0.
    if (msgType === 'comment') {
      return await probeCommentNavigation(message, targetUrl);
    }

    // PATH 2 routing: group-post comments take the feed-context flow
    // (navigate to /groups/<g>/, find article by post_id, comment inline)
    // instead of the permalink-page flow that keeps hitting FB's silent
    // redirect-back-to-/. Non-group surfaces (/watch, /reel, profile
    // posts, fb.watch, photo permalinks) keep the direct-nav crawler
    // pattern below — H1's permalink-redirect signal has only been
    // observed on /groups/<g>/posts/<p>/ targets.
    if (msgType === 'comment') {
      const groupHome = extractGroupHomeFromPostUrl(targetUrl);
      if (groupHome) {
        return await executeInGroupFeed(message, targetUrl, groupHome);
      }
    }

    let crawlInfo;
    try {
      crawlInfo = await THGCommands.navigateAndVerify(targetUrl);
    } catch (err) {
      return {
        ok: false,
        error: 'navigation_redirected',
        proof: {
          success: false,
          failure_reason: 'redirected_feed',
          page_url_after: '',
          notes: 'outbox.crawler_nav_failed: ' + (err && err.message ? err.message : String(err)),
          execution_id: String(message.execution_id || ''),
        },
      };
    }
    const tabId = crawlInfo.tab && crawlInfo.tab.id;
    if (!tabId) throw new Error('Facebook tab is not ready after navigateAndVerify');

    const releaseTabLock = acquireTabExecutionLock(tabId);
    if (!releaseTabLock) {
      // Race with another mutate-class caller on the just-created tab is
      // implausible (the tab is brand new), but we still defend: clean up
      // and let the outbox poller retry next cycle.
      await chrome.tabs.remove(tabId).catch(() => {});
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
      // Mirror the crawler's cleanup so the FB window doesn't accumulate
      // tabs across a daily batch. Restore window state if crawler had
      // to un-minimize it during navigateAndVerify.
      await chrome.tabs.remove(tabId).catch(() => {});
      if (crawlInfo.shouldReminimize && crawlInfo.crawlWinId) {
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
