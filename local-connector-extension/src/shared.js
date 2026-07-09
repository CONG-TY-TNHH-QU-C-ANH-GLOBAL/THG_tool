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
    'platforms/facebook/crawl_progress.js',
    'platforms/facebook/crawl_post_identity.js',
    'platforms/facebook/crawl_direct_post.js',
    'platforms/facebook/crawl_result.js',
    'platforms/facebook/crawl_pacing.js',
    'content/crawl.js',
    'content/proof.js',
    'content/navreport.js',
    'content/forensics.js',
    'content/facebook/commenting/constants/comment_constants.js',
    'content/facebook/commenting/composer/comment_composer_guard.js',
    'content/execution_dedup.js',
    'content/facebook/commenting/submit/comment_submit.js',
    'content/facebook/commenting/state-machine/comment_state_machine.js',
    'content/facebook/commenting/composer/comment_composer.js',
    'content/facebook/commenting/button/comment_button.js',
    'content/facebook/outbound/dom/outbound_dom.js',
    'content/facebook/outbound/posting/posting_outbound.js',
    'content/facebook/outbound/inbox/inbox_outbound.js',
    'content/facebook/outbound/commenting/target/post_id.js',
    'content/facebook/outbound/commenting/target/surface.js',
    'content/facebook/outbound/commenting/target/article.js',
    'content/facebook/outbound/commenting/target/composer.js',
    'content/facebook/outbound/commenting/commenting_target.js',
    'content/facebook/outbound/commenting/commenting_diag.js',
    'content/facebook/outbound/commenting/execute/result.js',
    'content/facebook/outbound/commenting/execute/direct_gates.js',
    'content/facebook/outbound/commenting/execute/direct.js',
    'content/facebook/outbound/commenting/execute/feed_gates.js',
    'content/facebook/outbound/commenting/execute/feed.js',
    'content/facebook/outbound/commenting/execute/rung2.js',
    'content/facebook/outbound/commenting/commenting_outbound.js',
    'content/facebook/outbound/outbound.js',
    'content/reverify.js',
    'content/facebook/commenting/executor/comment_executor.js',
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

  // fetchWithTimeout wraps fetch with an AbortController deadline so a stalled or
  // black-holed request can never leave an awaited promise unsettled — the root
  // cause of the connector popup hanging forever on "Verifying..." (Sprint 4).
  // On timeout it aborts and throws a tagged Error (name === 'TimeoutError') so
  // callers map it to an operator-facing message. It performs NO retry: a
  // consumed pairing code must never be replayed.
  const DEFAULT_FETCH_TIMEOUT_MS = 20000;
  async function fetchWithTimeout(url, options = {}, timeoutMs = DEFAULT_FETCH_TIMEOUT_MS) {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    try {
      return await fetch(url, { ...options, signal: controller.signal });
    } catch (err) {
      if (err && err.name === 'AbortError') {
        const timeoutErr = new Error('request timed out');
        timeoutErr.name = 'TimeoutError';
        throw timeoutErr;
      }
      throw err;
    } finally {
      clearTimeout(timer);
    }
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
    fetchWithTimeout,
    injectContentScripts,
    isFacebookUrl,
    normalizePairingCode,
    normalizeServerUrl,
    storageGet,
    storageSet
  };
})();
globalThis.THGShared = THGShared;
