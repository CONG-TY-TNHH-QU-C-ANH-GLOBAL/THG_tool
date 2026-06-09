
var THGCommands = globalThis.THGCommands || (() => {
  async function fetchCommands() {
    const res = await THGApi.agentFetch('/api/connectors/commands?limit=10');
    if (!res.ok) return [];
    const payload = await res.json().catch(() => ({}));
    return Array.isArray(payload.commands) ? payload.commands : [];
  }

  async function markCommandDone(commandId, error = '') {
    await THGApi.agentFetch(`/api/connectors/commands/${commandId}/done`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ error })
    }).catch(() => {});
  }

  async function executeInFacebookTab(tab, command) {
    if (!tab?.id) throw new Error('Facebook tab is not ready');
    try {
      return await chrome.tabs.sendMessage(tab.id, { type: 'thg_execute_command', command });
    } catch {
      await THGShared.injectContentScripts(tab.id);
      return chrome.tabs.sendMessage(tab.id, { type: 'thg_execute_command', command });
    }
  }

  // Supports both new envelope format (navigate_to at top level) and legacy
  // flat Task JSON (crawl_plan.sources[]). Returns "" when no concrete URL is
  // present so the caller can fail loud instead of silently crawling the
  // newsfeed (which previously hid routing bugs end-to-end).
  function sourceUrlFromCrawlPayload(command) {
    try {
      const payload = JSON.parse(command.payload_json || '{}');
      if (payload?.navigate_to) return String(payload.navigate_to);
      const sources = payload?.task?.crawl_plan?.sources || payload?.crawl_plan?.sources || [];
      const source = sources.find(s => s?.url);
      return source?.url ? String(source.url) : '';
    } catch {
      return '';
    }
  }

  function expectedPathFromUrl(url) {
    try {
      const u = new URL(url);
      // Normalize trailing slash so /groups/123 and /groups/123/ match.
      return u.pathname.replace(/\/+$/, '');
    } catch {
      return '';
    }
  }

  function tabUrlMatchesExpected(tabUrl, expectedUrl) {
    if (!expectedUrl) return false;
    const want = expectedPathFromUrl(expectedUrl);
    if (!want) return false;
    const got = expectedPathFromUrl(tabUrl || '');
    if (!got) return false;
    return got === want || got.startsWith(want + '/');
  }

  function crawlNavigateUrl(raw) {
    try {
      const u = new URL(String(raw || ''));
      if (!/(^|\.)facebook\.com$/i.test(u.hostname)) return String(raw || '');
      const parts = u.pathname.split('/').filter(Boolean);
      if (parts.length === 2 && parts[0] === 'groups') {
        u.searchParams.set('sorting_setting', 'CHRONOLOGICAL');
      }
      return u.toString();
    } catch {
      return String(raw || '');
    }
  }

  async function sendCrawlResult(command, result) {
    if (!result?.crawl_result) return;
    const body = {
      ...result.crawl_result,
      account_id: command.account_id || command.accountId || 0,
      status: 'completed'
    };
    const res = await THGApi.agentFetch('/api/connectors/crawl-result', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });
    if (!res.ok) {
      const text = await res.text().catch(() => '');
      throw new Error(`crawl result failed (${res.status}) ${text}`.trim());
    }
  }

  // Minimizes every Chrome window that has a Facebook tab so the user can
  // observe automation through the dashboard BrowserView instead.
  async function handleWindowControl(payload) {
    const action = String(payload?.action || '').toLowerCase();
    if (action !== 'minimize') return;
    // Window Respect (PR-2): never minimize the user's window in normal flow.
    // Honour a backend window_control:minimize ONLY when the debug/operator policy
    // explicitly enables it — otherwise it is a no-op (the user's full-screen Chrome
    // must not snap away under them).
    if (!THGWindowPolicy.shouldMinimizeAfterExecution()) return;
    const fbTabs = await chrome.tabs.query({
      url: ['https://facebook.com/*', 'https://*.facebook.com/*']
    });
    const windowIds = [...new Set(fbTabs.map(t => t.windowId).filter(Boolean))];
    for (const windowId of windowIds) {
      await chrome.windows.update(windowId, { state: 'minimized' }).catch(() => {});
    }
  }

  // Opens a foreground crawl tab in the Facebook window. Keeping the tab active
  // makes Facebook's virtual feed load consistently during long crawls.
  // If the Facebook window is minimized, restores it first so Chrome allows
  // full requestAnimationFrame scheduling; minimized windows throttle rAF
  // which prevents React/SPA from rendering the feed.
  // Returns { tab, shouldReminimize, crawlWinId } so the caller can re-minimize after.
  async function openCrawlTab(navigateTo, reuse = false) {
    const fbTabs = await chrome.tabs.query({
      url: ['https://facebook.com/*', 'https://*.facebook.com/*']
    });
    const crawlWinId = fbTabs[0]?.windowId || null;
    let shouldReminimize = false;
    if (crawlWinId) {
      const win = await chrome.windows.get(crawlWinId).catch(() => null);
      if (win?.state === 'minimized') {
        await chrome.windows.update(crawlWinId, { state: 'normal', focused: true }).catch(() => {});
        shouldReminimize = true;
        await THGShared.delay(600);
      } else {
        await chrome.windows.update(crawlWinId, { focused: true }).catch(() => {});
      }
    }
    // One automation tab per connector (PR-2): the COMMENT outbox path passes
    // reuse=true → reuse the remembered tab if alive (navigate it) instead of
    // opening a new tab per lead. The crawl path passes reuse=false so its temporary
    // tab stays independent and never hijacks the comment batch's tab.
    if (reuse) {
      const reused = await THGAutomationTab.reuseIfAlive(navigateTo);
      if (reused) {
        if (reused.windowId) await chrome.windows.update(reused.windowId, THGWindowPolicy.focusUpdate()).catch(() => {});
        return { tab: reused, shouldReminimize, crawlWinId: reused.windowId || crawlWinId };
      }
    }
    const tabOpts = { url: navigateTo, active: true };
    if (crawlWinId) tabOpts.windowId = crawlWinId;
    const tab = await chrome.tabs.create(tabOpts);
    if (reuse) await THGAutomationTab.remember(tab && tab.id);
    if (tab?.windowId) {
      // Window Respect (PR-2): focus only — do NOT force state:'normal' over a
      // maximized/fullscreen window (that snaps it to half-screen). A maximized
      // window renders fine; only a MINIMIZED one needed the restore above (124).
      await chrome.windows.update(tab.windowId, THGWindowPolicy.focusUpdate()).catch(() => {});
    }
    return { tab, shouldReminimize, crawlWinId };
  }

  // Re-reads tab.url after a navigate to confirm Facebook didn't redirect to
  // newsfeed. Returns the live tab object so the caller has fresh state.
  async function verifyTabAtExpected(tabId, expectedUrl) {
    const tab = await chrome.tabs.get(tabId).catch(() => null);
    if (!tab) return { tab: null, matched: false };
    return { tab, matched: tabUrlMatchesExpected(tab.url, expectedUrl) };
  }

  async function navigateAndVerify(navigateTo, opts = {}) {
    // Try up to 3 times: open tab, wait ready, then verify URL matches.
    // Facebook can redirect /groups/<id> to newsfeed when the user is logged
    // out or hits a checkpoint. Retrying without verification would mask that
    // and silently crawl the wrong page.
    //
    // PR8A: capture the navigation trace (from/to/duration/attempts + the last
    // actual landed URL) so the comment executor and the nav-failure path can
    // report a precise, reproducible reason instead of a bare redirect.
    //
    // PR8B-Redirect (settleMs): the post-ready settle is configurable. CRAWL
    // keeps the default 5000ms (the feed needs time to virtual-render). COMMENT
    // passes a SHORT settle (~800ms) on purpose — ROOT_CAUSE_REPORT proved the
    // target post loads+stabilises by ~t+2.9s but Facebook's SPA router resets
    // the tab to the home feed at ~t+8.4s; the old fixed 5000ms settle handed
    // the content script off at exactly that reset edge, so the comment executor
    // always entered on home. A short settle hands off inside the stable window
    // so gate-1 finds the post and types before the reset. gate-1 still polls
    // for article stability, so a small settle does not risk acting too early.
    const settleMs = typeof opts.settleMs === 'number' ? opts.settleMs : 5000;
    const navStart = Date.now();
    let lastActual = '';
    for (let attempt = 1; attempt <= 3; attempt++) {
      const info = await openCrawlTab(navigateTo, opts.reuseTab === true);
      const fromUrl = info.tab?.pendingUrl || info.tab?.url || '';
      try {
        await THGFacebookState.waitForTabReady(info.tab.id);
      } catch {
        // continue; verifyTabAtExpected will re-check tab state
      }
      await THGShared.delay(settleMs); // SPA render (crawl=5000; comment=short, beats FB reset)
      const { tab, matched } = await verifyTabAtExpected(info.tab.id, navigateTo);
      lastActual = tab?.url || lastActual;
      if (matched) {
        return {
          tab: tab || info.tab,
          shouldReminimize: info.shouldReminimize,
          crawlWinId: info.crawlWinId,
          navTrace: {
            from_url: fromUrl,
            to_url: navigateTo,
            landed_url: tab?.url || navigateTo,
            duration_ms: Date.now() - navStart,
            attempts: attempt,
          },
        };
      }
      console.warn(`[THGCommands] navigate verify failed attempt ${attempt}: expected=${navigateTo} actual=${tab?.url || 'unknown'}`);
      // Close the failed attempt's tab before retrying so temp crawl tabs don't
      // accumulate — EXCEPT the persistent automation tab (reuseTab), which must
      // stay open for the user / evidence and is simply re-navigated next attempt.
      if (opts.reuseTab !== true) {
        try { await chrome.tabs.remove(info.tab.id); } catch { /* ignore */ }
      }
      await THGShared.delay(3000);
    }
    // Surface the last actual landed URL + total duration so the caller can
    // classify the redirect (feed/home/login/checkpoint) without guessing.
    const err = new Error(`navigate verify failed: expected=${navigateTo} actual=${lastActual || 'unknown'}`);
    err.navTrace = { to_url: navigateTo, landed_url: lastActual, duration_ms: Date.now() - navStart, attempts: 3 };
    throw err;
  }

  async function process(target, state) {
    if (!target || !state.fbUserId) return;
    const commands = await fetchCommands();
    if (!commands.length) return;
    let liveState = state;
    for (const command of commands) {
      let error = '';
      let tempTabId = 0;
      let shouldReminimize = false;
      let crawlWinId = null;
      try {
        const cmdType = String(command.type || '').toLowerCase();
        if (cmdType === 'window_control') {
          const payload = JSON.parse(command.payload_json || '{}');
          await handleWindowControl(payload);
        } else if (cmdType === 'crawl') {
          const envelope = JSON.parse(command.payload_json || '{}');
          const requestedUrl = envelope?.navigate_to || sourceUrlFromCrawlPayload(command);
          if (!requestedUrl) {
            throw new Error('missing navigate_to in crawl payload (refusing newsfeed fallback)');
          }
          const navigateTo = crawlNavigateUrl(requestedUrl);
          console.log(`[THGCommands] crawl command #${command.id} navigate_to=${navigateTo}`);
          const useBackground = Boolean(envelope?.use_background_tab);
          if (useBackground) {
            // Use a temporary active crawl tab so Facebook keeps rendering the
            // virtual feed while the scraper scrolls.
            const crawlInfo = await navigateAndVerify(navigateTo);
            tempTabId = crawlInfo.tab.id;
            shouldReminimize = crawlInfo.shouldReminimize;
            crawlWinId = crawlInfo.crawlWinId;
            liveState = { ...liveState, tab: crawlInfo.tab };
          } else {
            liveState = await THGFacebookState.ensureFacebookTabVisible(navigateTo, { focus: true });
            await THGShared.delay(2500);
            const { matched } = await verifyTabAtExpected(liveState.tab?.id, navigateTo);
            if (!matched) {
              throw new Error(`navigate verify failed: expected=${navigateTo} actual=${liveState.tab?.url || 'unknown'}`);
            }
          }
          const result = await executeInFacebookTab(liveState.tab, command);
          if (!result?.ok) throw new Error(result?.error || 'command failed');
          if (result.crawl_result) await sendCrawlResult(command, result);
        } else {
          if (!liveState.tab) {
            liveState = await THGFacebookState.ensureFacebookTabVisible(undefined, { focus: false });
          }
          const result = await executeInFacebookTab(liveState.tab, command);
          if (!result?.ok) throw new Error(result?.error || 'command failed');
          if (result.crawl_result) await sendCrawlResult(command, result);
        }
      } catch (err) {
        error = err?.message || String(err);
      } finally {
        if (tempTabId) {
          await chrome.tabs.remove(tempTabId).catch(() => {});
        }
        // Re-minimize the Facebook window after crawl if we had restored it.
        if (shouldReminimize && crawlWinId) {
          await chrome.windows.update(crawlWinId, { state: 'minimized' }).catch(() => {});
        }
      }
      await markCommandDone(command.id, error);
    }
  }

  return { process, navigateAndVerify };
})();
globalThis.THGCommands = THGCommands;
