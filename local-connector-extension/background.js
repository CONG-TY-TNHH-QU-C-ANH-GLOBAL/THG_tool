importScripts(
  'src/shared.js',
  'src/window-policy.js',
  'src/automation_tab.js',
  'src/navwatch.js',
  'src/facebook-state.js',
  'src/api.js',
  'src/stream.js',
  'src/commands.js',
  'src/outbox.js',
  'src/reverify.js',
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
        // Forget Device clears ONLY this Chrome profile's workspace binding
        // (token/connector). Release the SERVER binding first so the profile
        // becomes re-pairable; without this the server row stays active and
        // blocks the next user with a typed pairing error. Local clear happens
        // regardless (operator intent), but a failed server release is
        // surfaced instead of silently swallowed.
        let releaseError = '';
        try {
          const res = await THGApi.agentFetch('/api/connectors/self/disconnect', { method: 'POST' });
          if (!res.ok) releaseError = `server refused release (${res.status})`;
        } catch { /* unpaired or offline — nothing to release / can't reach server */ }
        // browserProfileId is the stable per-profile installation id —
        // wiping it would make the next pairing look like a brand-new device
        // and defeat server-side ownership checks.
        const kept = await chrome.storage.local.get('browserProfileId');
        await chrome.storage.local.clear();
        if (kept?.browserProfileId) {
          await chrome.storage.local.set({ browserProfileId: kept.browserProfileId });
        }
        if (releaseError) {
          await THGShared.storageSet({ lastError: `Chưa gỡ được liên kết trên máy chủ (${releaseError}). Dùng nút Ngắt kết nối trên dashboard nếu cần.` });
        }
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
