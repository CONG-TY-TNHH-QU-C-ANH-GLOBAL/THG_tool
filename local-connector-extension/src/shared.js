var THGShared = globalThis.THGShared || (() => {
  const DEFAULT_SERVER_URL = 'https://sale.thgfulfill.com';
  const HEARTBEAT_ALARM = 'thg-heartbeat';
  const FACEBOOK_HOME = 'https://www.facebook.com/';
  const AUTO_FOCUS_FACEBOOK_TAB = false;
  // FALLBACK RE-INJECTION SET. injectContentScripts() feeds this list to
  // chrome.scripting.executeScript when a tab has no live content script (the
  // sendMessage "Receiving end does not exist" catch paths in commands.js /
  // outbox.js / reverify.js / facebook-state.js). It MUST be a byte-for-byte,
  // SAME-ORDER mirror of manifest.json content_scripts[0].js — otherwise a
  // re-injected tab loads bridge.js while its dependencies (THGExecDedup,
  // THGCommentExecutor, THGCommentSM, the comment chain, forensics, reverify)
  // are absent, and a comment delivered on the fallback path fails with a
  // ReferenceError / outbound_not_ready (the "blind agent needs 2-3 attempts"
  // symptom). H-1: the two lists had drifted (9 vs 18). The lockstep is now
  // enforced by src/content_files_sync.test.mjs, which fails closed on any
  // divergence. Each content module is guarded by `globalThis.X || (…)`, so
  // re-running an already-present script is a harmless no-op.
  const CONTENT_FILES = [
    'content/shared.js',
    'content/meta.js',
    'content/commands.js',
    'content/crawl.js',
    'content/proof.js',
    'content/navreport.js',
    'content/forensics.js',
    'content/comment_composer_guard.js',
    'content/execution_dedup.js',
    'content/comment_submit.js',
    'content/comment_state_machine.js',
    'content/comment_composer.js',
    'content/comment_button.js',
    'content/outbound.js',
    'content/reverify.js',
    'content/comment_executor.js',
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
  // Heartbeat reports this alongside the manifest version so the backend
  // version gate (PR-4) can distinguish stable/beta builds.
  const RELEASE_CHANNEL = 'stable';

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
    RELEASE_CHANNEL,
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
