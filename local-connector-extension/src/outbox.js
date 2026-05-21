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

  async function executeInFacebookTab(message) {
    const targetUrl = targetUrlForMessage(message);
    if (!targetUrl) throw new Error('outbox target URL is empty');
    if (String(message.type || '').toLowerCase() === 'comment' && !isCommentableFacebookPostUrl(targetUrl)) {
      throw new Error('comment_target_not_post_permalink');
    }
    let state = await THGFacebookState.ensureFacebookTabVisible(targetUrl);
    if (!state.tab?.id) throw new Error('Facebook tab is not ready');

    // PRIORITY A — per-tab execution lock at the BACKGROUND layer.
    // Defense-in-depth alongside the content-script-level lock in
    // content/bridge.js: if some other background caller starts a
    // mutate-class command on this tab concurrently, that caller
    // gets short-circuited here before any work is queued. We refuse
    // by THROWING instead of waiting — the outbox poller will pick
    // the message up again next cycle (every few seconds). Queueing
    // would silently extend latency without bounded back-pressure.
    const releaseTabLock = acquireTabExecutionLock(state.tab.id);
    if (!releaseTabLock) {
      throw new Error('tab_busy_executing');
    }

    try {
      await THGFacebookState.waitForTabReady(state.tab.id, 20000);
      await THGShared.delay(1200);

      // POST-NAVIGATION URL VERIFICATION.
      //
      // Critical safety check that the crawl path (commands.js
      // navigateAndVerify) already enforces, but the outbox path was
      // missing: confirm the tab actually settled on the target URL
      // before letting the content-script gates try to find the
      // article. Facebook can redirect a deep-link navigation
      // (chrome.tabs.update to /groups/<g>/posts/<p>/) to a feedish
      // surface (group home, /home.php) when:
      //   - the account has a soft anti-automation flag
      //   - the post moderation pending / restricted for this account
      //   - FB's session triggered a checkpoint
      //   - direct deep-link navigation without referrer/user-gesture
      //     fingerprint pattern got intercepted
      // Without this check the content-script's identity_gate_1 sits
      // through the full 8s stable-wait timeout on the WRONG page and
      // proof.js then masks the failure as 'redirected_feed' without
      // ever surfacing the navigation mismatch — exactly the
      // operator-frustrating symptom seen during the May-2026
      // diagnostic loop. Returning early here costs ~5ms instead of
      // ~8s and gives the operator-replay UI a clean, specific note.
      const liveTab = await chrome.tabs.get(state.tab.id).catch(() => null);
      const liveURL = (liveTab && liveTab.url) ? String(liveTab.url) : '';
      if (liveURL && !urlsMatchSameDestination(liveURL, targetUrl)) {
        console.warn('[THGOutbox] navigation redirected before content-script dispatch',
          { target_url: targetUrl, actual_url: liveURL });
        return {
          ok: false,
          error: 'navigation_redirected',
          proof: {
            success: false,
            failure_reason: 'redirected_feed',
            page_url_after: liveURL,
            notes: 'outbox.navigation_redirected: target_url=' + targetUrl +
                   ' actual=' + liveURL +
                   ' (FB navigation diverged before content-script could run gate-1)',
            execution_id: String(message.execution_id || ''),
          },
        };
      }

      try {
        return await chrome.tabs.sendMessage(state.tab.id, { type: 'thg_execute_outbound', message });
      } catch {
        await THGShared.injectContentScripts(state.tab.id);
        return await chrome.tabs.sendMessage(state.tab.id, { type: 'thg_execute_outbound', message });
      }
    } finally {
      releaseTabLock();
    }
  }

  // urlsMatchSameDestination compares two FB URLs for "same target
  // post/profile/page" semantics. Uses pathname only (query params and
  // hash strip out tracking/comment-deeplinks). Trailing slashes
  // normalized. Mirrors sameFacebookDestination in facebook-state.js
  // but kept local to outbox.js so the safety check stays self-
  // contained inside the outbox module.
  function urlsMatchSameDestination(actual, expected) {
    try {
      const a = new URL(actual);
      const b = new URL(expected);
      if (a.hostname !== b.hostname) return false;
      const ap = a.pathname.replace(/\/+$/, '');
      const bp = b.pathname.replace(/\/+$/, '');
      return ap === bp;
    } catch {
      return false;
    }
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
