var THGStream = globalThis.THGStream || (() => {
  async function sendHeartbeat(state) {
    const res = await THGApi.agentFetch('/api/connectors/heartbeat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(THGApi.stateBody(state))
    });
    if (res.status === 401 || res.status === 403) {
      await THGApi.clearDeviceToken();
      throw new Error('Phiên kết nối Chrome đã hết hiệu lực hoặc đã bị ngắt khỏi workspace. Tạo mã mới trên dashboard và kết nối lại.');
    }
    if (!res.ok) throw new Error(`Không đồng bộ được kênh kết nối Chrome (${res.status})`);
  }

  async function sendChromeStatus(target, state) {
    const res = await THGApi.agentFetch('/api/connectors/chrome-status', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(THGApi.stateBody(state, target?.account_id || target?.accountId || 0))
    });
    if (!res.ok) throw new Error(`chrome status failed (${res.status})`);
  }

  async function fetchTargets() {
    const res = await THGApi.agentFetch('/api/connectors/browser-targets');
    if (!res.ok) return [];
    const payload = await res.json().catch(() => ({}));
    return Array.isArray(payload.targets) ? payload.targets : [];
  }

  // sendScreenshot was removed (PR-F): the live screenshot stream is gone. This
  // module now only carries the connector's liveness + identity sync (heartbeat,
  // chrome-status, browser-targets). captureVisibleTab is no longer used.

  return {
    fetchTargets,
    sendChromeStatus,
    sendHeartbeat
  };
})();
globalThis.THGStream = THGStream;
