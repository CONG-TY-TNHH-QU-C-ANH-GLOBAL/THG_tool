/*
 * THG Connector Extension — PR8A.1: Navigation-source watcher.
 *
 * A background ring buffer of chrome.webNavigation events for FB tabs. Its sole
 * job is to NAME who navigated the comment tab to the home feed — settling the
 * "FB redirect vs our own chrome.tabs code" question with ground truth instead
 * of inference.
 *
 * Per event we keep transitionType + transitionQualifiers + a kind discriminator:
 *   - kind="committed" + qualifiers includes "server_redirect" → FB server 3xx.
 *   - kind="committed" + qualifiers includes "client_redirect" → FB page JS/meta redirect.
 *   - kind="history"   (onHistoryStateUpdated)                  → FB SPA router pushState/replaceState.
 *   - kind="committed" transition "typed"/"auto_toplevel" with NO redirect
 *     qualifier, firing when no FB-initiated nav is expected           → OUR chrome.tabs call.
 *
 * Top frame only (frameId === 0). Requires the "webNavigation" manifest
 * permission; degrades to an empty trace if the API is unavailable.
 */
var THGNavWatch = globalThis.THGNavWatch || (() => {
  const RING_MAX = 120;
  const events = []; // { tabId, url, kind, transition, qualifiers, ts }

  function record(e, kind) {
    if (!e || e.frameId !== 0) return; // ignore subframes (ads, plugins, embeds)
    events.push({
      tabId: e.tabId,
      url: e.url || '',
      kind,
      transition: e.transitionType || '',
      qualifiers: Array.isArray(e.transitionQualifiers) ? e.transitionQualifiers.join(',') : '',
      ts: e.timeStamp || Date.now(),
    });
    if (events.length > RING_MAX) events.splice(0, events.length - RING_MAX);
  }

  if (typeof chrome !== 'undefined' && chrome.webNavigation) {
    try {
      const fbFilter = { url: [{ hostSuffix: 'facebook.com' }, { hostSuffix: 'fb.watch' }] };
      chrome.webNavigation.onCommitted.addListener(e => record(e, 'committed'), fbFilter);
      // FB is an SPA: a reset-to-home is often a pushState, which fires
      // onHistoryStateUpdated, NOT onCommitted. Capturing both is what makes
      // the "FB SPA router did it" case visible.
      chrome.webNavigation.onHistoryStateUpdated.addListener(e => record(e, 'history'), fbFilter);
    } catch (_) { /* webNavigation unavailable — empty trace */ }
  }

  // eventsFor returns the trace for one tab since a start timestamp, normalised
  // to {url, transition, qualifiers, kind, t_ms} (ms since start). A 50ms slack
  // on the lower bound absorbs clock skew between Date.now() and event.timeStamp.
  function eventsFor(tabId, sinceTs) {
    if (!tabId || !sinceTs) return [];
    return events
      .filter(ev => ev.tabId === tabId && ev.ts >= sinceTs - 50)
      .map(ev => ({
        url: ev.url,
        transition: ev.transition,
        qualifiers: ev.qualifiers,
        kind: ev.kind,
        t_ms: Math.max(0, Math.round(ev.ts - sinceTs)),
      }));
  }

  return { eventsFor };
})();
globalThis.THGNavWatch = THGNavWatch;
