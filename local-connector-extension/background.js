importScripts(
  'src/shared.js',
  'src/window-policy.js',
  'src/navwatch.js',
  'src/facebook-state.js',
  'src/api.js',
  'src/stream.js',
  'src/commands.js',
  'src/outbox.js',
  'src/heartbeat.js'
);

chrome.runtime.onInstalled.addListener(() => {
  THGHeartbeat.schedule();
});

chrome.runtime.onStartup.addListener(() => {
  THGHeartbeat.schedule();
});

chrome.alarms.onAlarm.addListener(alarm => {
  if (alarm.name === THGShared.HEARTBEAT_ALARM) {
    THGHeartbeat.run().catch(err => THGShared.storageSet({ lastError: err.message || String(err) }));
  }
});

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  (async () => {
    try {
      if (message?.type === 'pair') {
        const payload = await THGApi.pairConnector(message.serverUrl, message.code);
        THGHeartbeat.schedule();
        sendResponse({ ok: true, connector: payload.connector });
        return;
      }
      if (message?.type === 'status') {
        const cfg = await THGApi.getConfig();
        const live = await THGHeartbeat.run().catch(() => null);
        sendResponse({ ok: true, config: cfg, live });
        return;
      }
      if (message?.type === 'forget') {
        await chrome.storage.local.clear();
        sendResponse({ ok: true });
        return;
      }
      if (message?.type === 'facebook_page_seen') {
        await THGHeartbeat.run().catch(() => {});
        sendResponse({ ok: true });
        return;
      }
      if (message?.type === 'thg_crawl_progress') {
        // Best-effort heartbeat relay. Failures are swallowed because the
        // crawl itself must not depend on observability working.
        try {
          await THGApi.agentFetch('/api/connectors/crawl-progress', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              task_id: message.task_id || '',
              intent: message.intent || 'facebook_crawl',
              account_id: Number(message.account_id) || 0,
              stage: message.stage || 'scraping',
              fetched: Number(message.fetched) || 0,
              max: Number(message.max) || 0,
              source_url: message.source_url || '',
              done: message.stage === 'finished'
            })
          });
        } catch { /* ignore */ }
        sendResponse({ ok: true });
        return;
      }
      sendResponse({ ok: false, error: 'unknown message' });
    } catch (err) {
      const text = err?.message || String(err);
      await THGShared.storageSet({ lastError: text });
      sendResponse({ ok: false, error: text });
    }
  })();
  return true;
});
