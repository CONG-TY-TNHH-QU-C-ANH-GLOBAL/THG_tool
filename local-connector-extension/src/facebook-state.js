var THGFacebookState = globalThis.THGFacebookState || (() => {
  function queryTabs(query) {
    return chrome.tabs.query(query);
  }

  // ensureBrowserProfileId returns a STABLE per-Chrome-profile UUID. chrome
  // .storage.local is scoped to the profile, so this id distinguishes profiles on
  // the same machine (PR-C). Generated once, then reused forever for this profile.
  async function ensureBrowserProfileId() {
    try {
      const got = await chrome.storage.local.get('browserProfileId');
      if (got && got.browserProfileId) return got.browserProfileId;
      const id = (crypto && crypto.randomUUID) ? crypto.randomUUID()
        : `p_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 10)}`;
      await chrome.storage.local.set({ browserProfileId: id });
      return id;
    } catch {
      return '';
    }
  }

  async function collectMetaFromTab(tab) {
    if (!tab?.id || !THGShared.isFacebookUrl(tab.url)) return {};
    try {
      const res = await chrome.tabs.sendMessage(tab.id, { type: 'thg_collect_meta' });
      return res?.meta || {};
    } catch {
      try {
        await THGShared.injectContentScripts(tab.id);
        const res = await chrome.tabs.sendMessage(tab.id, { type: 'thg_collect_meta' });
        return res?.meta || {};
      } catch {
        return {};
      }
    }
  }

  async function collectFacebookState(preferredTabId = 0) {
    const [activeTabs, fbTabs, cookie] = await Promise.all([
      queryTabs({ active: true, currentWindow: true }),
      queryTabs({ url: ['https://facebook.com/*', 'https://*.facebook.com/*'] }),
      chrome.cookies.get({ url: 'https://www.facebook.com', name: 'c_user' }).catch(() => null)
    ]);
    const active = activeTabs.find(t => THGShared.isFacebookUrl(t.url));
    const preferred = preferredTabId ? fbTabs.find(t => t.id === preferredTabId) : null;
    const firstFb = preferred || active || fbTabs[0] || null;
    const currentUrl = firstFb?.url || '';
    const lower = currentUrl.toLowerCase();
    let streamStatus = firstFb ? 'facebook_login_required' : 'chrome_connected';
    if (lower.includes('checkpoint') || lower.includes('two_step')) {
      streamStatus = 'facebook_human_required';
    }
    if (cookie?.value) {
      streamStatus = 'facebook_logged_in';
    }
    const meta = await collectMetaFromTab(firstFb);
    const browserProfileId = await ensureBrowserProfileId();
    // PR-B (B3): identity TRUTH is the c_user cookie. Report HOW confident /
    // where-from so the backend readiness (PR-D) + health board (PR-E) can show
    // "identity verified vs unknown" without trusting the (cosmetic) display name.
    const hasCUser = !!cookie?.value;
    return {
      currentUrl,
      fbUserId: cookie?.value || '',
      fbDisplayName: meta.fb_display_name || '',
      fbUsername: meta.fb_username || '',
      fbProfileUrl: meta.fb_profile_url || '',
      loginEmail: meta.login_email || '',
      streamStatus,
      identityConfidence: hasCUser ? 'high' : 'none',
      identityExtractionMethod: hasCUser ? 'cookie_c_user' : 'none',
      identityLastVerifiedAt: hasCUser ? new Date().toISOString() : '',
      browserProfileId,
      tab: firstFb
    };
  }

  function sameFacebookDestination(current, next) {
    try {
      const a = new URL(current || '');
      const b = new URL(next || '');
      return a.hostname === b.hostname && a.pathname.replace(/\/+$/, '') === b.pathname.replace(/\/+$/, '');
    } catch {
      return false;
    }
  }

  async function ensureFacebookTabVisible(url = THGShared.FACEBOOK_HOME, options = {}) {
    const focus = Boolean(options.focus);
    const fbTabs = await queryTabs({ url: ['https://facebook.com/*', 'https://*.facebook.com/*'] });
    let tab = fbTabs.find(t => t.active) || fbTabs[0] || null;
    if (!tab) {
      tab = await chrome.tabs.create({ url, active: focus });
    } else if (tab.id) {
      const update = {};
      if (url && !sameFacebookDestination(tab.url, url)) {
        update.url = url;
      }
      if (focus) {
        update.active = true;
      }
      if (Object.keys(update).length > 0) {
        tab = await chrome.tabs.update(tab.id, update).catch(() => tab);
      }
      if (focus && tab.windowId) {
        await chrome.windows.update(tab.windowId, { focused: true }).catch(() => {});
      }
    }
    if (tab?.id) {
      tab = await waitForTabReady(tab.id).catch(() => tab);
    }
    await THGShared.delay(focus ? 600 : 250);
    return collectFacebookState(tab?.id || 0);
  }

  async function waitForTabReady(tabId, timeoutMs = 15000) {
    const started = Date.now();
    while (Date.now() - started < timeoutMs) {
      const tab = await chrome.tabs.get(tabId).catch(() => null);
      if (tab?.status === 'complete') {
        await THGShared.delay(500);
        return tab;
      }
      await THGShared.delay(250);
    }
    return chrome.tabs.get(tabId).catch(() => null);
  }

  function chooseTarget(targets, fbUserId) {
    if (!Array.isArray(targets) || targets.length === 0) return null;
    if (fbUserId) {
      const sameFbUser = targets.find(t => String(t.fb_user_id || t.fbUserId || '') === String(fbUserId));
      if (sameFbUser) return sameFbUser;
    }
    return targets.find(t => !(t.fb_user_id || t.fbUserId)) || targets[0];
  }

  return {
    chooseTarget,
    collectFacebookState,
    ensureFacebookTabVisible,
    waitForTabReady
  };
})();
globalThis.THGFacebookState = THGFacebookState;
