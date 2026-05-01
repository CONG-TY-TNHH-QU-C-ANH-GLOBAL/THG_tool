const DEFAULT_SERVER_URL = 'https://sale.thgfulfill.com';
const HEARTBEAT_ALARM = 'thg-heartbeat';
const CAPABILITIES = {
  local_chrome: true,
  browser_control: 'user_chrome_extension',
  screen_capture: true,
  dom_metadata: true,
  extension_bridge: 'supported'
};

function normalizeServerUrl(value) {
  const text = String(value || DEFAULT_SERVER_URL).trim();
  return (text || DEFAULT_SERVER_URL).replace(/\/+$/, '');
}

function normalizePairingCode(value) {
  const cleaned = String(value || '').toUpperCase().replace(/[^A-Z0-9]/g, '');
  return cleaned.length === 8 ? `${cleaned.slice(0, 4)}-${cleaned.slice(4)}` : cleaned;
}

function storageGet(keys) {
  return chrome.storage.local.get(keys);
}

function storageSet(value) {
  return chrome.storage.local.set(value);
}

async function getConfig() {
  const cfg = await storageGet([
    'serverUrl',
    'deviceToken',
    'connectorId',
    'connectorName',
    'lastStatus',
    'lastError'
  ]);
  return {
    serverUrl: normalizeServerUrl(cfg.serverUrl),
    deviceToken: cfg.deviceToken || '',
    connectorId: cfg.connectorId || 0,
    connectorName: cfg.connectorName || 'THG Chrome Extension',
    lastStatus: cfg.lastStatus || '',
    lastError: cfg.lastError || ''
  };
}

function queryTabs(query) {
  return chrome.tabs.query(query);
}

function getCookie(details) {
  return chrome.cookies.get(details);
}

async function collectFacebookState() {
  const [activeTabs, fbTabs, cookie] = await Promise.all([
    queryTabs({ active: true, currentWindow: true }),
    queryTabs({ url: ['https://facebook.com/*', 'https://*.facebook.com/*'] }),
    getCookie({ url: 'https://www.facebook.com', name: 'c_user' }).catch(() => null)
  ]);
  const active = activeTabs.find(t => /^https:\/\/([^/]+\.)?facebook\.com\//i.test(t.url || ''));
  const firstFb = active || fbTabs[0] || null;
  const currentUrl = firstFb?.url || '';
  const lower = currentUrl.toLowerCase();
  let streamStatus = firstFb ? 'facebook_login_required' : 'chrome_connected';
  if (lower.includes('checkpoint') || lower.includes('two_step')) {
    streamStatus = 'facebook_human_required';
  }
  if (cookie?.value) {
    streamStatus = 'facebook_logged_in';
  }
  return {
    currentUrl,
    fbUserId: cookie?.value || '',
    streamStatus,
    tab: firstFb
  };
}

async function pairConnector(serverUrl, code) {
  const normalized = normalizeServerUrl(serverUrl);
  const state = await collectFacebookState();
  const body = {
    code: normalizePairingCode(code),
    hostname: 'Chrome Extension',
    os: `${navigator.platform || 'unknown'} / ${navigator.userAgent || 'Chrome'}`,
    version: chrome.runtime.getManifest().version,
    kind: 'extension_connector',
    transport: 'chrome_extension',
    capabilities_json: JSON.stringify(CAPABILITIES),
    current_url: state.currentUrl,
    fb_user_id: state.fbUserId,
    stream_status: state.streamStatus
  };
  const res = await fetch(`${normalized}/api/connectors/pair`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
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
      throw new Error('Không xác nhận được mã kết nối. Vui lòng tạo mã mới trong Browser dashboard, kiểm tra THG server trùng với domain dashboard, rồi kết nối lại.');
    }
    throw new Error(`Kết nối thất bại (${res.status}): ${message}`);
  }
  const payload = await res.json();
  await storageSet({
    serverUrl: normalized,
    deviceToken: payload.device_token,
    connectorId: payload.connector?.id || 0,
    connectorName: payload.connector?.name || 'THG Chrome Extension',
    lastError: ''
  });
  await heartbeat();
  return payload;
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

async function sendHeartbeat(state) {
  const body = {
    hostname: 'Chrome Extension',
    os: `${navigator.platform || 'unknown'} / Chrome`,
    version: chrome.runtime.getManifest().version,
    kind: 'extension_connector',
    transport: 'chrome_extension',
    capabilities_json: JSON.stringify(CAPABILITIES),
    current_url: state.currentUrl,
    fb_user_id: state.fbUserId,
    stream_status: state.streamStatus
  };
  const res = await agentFetch('/api/agent/heartbeat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  });
  if (!res.ok) throw new Error(`heartbeat failed (${res.status})`);
}

async function sendChromeStatus(target, state) {
  const res = await agentFetch('/api/agent/chrome-status', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      account_id: target?.account_id || target?.accountId || 0,
      current_url: state.currentUrl,
      fb_user_id: state.fbUserId,
      stream_status: state.streamStatus
    })
  });
  if (!res.ok) throw new Error(`chrome status failed (${res.status})`);
}

