/*
 * THG Connector Extension — PR8A.1: Navigation-source watcher.
 *
 * A background ring buffer of chrome.webNavigation events for FB tabs. Its sole
 * job is to NAME who navigated the comment tab to the home feed — settling the
 * "FB redirect vs our own chrome.tabs code" question with ground truth instead
 * of inference.
 *
 * PR8 nav timeline — all four top-frame webNavigation events are recorded so
 * the ROOT_CAUSE_REPORT can reconstruct the full sequence per attempt:
 *   - kind="before"    (onBeforeNavigate)       → a top-level load is STARTING.
 *   - kind="committed" (onCommitted)            → that load committed; carries
 *                                                 transitionType + qualifiers.
 *   - kind="completed" (onCompleted)            → the load finished (DOM ready).
 *   - kind="history"   (onHistoryStateUpdated)  → FB SPA router pushState/replaceState.
 *
 * Attribution (read off the committed event for the home/feed URL):
 *   - committed + qualifiers includes "server_redirect" → FB server 3xx.
 *   - committed + qualifiers includes "client_redirect" → FB page JS/meta redirect.
 *   - history                                            → FB SPA router reset.
 *   - committed transition "typed"/"auto_toplevel" with NO redirect qualifier,
 *     firing when no FB-initiated nav is expected         → OUR chrome.tabs call.
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
      // onBeforeNavigate: a top-level load is starting. Marks the LEADING edge
      // of a redirect — pairs with the committed event to show "where it was
      // heading vs where it landed".
      chrome.webNavigation.onBeforeNavigate.addListener(e => record(e, 'before'), fbFilter);
      chrome.webNavigation.onCommitted.addListener(e => record(e, 'committed'), fbFilter);
      // onCompleted: the load finished. Confirms the landing actually settled
      // (distinguishes "redirected and stayed" from "flickered through").
      chrome.webNavigation.onCompleted.addListener(e => record(e, 'completed'), fbFilter);
      // FB is an SPA: a reset-to-home is often a pushState, which fires
      // onHistoryStateUpdated, NOT onCommitted. Capturing all four is what makes
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
