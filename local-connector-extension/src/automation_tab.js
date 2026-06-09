// Automation Tab lifecycle (incident PR-2 + PR-2.1 hardening). One connector/account
// drives ALL leads through a SINGLE persistent automation tab instead of opening a
// new tab per comment. Kept out of the commands.js/outbox.js legacy files: they only
// CALL these helpers. The user-owned window is never closed/resized (window-policy).
//
// PR-2.1: the remembered tab id is PERSISTED to chrome.storage.session (survives an
// MV3 service-worker restart; clears when the browser closes), with a fallback to
// chrome.storage.local. When the tab is gone the stored id is cleared.
var THGAutomationTab = globalThis.THGAutomationTab || (() => {
  const KEY = 'thg_automation_tab_id';

  function store() {
    try { if (chrome.storage && chrome.storage.session) return chrome.storage.session; } catch (_) {}
    try { if (chrome.storage && chrome.storage.local) return chrome.storage.local; } catch (_) {}
    return null;
  }

  async function getRemembered() {
    const s = store();
    if (!s) return 0;
    try { const r = await s.get(KEY); return Number(r && r[KEY]) || 0; } catch (_) { return 0; }
  }

  async function remember(tabId) {
    const s = store();
    if (!s) return;
    try {
      if (tabId) await s.set({ [KEY]: tabId });
      else await s.remove(KEY);
    } catch (_) { /* best-effort */ }
  }

  async function clear() { return remember(0); }

  async function isAlive(tabId) {
    const id = tabId || (await getRemembered());
    if (!id) return false;
    try { const t = await chrome.tabs.get(id); return !!t && t.id === id; } catch (_) { return false; }
  }

  async function navigate(tabId, url) {
    await chrome.tabs.update(tabId, { url, active: true });
    try { return await chrome.tabs.get(tabId); } catch (_) { return { id: tabId }; }
  }

  // reuseIfAlive: when the persisted automation tab still exists, navigate it to url
  // and return the tab; otherwise CLEAR the stale id and return null so the caller
  // creates a fresh tab (and remembers it). This is what turns "10 comments = 10
  // tabs" into one tab, surviving service-worker restarts.
  async function reuseIfAlive(url) {
    const id = await getRemembered();
    if (!id) return null;
    if (!(await isAlive(id))) { await clear(); return null; }
    return navigate(id, url);
  }

  return { remember, getRemembered, isAlive, navigate, reuseIfAlive, clear };
})();

if (typeof module !== 'undefined' && module.exports) module.exports = THGAutomationTab;
