importScripts(
  'src/shared.js',
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
      sendResponse({ ok: false, error: 'unknown message' });
    } catch (err) {
      const text = err?.message || String(err);
      await THGShared.storageSet({ lastError: text });
      sendResponse({ ok: false, error: text });
    }
  })();
  return true;
});
