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
    if (String(message.type || '').toLowerCase() === 'comment' && !isCommentableFacebookPostUrl(targetUrl)) {
      throw new Error('comment_target_not_post_permalink');
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
