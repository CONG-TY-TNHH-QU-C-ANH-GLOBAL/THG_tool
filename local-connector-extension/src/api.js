var THGApi = globalThis.THGApi || (() => {
  async function getConfig() {
    const cfg = await THGShared.storageGet(['serverUrl', 'deviceToken', 'connectorId', 'connectorName', 'lastStatus', 'lastError']);
    return {
      serverUrl: THGShared.normalizeServerUrl(cfg.serverUrl),
      deviceToken: cfg.deviceToken || '',
      connectorId: cfg.connectorId || 0,
      connectorName: cfg.connectorName || 'THG Chrome Extension',
      lastStatus: cfg.lastStatus || '',
      lastError: cfg.lastError || ''
    };
  }

  function stateBody(state, accountId = 0) {
    return {
      account_id: accountId,
      hostname: 'Chrome Extension',
      os: `${navigator.platform || 'unknown'} / Chrome`,
      version: chrome.runtime.getManifest().version,
      kind: 'extension_connector',
      transport: 'chrome_extension',
      capabilities_json: JSON.stringify(THGShared.CAPABILITIES),
      current_url: state.currentUrl,
      fb_user_id: state.fbUserId,
      fb_display_name: state.fbDisplayName,
      fb_username: state.fbUsername,
      fb_profile_url: state.fbProfileUrl,
      login_email: state.loginEmail,
      stream_status: state.streamStatus,
      identity_confidence: state.identityConfidence || '',
      identity_extraction_method: state.identityExtractionMethod || '',
      identity_last_verified_at: state.identityLastVerifiedAt || '',
      browser_profile_id: state.browserProfileId || ''
    };
  }

  async function agentFetch(path, options = {}) {
    const cfg = await getConfig();
    if (!cfg.deviceToken) throw new Error('Extension is not paired yet');
    const headers = {
      ...(options.headers || {}),
      'X-Agent-Token': cfg.deviceToken,
      'X-Agent-Hostname': 'Chrome Extension',
      'X-Agent-OS': `${navigator.platform || 'unknown'} / Chrome`,
      'X-Agent-Version': chrome.runtime.getManifest().version
    };
    return fetch(`${cfg.serverUrl}${path}`, { ...options, headers });
  }

  async function clearDeviceToken() {
    await chrome.storage.local.remove(['deviceToken', 'connectorId', 'connectorName', 'lastStatus']);
  }

  async function pairConnector(serverUrl, code) {
    const normalized = THGShared.normalizeServerUrl(serverUrl);
    const state = await THGFacebookState.collectFacebookState();
    const res = await fetch(`${normalized}/api/connectors/pair`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        ...stateBody(state),
        code: THGShared.normalizePairingCode(code)
      })
    });
    if (!res.ok) {
      const text = await res.text();
      let message = text;
      try {
        const payload = JSON.parse(text);
        message = payload.error || payload.message || text;
      } catch {
        message = text;
      }
      if (res.status === 400 && /invalid|already used|expired/i.test(message)) {
        throw new Error('Mã kết nối không hợp lệ hoặc đã hết hạn. Tạo mã mới trong Browser dashboard, kiểm tra THG server trùng domain dashboard, rồi kết nối lại.');
      }
      throw new Error(`Kết nối thất bại (${res.status}): ${message}`);
    }
    const payload = await res.json();
    await THGShared.storageSet({
      serverUrl: normalized,
      deviceToken: payload.device_token,
      connectorId: payload.connector?.id || 0,
      connectorName: payload.connector?.name || 'THG Chrome Extension',
      lastError: ''
    });
    await THGHeartbeat.run();
    return payload;
  }

  return {
    agentFetch,
    clearDeviceToken,
    getConfig,
    pairConnector,
    stateBody
  };
})();
globalThis.THGApi = THGApi;
