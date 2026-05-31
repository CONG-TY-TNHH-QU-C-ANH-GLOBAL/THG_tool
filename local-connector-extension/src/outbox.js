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

  // extractGroupHomeFromPostUrl returns the group-home URL ("/groups/<g>/")
  // for a group-post URL, or "" for non-group targets (profile posts,
  // /watch, /reel, fb.watch, photo permalinks). Used to gate the
  // human-flow navigation path: only group posts go through the
  // group-home-then-click flow, since the redirect-on-deep-link pattern
  // documented in AUTOCOMMENT_REDIRECT_INVESTIGATION.md fires specifically
  // for /groups/<g>/posts/<p>/ deep links.
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

  // findAndClickPostAnchorInPage is injected via chrome.scripting.executeScript
  // and runs in the FB tab's page context. It scrolls the group feed N
  // times to lazy-load posts, then scans anchors with a wide selector net
  // (FB uses several permalink shapes) and matches by either pathname OR
  // post-id query parameter. On match: scrollIntoView + .click() so FB's
  // SPA router fires the in-tab pushState navigation.
  //
  // Why scroll-then-scan: 2026-05-31 diagnostic showed `scanned=1 anchors`
  // on the post-anchor selector immediately after 1.5s SPA settle — way
  // too few. The group home renders post permalinks incrementally via
  // virtual scrolling; the freshest posts above the fold may not include
  // the lead's target post. Scrolling 3–5 times forces FB to mount more
  // post articles into the DOM.
  //
  // Why wide selector net: FB uses at least four permalink anchor shapes
  // on group feeds:
  //   1. <a href="/groups/<g>/posts/<p>/">           direct path
  //   2. <a href="/permalink.php?story_fbid=<p>...">  legacy/external
  //   3. <a href="?story_fbid=<p>&id=<g>">            query-only on group
  //   4. <a href="...?multi_permalinks=<p>...">       multi-post links
  // The original selector (`/posts/`, `/permalink/`) only catches #1. v2
  // also extracts the post id from the target URL and id-matches against
  // story_fbid/multi_permalinks query parameters.
  //
  // Must be self-contained — chrome.scripting.executeScript serialises
  // the function source, so no closure references back to module scope.
  async function findAndClickPostAnchorInPage(postPath) {
    const sleep = (ms) => new Promise(r => setTimeout(r, ms));
    const norm = (p) => String(p || '').replace(/\/+$/, '');
    const want = norm(postPath);
    if (!want) return { ok: false, found_count: 0, scrolls: 0, post_id: '' };
    const idMatch = want.match(/\/(?:posts|permalink)\/(\d+)/);
    const postId = idMatch ? idMatch[1] : '';

    const selector = [
      'a[href*="/posts/"]',
      'a[href*="/permalink/"]',
      'a[href*="story_fbid="]',
      'a[href*="multi_permalinks="]',
    ].join(',');

    function scanOnce() {
      const anchors = Array.from(document.querySelectorAll(selector));
      for (const a of anchors) {
        if (!a.href) continue;
        let url;
        try { url = new URL(a.href, location.origin); } catch { continue; }
        const got = norm(url.pathname);
        if (got === want || got.startsWith(want + '/')) {
          return { anchor: a, total: anchors.length };
        }
        if (postId) {
          const q = url.searchParams;
          if (q.get('story_fbid') === postId ||
              q.get('multi_permalinks') === postId) {
            return { anchor: a, total: anchors.length };
          }
        }
      }
      return { anchor: null, total: anchors.length };
    }

    // First scan: maybe the post is already above the fold.
    let last = scanOnce();
    if (last.anchor) {
      try { last.anchor.scrollIntoView({ block: 'center', behavior: 'instant' }); } catch {}
      last.anchor.click();
      return { ok: true, href: last.anchor.href, scrolls: 0, matched_count: 1 };
    }

    // Scroll-then-scan loop. Each scroll triggers FB's virtual scroller
    // to render the next batch of posts. ~5 cycles (≈3000px each, 800ms
    // settle) covers a typical group's freshest 30–50 posts without
    // burning the operator-replay window.
    const MAX_SCROLLS = 5;
    for (let i = 1; i <= MAX_SCROLLS; i++) {
      try {
        window.scrollBy({ top: 3000, behavior: 'instant' });
      } catch {
        window.scrollTo(0, window.scrollY + 3000);
      }
      await sleep(800);
      last = scanOnce();
      if (last.anchor) {
        try { last.anchor.scrollIntoView({ block: 'center', behavior: 'instant' }); } catch {}
        last.anchor.click();
        return { ok: true, href: last.anchor.href, scrolls: i, matched_count: 1 };
      }
    }
    return {
      ok: false,
      found_count: last.total,
      scrolls: MAX_SCROLLS,
      post_id: postId,
    };
  }

  // navigateToPostViaGroupClick implements the human-flow navigation
  // pattern prescribed by AUTOCOMMENT_REDIRECT_INVESTIGATION.md H1
  // decision table when candidate fixes (c0ce159 URL verify, b93b783
  // foreground tab, 8209178 matcher harmonization) proved insufficient:
  // direct chrome.tabs.update to /groups/<g>/posts/<p>/ keeps being
  // redirected to / by FB's anti-automation heuristic. Group home
  // (/groups/<g>/) is a frequently-visited human surface and rarely
  // gets the redirect, so we navigate there first, then dispatch a
  // real click on the post anchor in-DOM. The click goes through FB's
  // own SPA router (history.pushState), bypassing the deep-link
  // redirect path entirely.
  //
  // Returns { ok, landed_at, notes, tab? }. On ok=true the tab is on
  // the target post URL with DOM settled; caller proceeds to gate-1.
  // On ok=false the notes string carries the diagnostic line that
  // execution_attempts.evidence_json.notes will record.
  async function navigateToPostViaGroupClick(targetUrl, message) {
    const groupHome = extractGroupHomeFromPostUrl(targetUrl);
    if (!groupHome) return { ok: false, notes: 'group_click.not_a_group_post', landed_at: '' };
    let state = await THGFacebookState.ensureFacebookTabVisible(groupHome, { focus: true });
    if (!state.tab?.id) return { ok: false, notes: 'group_click.tab_not_ready', landed_at: '' };
    if (state.tab.windowId) {
      try {
        const win = await chrome.windows.get(state.tab.windowId).catch(() => null);
        if (win && win.state === 'minimized') {
          await chrome.windows.update(state.tab.windowId, { state: 'normal', focused: true }).catch(() => {});
          await THGShared.delay(600);
        } else {
          await chrome.windows.update(state.tab.windowId, { focused: true }).catch(() => {});
        }
      } catch { /* best effort */ }
    }
    await THGFacebookState.waitForTabReady(state.tab.id, 20000).catch(() => {});
    // SPA settle: group home renders its post list incrementally via
    // virtual scrolling. 1.5s is empirically the lower bound where the
    // 5–10 freshest posts are mounted in DOM; tighter and the anchor
    // scan will miss posts that haven't lazy-loaded yet.
    await THGShared.delay(1500);
    const groupTab = await chrome.tabs.get(state.tab.id).catch(() => null);
    const groupURL = (groupTab && groupTab.url) ? String(groupTab.url) : '';
    if (!groupURL || !urlsMatchSameDestination(groupURL, groupHome)) {
      return {
        ok: false,
        landed_at: groupURL,
        notes: 'group_click.group_home_redirected: target=' + groupHome + ' actual=' + groupURL,
      };
    }
    let postPath = '';
    try { postPath = new URL(targetUrl).pathname; } catch { postPath = ''; }
    let clickResult = { ok: false, found_count: 0 };
    try {
      const out = await chrome.scripting.executeScript({
        target: { tabId: state.tab.id },
        func: findAndClickPostAnchorInPage,
        args: [postPath],
      });
      clickResult = (out && out[0] && out[0].result) || clickResult;
    } catch (err) {
      return {
        ok: false,
        landed_at: groupURL,
        notes: 'group_click.inject_failed: ' + (err && err.message ? err.message : String(err)),
      };
    }
    if (!clickResult.ok) {
      return {
        ok: false,
        landed_at: groupURL,
        notes: 'group_click.post_anchor_not_found: path=' + postPath +
               ' post_id=' + (clickResult.post_id || '') +
               ' scanned=' + (clickResult.found_count || 0) + ' anchors after ' +
               (clickResult.scrolls || 0) + ' scrolls',
      };
    }
    // Poll the tab URL until FB SPA's pushState completes. Same 10s
    // budget the gate-1 stable-wait uses; we want to fail fast if the
    // click did not produce in-tab navigation.
    const started = Date.now();
    while (Date.now() - started < 10000) {
      await THGShared.delay(300);
      const liveTab = await chrome.tabs.get(state.tab.id).catch(() => null);
      const liveURL = (liveTab && liveTab.url) ? String(liveTab.url) : '';
      if (liveURL && urlsMatchSameDestination(liveURL, targetUrl)) {
        // Article must finish mounting before gate-1 polls for the
        // composer. 800ms matches the existing post-navigation settle
        // delay in the direct-nav flow.
        await THGShared.delay(800);
        return { ok: true, landed_at: liveURL, tab: liveTab, notes: 'group_click.landed: clicked=' + (clickResult.href || '') };
      }
    }
    const finalTab = await chrome.tabs.get(state.tab.id).catch(() => null);
    const finalURL = (finalTab && finalTab.url) ? String(finalTab.url) : '';
    return {
      ok: false,
      landed_at: finalURL,
      notes: 'group_click.no_navigation: clicked=' + (clickResult.href || '') +
             ' but tab URL stayed at ' + finalURL + ' (target=' + targetUrl + ')',
    };
  }

  async function executeInFacebookTab(message) {
    const targetUrl = targetUrlForMessage(message);
    if (!targetUrl) throw new Error('outbox target URL is empty');
    if (String(message.type || '').toLowerCase() === 'comment' && !isCommentableFacebookPostUrl(targetUrl)) {
      throw new Error('comment_target_not_post_permalink');
    }
    // GROUP-POST COMMENT PATH — human-flow navigation.
    //
    // For /groups/<g>/posts/<p>/ targets we skip the direct
    // chrome.tabs.update flow entirely and use the group-home-then-
    // click pattern (navigateToPostViaGroupClick). The May-2026
    // diagnostic loop in AUTOCOMMENT_REDIRECT_INVESTIGATION.md confirmed
    // H1 — direct deep-link navigation gets redirected to / by FB's
    // anti-automation heuristic regardless of the foreground-tab /
    // URL-verify defenses shipped in c0ce159, b93b783, 8209178.
    // The group-home click goes through FB's SPA router which is the
    // human-flow that does NOT trigger the redirect.
    //
    // Non-group surfaces (/watch, /reel, profile posts, fb.watch,
    // photo permalinks) keep the direct-nav flow — H1 has not been
    // observed there and group-home isn't applicable.
    const groupHome = extractGroupHomeFromPostUrl(targetUrl);
    if (groupHome && String(message.type || '').toLowerCase() === 'comment') {
      const nav = await navigateToPostViaGroupClick(targetUrl, message);
      if (!nav.ok) {
        return {
          ok: false,
          error: 'navigation_redirected',
          proof: {
            success: false,
            failure_reason: 'redirected_feed',
            page_url_after: nav.landed_at || '',
            notes: nav.notes,
            execution_id: String(message.execution_id || ''),
          },
        };
      }
      const releaseTabLock = acquireTabExecutionLock(nav.tab.id);
      if (!releaseTabLock) {
        throw new Error('tab_busy_executing');
      }
      try {
        try {
          return await chrome.tabs.sendMessage(nav.tab.id, { type: 'thg_execute_outbound', message });
        } catch {
          await THGShared.injectContentScripts(nav.tab.id);
          return await chrome.tabs.sendMessage(nav.tab.id, { type: 'thg_execute_outbound', message });
        }
      } finally {
        releaseTabLock();
      }
    }
    // Mirror the crawl path's foreground-navigation pattern
    // (commands.js::openCrawlTab). Direct deep-link navigation
    // (chrome.tabs.update on a background/minimized FB tab) is one of
    // the patterns Facebook's anti-automation heuristics correlate
    // with bot traffic — the SPA can throttle background-tab render
    // work via requestAnimationFrame suspension, and FB's referrer/
    // user-gesture fingerprint differs for background updates. Forcing
    // focus=true makes the tab + window active during navigation,
    // matching how the crawl path already operates and giving the FB
    // SPA the same lifecycle signals a human-clicked link produces.
    // This is independent of (and complementary to) the post-
    // navigation URL verification below.
    let state = await THGFacebookState.ensureFacebookTabVisible(targetUrl, { focus: true });
    if (!state.tab?.id) throw new Error('Facebook tab is not ready');
    if (state.tab.windowId) {
      try {
        const win = await chrome.windows.get(state.tab.windowId).catch(() => null);
        if (win && win.state === 'minimized') {
          await chrome.windows.update(state.tab.windowId, { state: 'normal', focused: true }).catch(() => {});
          await THGShared.delay(600);
        } else {
          await chrome.windows.update(state.tab.windowId, { focused: true }).catch(() => {});
        }
      } catch { /* best effort */ }
    }

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
  // post/profile/page" semantics. Mirrors the crawl path's already-
  // production-proven matcher in commands.js::tabUrlMatchesExpected:
  // pathname-based (drops query + hash), trailing-slash normalized,
  // and LENIENT on subpath — `got` matches if it equals `want` OR
  // starts with `want + "/"`. The startsWith branch handles FB's
  // common post-navigation URL embellishments:
  //   want = /groups/X/posts/Y
  //   tab  = /groups/X/posts/Y/comment/Z      (FB scrolls to comment)
  //   tab  = /groups/X/posts/Y                (no comment focus)
  //   tab  = /groups/X/posts/Y/?modal=true    (modal variant)
  // All three are "same target post" semantically — failing the
  // outbox safety check on /comment/Z would cause false-positive
  // navigation_redirected outcomes when the comment actually landed
  // and FB just deep-linked to the new comment. Strict exact-pathname
  // (what c0ce159 originally shipped) would have caused this regression.
  function urlsMatchSameDestination(actual, expected) {
    try {
      const a = new URL(actual);
      const b = new URL(expected);
      if (a.hostname !== b.hostname) return false;
      const got = a.pathname.replace(/\/+$/, '');
      const want = b.pathname.replace(/\/+$/, '');
      if (!want || !got) return false;
      return got === want || got.startsWith(want + '/');
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
