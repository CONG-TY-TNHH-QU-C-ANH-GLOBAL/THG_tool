// Async comment reverify — connector poller (spec: specs/COMMENT_ASYNC_REVERIFY.md, PR-A).
// Claims due reverify jobs from the backend, re-opens each target post in the existing
// logged-in FB tab, asks the content script to search for the comment, and reports the
// verdict back. The backend appends the append-only correction on a positive match. Runs
// AFTER the outbox poller in the heartbeat cycle so the two never drive the tab at once.
var THGReverify = globalThis.THGReverify || (() => {
  let processing = null;

  async function claim() {
    const res = await THGApi.agentFetch('/api/agent/reverify/claim?limit=3');
    return (res && res.reverifies) || [];
  }

  async function report(id, found, permalink, notes) {
    await THGApi.agentFetch('/api/agent/reverify/result', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        id,
        found: !!found,
        comment_permalink: permalink || '',
        notes: notes || '',
      }),
    });
  }

  async function reverifyOne(job) {
    const url = String((job && job.target_url) || '').trim();
    // Only a DEFINITIVE search verdict (result.ok===true) is reported. A missing target,
    // no tab, busy composer, or content error leaves the row PENDING for the next cycle —
    // we must never mark a comment not_found just because we couldn't look this time.
    if (!url) return;
    const state = await THGFacebookState.ensureFacebookTabVisible(url, { focus: false });
    const tabId = state && state.tabId;
    if (!tabId) return;

    let result;
    try {
      result = await chrome.tabs.sendMessage(tabId, { type: 'thg_reverify_comment', message: job });
    } catch (e) {
      return; // content not reachable — leave pending
    }
    if (!result || result.ok !== true) return; // busy / not-ready — leave pending
    await report(job.id, !!result.found, result.comment_permalink || '', result.notes || '');
  }

  async function processOnce() {
    const jobs = await claim();
    for (const job of jobs) {
      // Best-effort per job: a failure leaves the row pending for the next cycle.
      try { await reverifyOne(job); } catch (e) { /* swallow; next poll retries */ }
    }
  }

  // process is guarded like the outbox poller so overlapping heartbeats don't double-run.
  function process() {
    if (processing) return processing;
    processing = processOnce().finally(() => { processing = null; });
    return processing;
  }

  return { process };
})();
globalThis.THGReverify = THGReverify;
if (typeof module !== 'undefined' && module.exports) module.exports = THGReverify;
