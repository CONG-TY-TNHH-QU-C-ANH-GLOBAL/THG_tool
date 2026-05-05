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

  async function process(target, state) {
    if (!target || !state.fbUserId) return;
    const commands = await fetchCommands();
    if (!commands.length) return;
    let liveState = state;
    for (const command of commands) {
      let error = '';
      let tempTabId = 0;
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
            // Open a background tab so the user's active tab is never touched.
            const tab = await chrome.tabs.create({ url: navigateTo, active: false });
            tempTabId = tab.id;
            await THGFacebookState.waitForTabReady(tab.id);
            await THGShared.delay(2500);
            liveState = { ...liveState, tab };
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
      }
      await markCommandDone(command.id, error);
    }
  }

  return { process };
})();
globalThis.THGCommands = THGCommands;
