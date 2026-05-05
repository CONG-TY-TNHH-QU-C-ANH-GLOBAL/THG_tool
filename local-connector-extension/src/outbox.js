var THGOutbox = globalThis.THGOutbox || (() => {
  async function fetchApprovedOutbox() {
    const res = await THGApi.agentFetch('/api/connectors/outbox?limit=5');
    if (!res.ok) return [];
    const payload = await res.json().catch(() => ({}));
    return Array.isArray(payload.messages) ? payload.messages : [];
  }

  async function completeOutbox(id, ok, error = '') {
    const path = ok ? 'sent' : 'failed';
    await THGApi.agentFetch(`/api/connectors/outbox/${id}/${path}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ error })
    });
  }

  function targetUrlForMessage(message) {
    const typ = String(message.type || '').toLowerCase();
    const raw = String(message.target_url || message.targetUrl || '').trim();
    if (raw) return raw;
    if (typ === 'profile_post') return THGShared.FACEBOOK_HOME;
    return '';
  }

  async function executeInFacebookTab(message) {
    const targetUrl = targetUrlForMessage(message);
    if (!targetUrl) throw new Error('outbox target URL is empty');
    let state = await THGFacebookState.ensureFacebookTabVisible(targetUrl);
    if (!state.tab?.id) throw new Error('Facebook tab is not ready');
    await THGFacebookState.waitForTabReady(state.tab.id, 20000);
    await THGShared.delay(1200);
    try {
      return await chrome.tabs.sendMessage(state.tab.id, { type: 'thg_execute_outbound', message });
    } catch {
      await THGShared.injectContentScripts(state.tab.id);
      return chrome.tabs.sendMessage(state.tab.id, { type: 'thg_execute_outbound', message });
    }
  }

  async function process(target, state) {
    if (!target || !state.fbUserId) return;
    const messages = await fetchApprovedOutbox();
    if (!messages.length) return;
    for (const message of messages) {
      let ok = false;
      let error = '';
      try {
        const result = await executeInFacebookTab(message);
        ok = Boolean(result?.ok);
        if (!ok) error = result?.error || result?.detail || 'outbound action failed';
      } catch (err) {
        error = err?.message || String(err);
      }
      await completeOutbox(message.id, ok, error).catch(() => {});
    }
  }

  return { process };
})();
globalThis.THGOutbox = THGOutbox;
