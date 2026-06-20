// THGContentOutbound — outbound action FACADE. After Workstream A PR2–PR5 every action layer
// lives in its own module; this file only dispatches by type and re-exports the comment
// entrypoints. Public Chrome surface is exactly 4 methods (executeOutbound,
// executeCommentInFeed, probeRung2Click, executeCommentViaRung2) — comment_executor.js /
// bridge.js / channels read only these. _test stays Node-only via module.exports.
globalThis.THGContentOutbound = globalThis.THGContentOutbound || (() => {
  // Each layer is manifest-loaded before this file; guarded fallback for Node tests.
  const THGPosting = globalThis.THGPostingOutbound
    || (typeof require === 'function' ? require('./posting/posting_outbound.js') : null);
  if (!THGPosting) {
    throw new Error('THGPostingOutbound is required before outbound.js');
  }
  const THGInbox = globalThis.THGInboxOutbound
    || (typeof require === 'function' ? require('./inbox/inbox_outbound.js') : null);
  if (!THGInbox) {
    throw new Error('THGInboxOutbound is required before outbound.js');
  }
  const THGCommenting = globalThis.THGCommentingOutbound
    || (typeof require === 'function' ? require('./commenting/commenting_outbound.js') : null);
  if (!THGCommenting) {
    throw new Error('THGCommentingOutbound is required before outbound.js');
  }

  async function executeOutbound(message) {
    const content = String(message?.content || '').trim();
    if (!content) return { ok: false, error: 'outbox_content_empty' };
    if (content.length > 3000) return { ok: false, error: 'outbox_content_too_long' };
    const type = String(message?.type || '').trim().toLowerCase();
    // target_url is the SAME field outbox.js navigates the tab to via
    // chrome.tabs.update. Surfacing it to the comment executor lets us
    // pin the DOM search to the exact post the queue intended, instead
    // of the first comment button visible on the SPA-rendered page.
    const targetUrl = String(message?.target_url || message?.targetUrl || '').trim();
    // execution_id is the server-issued idempotency token. We do NOT
    // mutate it here; we just thread it through. The proof builder in
    // commentResult attaches it to proof.execution_id so the eventual
    // /sent or /failed POST body echoes it. Backend's
    // FinalizeOutboundAttempt CAS requires this to match the row's
    // current execution_id; replays and re-claim collisions are
    // rejected there.
    const executionId = String(message?.execution_id || message?.executionId || '').trim();
    // PR8A: thread the executing account id + the background navigation trace
    // (from/to/duration, attached by src/outbox.js before sendMessage) into the
    // comment executor so the NavDiagnostic it builds is complete.
    const navOpts = {
      accountId: Number(message?.account_id || message?.accountId || 0) || 0,
      navTrace: message?.nav_trace || null,
      outboundId: Number(message?.id || message?.outbound_id || 0) || 0,
    };
    if (type === 'comment') return THGCommenting.executeComment(content, targetUrl, executionId, navOpts);
    if (type === 'inbox') return THGInbox.executeInbox(content, executionId);
    if (type === 'group_post' || type === 'profile_post') return THGPosting.executePost(content, executionId);
    return { ok: false, error: `unsupported_outbox_type:${type}` };
  }

  // Public 4-key surface. The three comment entrypoints delegate to THGCommentingOutbound;
  // executeOutbound's comment branch does too. Shape unchanged for all external callers.
  const api = {
    executeOutbound,
    executeCommentInFeed: THGCommenting.executeCommentInFeed,
    probeRung2Click: THGCommenting.probeRung2Click,
    executeCommentViaRung2: THGCommenting.executeCommentViaRung2,
  };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  return api;
})();
