var THGShared = globalThis.THGShared || (() => {
  const DEFAULT_SERVER_URL = 'https://sale.thgfulfill.com';
  const HEARTBEAT_ALARM = 'thg-heartbeat';
  const FACEBOOK_HOME = 'https://www.facebook.com/';
  const AUTO_FOCUS_FACEBOOK_TAB = false;
  const CONTENT_FILES = [
    'content/shared.js',
    'content/meta.js',
    'content/commands.js',
    'content/crawl.js',
    'content/outbound.js',
    'content/bridge.js',
    'content.js'
  ];
  const CAPABILITIES = {
    provider: 'chrome_extension_facebook',
    chrome_extension: true,
    browser_control: 'facebook_tab_extension',
    screen_capture: 'active_facebook_tab_only',
    dashboard_stream: true,
    dom_metadata: true,
    command_polling: true,
    crawl_visible_posts: true,
    input_relay: true,
    outbox_polling: true,
    outbound_executor: true,
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

  function isFacebookUrl(url) {
    return /^https:\/\/([^/]+\.)?facebook\.com\//i.test(String(url || ''));
  }

  function storageGet(keys) {
    return chrome.storage.local.get(keys);
  }

  function storageSet(value) {
    return chrome.storage.local.set(value);
  }

  function delay(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
  }

  async function injectContentScripts(tabId) {
    if (!tabId) throw new Error('Facebook tab is not ready');
    await chrome.scripting.executeScript({ target: { tabId }, files: CONTENT_FILES });
  }

  return {
    AUTO_FOCUS_FACEBOOK_TAB,
    CAPABILITIES,
    CONTENT_FILES,
    DEFAULT_SERVER_URL,
    FACEBOOK_HOME,
    HEARTBEAT_ALARM,
    delay,
    injectContentScripts,
    isFacebookUrl,
    normalizePairingCode,
    normalizeServerUrl,
    storageGet,
    storageSet
  };
})();
globalThis.THGShared = THGShared;