async function fetchTargets() {
  const res = await agentFetch('/api/agent/browser-targets');
  if (!res.ok) return [];
  const payload = await res.json().catch(() => ({}));
  return Array.isArray(payload.targets) ? payload.targets : [];
}

function captureVisibleTab(windowId) {
  return chrome.tabs.captureVisibleTab(windowId, { format: 'jpeg', quality: 45 });
}

async function maybeSendScreenshot(target, state) {
  const accountId = target?.account_id || target?.accountId || 0;
  if (!accountId || !state.tab || !state.tab.active || !state.currentUrl) return;
  const imageData = await captureVisibleTab(state.tab.windowId);
  const res = await agentFetch('/api/agent/screenshot', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      account_id: accountId,
      image_data: imageData,
      current_url: state.currentUrl,
      fb_user_id: state.fbUserId,
      stream_status: state.streamStatus
    })
  });
  if (!res.ok) throw new Error(`screenshot failed (${res.status})`);
}

async function heartbeat() {
  const cfg = await getConfig();
  if (!cfg.deviceToken) return { paired: false };
  const state = await collectFacebookState();
  await sendHeartbeat(state);
  const targets = await fetchTargets().catch(() => []);
  const target = targets.find(t => t.account_id || t.accountId) || null;
  if (target) {
    await sendChromeStatus(target, state).catch(() => {});
    await maybeSendScreenshot(target, state).catch(() => {});
  }
  await storageSet({
    lastStatus: state.streamStatus,
    lastError: '',
    lastSeenAt: new Date().toISOString()
  });
  return { paired: true, status: state.streamStatus, currentUrl: state.currentUrl, fbUserId: state.fbUserId };
}

chrome.runtime.onInstalled.addListener(() => {
  chrome.alarms.create(HEARTBEAT_ALARM, { periodInMinutes: 1 });
});

chrome.runtime.onStartup.addListener(() => {
  chrome.alarms.create(HEARTBEAT_ALARM, { periodInMinutes: 1 });
});

chrome.alarms.onAlarm.addListener(alarm => {
  if (alarm.name === HEARTBEAT_ALARM) {
    heartbeat().catch(err => storageSet({ lastError: err.message || String(err) }));
  }
});

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  (async () => {
    try {
      if (message?.type === 'pair') {
        const payload = await pairConnector(message.serverUrl, message.code);
        sendResponse({ ok: true, connector: payload.connector });
        return;
      }
      if (message?.type === 'status') {
        const cfg = await getConfig();
        const live = await heartbeat().catch(() => null);
        sendResponse({ ok: true, config: cfg, live });
        return;
      }
      if (message?.type === 'forget') {
        await chrome.storage.local.clear();
        sendResponse({ ok: true });
        return;
      }
      if (message?.type === 'facebook_page_seen') {
        await heartbeat().catch(() => {});
        sendResponse({ ok: true });
        return;
      }
      sendResponse({ ok: false, error: 'unknown message' });
    } catch (err) {
      const text = err?.message || String(err);
      await storageSet({ lastError: text });
      sendResponse({ ok: false, error: text });
    }
  })();
  return true;
});
