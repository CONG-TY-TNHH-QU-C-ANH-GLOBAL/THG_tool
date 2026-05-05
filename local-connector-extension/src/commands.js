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

  function sourceUrlFromCrawlPayload(command) {
    try {
      const payload = JSON.parse(command.payload_json || '{}');
      const source = payload?.crawl_plan?.sources?.find(s => s?.url);
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

  async function process(target, state) {
    if (!target || !state.fbUserId) return;
    const commands = await fetchCommands();
    if (!commands.length) return;
    let liveState = state;
    for (const command of commands) {
      let error = '';
      try {
        if (String(command.type || '').toLowerCase() === 'crawl') {
          liveState = await THGFacebookState.ensureFacebookTabVisible(sourceUrlFromCrawlPayload(command), { focus: false });
          await THGShared.delay(2500);
        } else if (!liveState.tab) {
          liveState = await THGFacebookState.ensureFacebookTabVisible(undefined, { focus: false });
        }
        const result = await executeInFacebookTab(liveState.tab, command);
        if (!result?.ok) throw new Error(result?.error || 'command failed');
        if (result.crawl_result) await sendCrawlResult(command, result);
      } catch (err) {
        error = err?.message || String(err);
      }
      await markCommandDone(command.id, error);
    }
  }

  return { process };
})();
globalThis.THGCommands = THGCommands;
