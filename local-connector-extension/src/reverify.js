// Async comment reverify — connector poller (spec: specs/COMMENT_ASYNC_REVERIFY.md, PR-A).
// Claims due reverify jobs, re-opens each target post in the existing logged-in FB tab, asks
// the content script to search for the comment, and reports the verdict. The backend appends
// the append-only correction on a positive match. Runs AFTER the outbox poller so the two
// never drive the tab at once.
//
// FAIL-SAFE (PR-A fix): EVERY claimed job reports a terminal verdict (verified / not_found /
// error) with a clear reason — a job must NEVER stay pending+claimed forever. Stage logs
// (reverify_*) trace the pipeline in the connector console.
var THGReverify = globalThis.THGReverify || (() => {
  let processing = null;

  function log(stage, extra) {
    try { console.log('[THG reverify] ' + stage, extra || ''); } catch (e) { /* ignore */ }
  }

  async function claim() {
    const res = await THGApi.agentFetch('/api/agent/reverify/claim?limit=3');
    return (res && res.reverifies) || [];
  }

  // report ALWAYS fires for a claimed job so attempted_at is set and the row leaves pending.
  async function report(id, payload) {
    log('reverify_result_post_started', { id });
    try {
      await THGApi.agentFetch('/api/agent/reverify/result', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          id,
          found: !!payload.found,
          comment_permalink: payload.comment_permalink || '',
          notes: payload.notes || '',
          error: payload.error || '',
        }),
      });
      log('reverify_result_posted', { id, found: !!payload.found, error: payload.error || '' });
    } catch (e) {
      // The result POST itself failed — the backend's stale-claim lease will reclaim it.
      log('reverify_result_post_failed', { id, error: (e && e.message) || String(e) });
      try { THGShared.storageSet({ lastError: 'reverify_result_post_failed: ' + ((e && e.message) || '') }); } catch (_) { /* ignore */ }
    }
  }

  // sendToContent injects + retries like the outbox path: an existing FB tab may still hold an
  // old content script without the reverify handler until re-injected.
  async function sendToContent(tabId, job) {
    try {
      return await chrome.tabs.sendMessage(tabId, { type: 'thg_reverify_comment', message: job });
    } catch (e) {
      await THGShared.injectContentScripts(tabId);
      return await chrome.tabs.sendMessage(tabId, { type: 'thg_reverify_comment', message: job });
    }
  }

  async function reverifyOne(job) {
    const url = String((job && job.target_url) || '').trim();
    if (!url) { await report(job.id, { error: 'no_target_url' }); return; }
    log('reverify_worker_started', { id: job.id, url });

    let tabId = 0;
    try {
      const state = await THGFacebookState.ensureFacebookTabVisible(url, { focus: false });
      tabId = state && state.tab && state.tab.id; // state shape: { tab: <chrome.tabs.Tab>, ... }
    } catch (e) {
      await report(job.id, { error: 'nav_failed:' + ((e && e.message) || 'unknown') });
      return;
    }
    if (!tabId) { await report(job.id, { error: 'no_tab_after_nav' }); return; }
    log('reverify_tab_opened', { id: job.id, tabId });

    let result;
    try {
      result = await sendToContent(tabId, job);
    } catch (e) {
      await report(job.id, { error: 'content_unreachable:' + ((e && e.message) || 'no_response') });
      return;
    }
    if (!result || result.ok !== true) {
      await report(job.id, { error: (result && result.error) || 'reverify_no_result' });
      return;
    }
    log('reverify_content_script_loaded', { id: job.id, found: !!result.found });
    await report(job.id, {
      found: !!result.found,
      comment_permalink: result.comment_permalink || '',
      notes: result.notes || '',
    });
  }

  async function processOnce() {
    let jobs = [];
    try {
      jobs = await claim();
    } catch (e) {
      log('reverify_worker_error', { stage: 'claim', error: (e && e.message) || String(e) });
      return;
    }
    if (jobs.length) log('reverify_claim_received', { count: jobs.length });
    for (const job of jobs) {
      try {
        await reverifyOne(job);
      } catch (e) {
        log('reverify_worker_error', { id: job && job.id, error: (e && e.message) || String(e) });
        // Fail-safe: even a thrown worker reports an error so the job leaves pending.
        try { await report(job.id, { error: 'worker_exception:' + ((e && e.message) || 'unknown') }); } catch (_) { /* ignore */ }
      }
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
