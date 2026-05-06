
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
  // flat Task JSON (crawl_plan.sources[]).
  function sourceUrlFromCrawlPayload(command) {
    try {
      const payload = JSON.parse(command.payload_json || '{}');
      if (payload?.navigate_to) return payload.navigate_to;
      const sources = payload?.task?.crawl_plan?.sources || payload?.crawl_plan?.sources || [];
      const source = sources.find(s => s?.url);
      return source?.url || THGShared.FACEBOOK_HOME;
    } catch {
      return THGShared.FACEBOOK_HOME;
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
    const fbTabs = await chrome.tabs.query({
      url: ['https://facebook.com/*', 'https://*.facebook.com/*']
    });
    const windowIds = [...new Set(fbTabs.map(t => t.windowId).filter(Boolean))];
    for (const windowId of windowIds) {
      await chrome.windows.update(windowId, { state: 'minimized' }).catch(() => {});
    }
  }

  // Opens a background tab for crawling in the Facebook window.
  // If the Facebook window is minimized, restores it first so Chrome allows
  // full requestAnimationFrame scheduling — minimized windows throttle rAF
  // which prevents React/SPA from rendering the feed.
  // Returns { tab, shouldReminimize, crawlWinId } so the caller can re-minimize after.
  async function openCrawlTab(navigateTo) {
    const fbTabs = await chrome.tabs.query({
      url: ['https://facebook.com/*', 'https://*.facebook.com/*']
    });
    const crawlWinId = fbTabs[0]?.windowId || null;
    let shouldReminimize = false;
    if (crawlWinId) {
      const win = await chrome.windows.get(crawlWinId).catch(() => null);
      if (win?.state === 'minimized') {
        await chrome.windows.update(crawlWinId, { state: 'normal' }).catch(() => {});
        shouldReminimize = true;
        await THGShared.delay(600);
      }
    }
    const tabOpts = { url: navigateTo, active: false };
    if (crawlWinId) tabOpts.windowId = crawlWinId;
    const tab = await chrome.tabs.create(tabOpts);
    return { tab, shouldReminimize, crawlWinId };
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
          const navigateTo = envelope?.navigate_to || sourceUrlFromCrawlPayload(command);
          const useBackground = Boolean(envelope?.use_background_tab);
          if (useBackground) {
            // Restore the Facebook window from minimize before opening the crawl tab.
            // Inactive tabs in a visible window render at full speed; minimized
            // windows throttle rendering and prevent the feed from loading.
            const crawlInfo = await openCrawlTab(navigateTo);
            tempTabId = crawlInfo.tab.id;
            shouldReminimize = crawlInfo.shouldReminimize;
            crawlWinId = crawlInfo.crawlWinId;
            await THGFacebookState.waitForTabReady(crawlInfo.tab.id);
            await THGShared.delay(5000); // Allow Facebook SPA to fully render feed
            liveState = { ...liveState, tab: crawlInfo.tab };
          } else {
            liveState = await THGFacebookState.ensureFacebookTabVisible(navigateTo, { focus: false });
            await THGShared.delay(2500);
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

  return { process };
})();
globalThis.THGCommands = THGCommands;
