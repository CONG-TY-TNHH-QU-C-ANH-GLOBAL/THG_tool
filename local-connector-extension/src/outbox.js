var THGOutbox = globalThis.THGOutbox || (() => {
  let processing = null;

  async function fetchApprovedOutbox() {
    const res = await THGApi.agentFetch('/api/connectors/outbox?limit=1');
    if (!res.ok) return [];
    const payload = await res.json().catch(() => ({}));
    return Array.isArray(payload.messages) ? payload.messages : [];
  }

  // completeOutbox reports the click outcome to the backend.
  //
  // Step 3b — the body is now an ExtensionExecutionReport (see Go side at
  // internal/runtime/verifier.go). When a `proof` object is supplied by
  // the content-script executor, it ships verbatim so the backend's
  // ClassifyExtensionReport can reach `dom_verified` (instead of the
  // legacy `optimistic_success` no-proof fallback). The legacy `error`
  // field is preserved in `failure_reason` for backward compatibility
  // with older server builds during rollout.
  async function completeOutbox(id, ok, error = '', proof = null) {
    const path = ok ? 'sent' : 'failed';
    const body = proof
      ? { ...proof, success: !!ok, failure_reason: proof.failure_reason || (ok ? '' : error || '') }
      : { success: !!ok, failure_reason: ok ? '' : error || '', error };
    await THGApi.agentFetch(`/api/connectors/outbox/${id}/${path}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    });
  }

  function targetUrlForMessage(message) {
    const typ = String(message.type || '').toLowerCase();
    const raw = String(message.target_url || message.targetUrl || '').trim();
    if (raw) return raw;
    if (typ === 'profile_post') return THGShared.FACEBOOK_HOME;
    return '';
  }

  function isCommentableFacebookPostUrl(raw) {
    try {
      const url = new URL(String(raw || '').trim());
      const host = url.hostname.toLowerCase();
      if (host !== 'fb.watch' && !host.endsWith('.fb.watch') &&
        host !== 'facebook.com' && !host.endsWith('.facebook.com')) {
        return false;
      }
      const path = url.pathname.toLowerCase();
      if ((host === 'fb.watch' || host.endsWith('.fb.watch')) && path.replace(/^\/+|\/+$/g, '')) return true;
      const query = url.searchParams;
      if (query.get('story_fbid') || query.get('multi_permalinks')) return true;
      if (path.includes('/posts/') || path.includes('/permalink/') ||
        path.includes('/videos/') || path.includes('/reel/') ||
        path.includes('/watch/') || path.includes('/share/')) {
        return true;
      }
      return path.endsWith('/photo.php') && Boolean(query.get('fbid'));
    } catch {
      return false;
    }
  }

  async function executeInFacebookTab(message) {
    const targetUrl = targetUrlForMessage(message);
    if (!targetUrl) throw new Error('outbox target URL is empty');
    if (String(message.type || '').toLowerCase() === 'comment' && !isCommentableFacebookPostUrl(targetUrl)) {
      throw new Error('comment_target_not_post_permalink');
    }
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

  async function processOnce(target, state) {
    if (!target || !state.fbUserId) return;
    const messages = await fetchApprovedOutbox();
    if (!messages.length) return;
    for (const message of messages) {
      let ok = false;
      let error = '';
      let proof = null;
      try {
        const result = await executeInFacebookTab(message);
        ok = Boolean(result?.ok);
        proof = result?.proof || null;
        if (!ok) error = result?.error || result?.detail || 'outbound action failed';
      } catch (err) {
        error = err?.message || String(err);
      }
      if (error) {
        await THGShared.storageSet({ lastOutboxError: error, lastError: error }).catch(() => {});
      }
      await completeOutbox(message.id, ok, error, proof).catch(() => {});
    }
  }

  async function process(target, state) {
    if (processing) return processing;
    processing = processOnce(target, state).finally(() => {
      processing = null;
    });
    return processing;
  }

  return { process };
})();
globalThis.THGOutbox = THGOutbox;
