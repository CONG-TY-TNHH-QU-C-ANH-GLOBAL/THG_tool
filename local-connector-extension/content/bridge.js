if (!globalThis.THGContentBridgeInstalled) {
globalThis.THGContentBridgeInstalled = true;

async function thgExecuteCommand(command) {
  const basic = THGContentCommands.executeBasicCommand(command);
  if (basic) return basic;
  const payload = typeof command.payload_json === 'string' ? JSON.parse(command.payload_json || '{}') : (command.payload_json || {});
  if (String(command.type || '').toLowerCase() === 'crawl') {
    // Support both old flat-task format and new ConnectorCrawlEnvelope format
    // where the task lives under payload.task.
    const task = payload?.task || payload;
    // Forward the expected URL so the content script can refuse to scrape
    // when Facebook silently redirected the tab (login wall, checkpoint).
    const expectedUrl = payload?.navigate_to
      || task?.crawl_plan?.sources?.[0]?.url
      || '';
    const gate = payload?.market_signal_gate || task?.extras?.market_signal_gate || null;
    const userPrompt = payload?.user_prompt || task?.extras?.user_prompt || '';
    const accountId = command?.account_id || command?.accountId || 0;
    const result = await THGContentCrawl.crawlVisibleFacebookPosts(task, expectedUrl, accountId);
    // Echo gate + user prompt back to the server so the crawl-result endpoint
    // applies the same Brain-derived gating and anchors the AI classifier to
    // the operator's current goal without re-reading org context.
    if (result?.ok && result?.crawl_result) {
      if (gate) result.crawl_result.market_signal_gate = gate;
      if (userPrompt) result.crawl_result.user_prompt = userPrompt;
    }
    return result;
  }
  return { ok: false, error: `Unsupported command type: ${command.type}` };
}

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  (async () => {
    try {
      if (message?.type === 'thg_collect_meta') {
        sendResponse({ ok: true, meta: THGContentMeta.collectFacebookMeta() });
        return;
      }
      if (message?.type === 'thg_execute_command') {
        sendResponse(await thgExecuteCommand(message.command || {}));
        return;
      }
      if (message?.type === 'thg_execute_outbound') {
        sendResponse(await THGContentOutbound.executeOutbound(message.message || {}));
        return;
      }
      sendResponse({ ok: false, error: 'unknown content message' });
    } catch (err) {
      sendResponse({ ok: false, error: err?.message || String(err) });
    }
  })();
  return true;
});
}
