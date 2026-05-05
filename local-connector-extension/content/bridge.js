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
    return THGContentCrawl.crawlVisibleFacebookPosts(task);
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
