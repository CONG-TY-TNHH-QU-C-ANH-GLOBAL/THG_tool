var THGFacebookState = globalThis.THGFacebookState || (() => {
  function queryTabs(query) {
    return chrome.tabs.query(query);
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

  async function collectFacebookState() {
    const [activeTabs, fbTabs, cookie] = await Promise.all([
      queryTabs({ active: true, currentWindow: true }),
      queryTabs({ url: ['https://facebook.com/*', 'https://*.facebook.com/*'] }),
      chrome.cookies.get({ url: 'https://www.facebook.com', name: 'c_user' }).catch(() => null)
    ]);
    const active = activeTabs.find(t => THGShared.isFacebookUrl(t.url));
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
    const meta = await collectMetaFromTab(firstFb);
    return {
      currentUrl,
      fbUserId: cookie?.value || '',
      fbDisplayName: meta.fb_display_name || '',
      fbUsername: meta.fb_username || '',
      fbProfileUrl: meta.fb_profile_url || '',
      loginEmail: meta.login_email || '',
      streamStatus,
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

  async function ensureFacebookTabVisible(url = THGShared.FACEBOOK_HOME) {
    const fbTabs = await queryTabs({ url: ['https://facebook.com/*', 'https://*.facebook.com/*'] });
    let tab = fbTabs.find(t => t.active) || fbTabs[0] || null;
    if (!tab) {
      await chrome.tabs.create({ url, active: true });
    } else if (tab.id) {
      if (url && !sameFacebookDestination(tab.url, url)) {
        await chrome.tabs.update(tab.id, { url, active: true }).catch(() => {});
      } else {
        await chrome.tabs.update(tab.id, { active: true }).catch(() => {});
      }
      if (tab.windowId) {
        await chrome.windows.update(tab.windowId, { focused: true }).catch(() => {});
      }
    }
    await THGShared.delay(1000);
    return collectFacebookState();
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
