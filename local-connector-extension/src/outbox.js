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
    // Foreground the tab/window WITHOUT navigating (no reload → no frame churn).
    try { if (state.tab.windowId) await chrome.windows.update(state.tab.windowId, { state: 'normal', focused: true }); } catch { /* ignore */ }
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

    // ─── COMMENT DELIVERY — SINGLE PATH (Path 2: comment in the group feed) ──
    // Root cause of `c.rung2.no_terminal_from_content_script` (confirmed once
    // the AI text-generation bug was fixed and a real comment finally reached
    // execution): the rung2 path had the CONTENT SCRIPT navigate the tab to the
    // permalink while it was holding the onMessage response channel open. ANY
    // navigation — successful or redirected — reloads the document and destroys
    // the very content script awaiting the response, so the channel closes
    // before a terminal is returned. It is an architectural flaw, not FB
    // blocking the permalink.
    //
    // Path 2 fixes this structurally: the BACKGROUND does the navigation first
    // (navigateAndVerify to /groups/<g>/ — group HOME, the crawler's proven
    // surface), THEN sends the comment command to the already-loaded content
    // script, which only locates the target article by post_id and types — it
    // never navigates, so its frame survives and a terminal always returns.
    //
    // Non-group comment targets (profile posts, /watch, /reel, fb.watch, photo
    // permalinks) have no group home; they fall through to the generic
    // crawler-nav path below (background navigates, content script only types).
    if (msgType === 'comment') {
      const groupHome = extractGroupHomeFromPostUrl(targetUrl);
      if (groupHome) {
        return await executeInGroupFeed(message, targetUrl, groupHome);
      }
      // No group home → fall through to the direct crawler-nav path below.
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
