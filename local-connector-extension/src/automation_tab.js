// Automation Tab lifecycle (incident PR-2). One connector/account drives ALL leads
// through a SINGLE persistent automation tab instead of opening a new tab per
// comment. Kept out of the commands.js/outbox.js legacy files: they only CALL these
// helpers. The user-owned window is never closed/resized (see window-policy.js).
var THGAutomationTab = globalThis.THGAutomationTab || (() => {
  let rememberedTabId = 0;

  function remember(tabId) { rememberedTabId = tabId || 0; }
  function getRemembered() { return rememberedTabId; }

  async function isAlive(tabId) {
    const id = tabId || rememberedTabId;
    if (!id) return false;
    try {
      const t = await chrome.tabs.get(id);
      return !!t && t.id === id;
    } catch (_) {
      return false;
    }
  }

  async function navigate(tabId, url) {
    await chrome.tabs.update(tabId, { url, active: true });
    try { return await chrome.tabs.get(tabId); } catch (_) { return { id: tabId }; }
  }

  // reuseIfAlive: when the remembered automation tab still exists, navigate it to
  // url and return the tab; otherwise return null so the caller creates a fresh tab
  // (and remembers it). This is what turns "10 comments = 10 tabs" into one tab.
  async function reuseIfAlive(url) {
    const id = getRemembered();
    if (!(await isAlive(id))) return null;
    return navigate(id, url);
  }

  return { remember, getRemembered, isAlive, navigate, reuseIfAlive };
})();

if (typeof module !== 'undefined' && module.exports) module.exports = THGAutomationTab;
