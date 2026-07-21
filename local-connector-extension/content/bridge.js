if (!globalThis.THGContentBridgeInstalled) {
globalThis.THGContentBridgeInstalled = true;

// CONTENT-SCRIPT EXECUTION LOCK (PRIORITY A, tier 2).
//
// The background already enforces a per-tab lock when dispatching
// outbound work (src/outbox.js tabExecutionLocks). This is the
// matching lock on the receiving end so:
//
//   - a runaway background instance that bypasses its own lock cannot
//     race two mutate-class commands on the same DOM
//   - any future code path that calls chrome.tabs.sendMessage with a
//     mutate command (crawl invoker, ad-hoc tools, debug shortcuts)
//     gets serialised here automatically
//
// We do NOT lock read-class commands (thg_collect_meta is a pure
// observation called by heartbeat polling). Locking those would
// stale-out the meta tab state during a 20–30s outbound execution
// for no safety benefit. Mutate-class types are listed explicitly
// below so the lock surface is auditable.
const MUTATING_COMMAND_TYPES = new Set(['thg_execute_outbound', 'thg_execute_command', 'thg_comment_in_group_feed', 'thg_comment_via_rung2']);

// activeMutation is the currently-running mutate-class command's
// promise, or null when the content script is idle. Stored at the
// script scope (not per-frame map) because each Facebook tab has at
// most one main-frame content script — the tab IS the unit.
let activeMutation = null;

async function thgExecuteCommand(command) {
  const basic = THGContentCommands.executeBasicCommand(command);
  if (basic) return basic;
  const payload = typeof command.payload_json === 'string' ? JSON.parse(command.payload_json || '{}') : (command.payload_json || {});
  if (String(command.type || '').toLowerCase() === 'crawl') {
    // Support both old flat-task format and new ConnectorCrawlEnvelope format
    // where the task lives under payload.task.
    const task = payload?.task || payload;
    // Forward the expected URL so the content script can refuse to scrape
    // when Facebook silently redirected the tab (login wall, checkpoint).
    const expectedUrl = payload?.navigate_to
      || task?.crawl_plan?.sources?.[0]?.url
      || '';
    const gate = payload?.market_signal_gate || task?.extras?.market_signal_gate || null;
    const userPrompt = payload?.user_prompt || task?.extras?.user_prompt || '';
    const accountId = command?.account_id || command?.accountId || 0;
    const result = await THGContentCrawl.crawlVisibleFacebookPosts(task, expectedUrl, accountId);
    // Echo gate + user prompt back to the server so the crawl-result endpoint
    // applies the same Brain-derived gating and anchors the AI classifier to
    // the operator's current goal without re-reading org context.
    if (result?.ok && result?.crawl_result) {
      if (gate) result.crawl_result.market_signal_gate = gate;
      if (userPrompt) result.crawl_result.user_prompt = userPrompt;
    }
    return result;
  }
  return { ok: false, error: `Unsupported command type: ${command.type}` };
}

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  (async () => {
    try {
      const type = message?.type;

      // Read-class: no lock, runs concurrently with whatever mutate
      // is in flight (or with itself, harmlessly).
      if (type === 'thg_collect_meta') {
        sendResponse({ ok: true, meta: THGContentMeta.collectFacebookMeta() });
        return;
      }

      // Rung-2 navigation probe: click-navigate toward the permalink and
      // return immediately (background measures the trajectory). Diagnostic,
      // non-mutating to FB state (no comment typed) — not lock-gated.
      if (type === 'thg_nav_probe_rung2') {
        sendResponse(THGContentOutbound.probeRung2Click(message.message || {}));
        return;
      }

      // Async reverify: read-only re-check of a posted comment (no compose/submit). Yields
      // if a mutate command is mid-flight so it never fights the composer for the tab; the
      // connector leaves the row pending and retries next cycle. See specs/domains/facebook-sales-intelligence/features/comment-automation/technical.md.
      if (type === 'thg_reverify_comment') {
        if (activeMutation) { sendResponse({ ok: false, error: 'tab_busy_executing' }); return; }
        sendResponse(await THGCommentExecutor.execute('thg_reverify_comment', message.message || {}));
        return;
      }

      // Mutate-class: serialise via activeMutation. A second
      // mutate command arriving while one is in flight is REJECTED
      // immediately — the caller (background outbox) is expected to
      // retry on its next poll cycle. We don't queue because an
      // unbounded queue inside the content script outlives the SW
      // and can re-execute messages whose backend status has moved
      // on. Reject + retry keeps the source of truth at the server.
      if (MUTATING_COMMAND_TYPES.has(type)) {
        // Idempotency (incident root fix): the background uses at-least-once delivery
        // and RESENDS a mutate command when the message channel drops mid-execution
        // (FB SPA reset). Suppress a resend of an in-flight / just-completed
        // execution_id so it cannot re-insert the comment → the doubled "A+A".
        const execId = String((message && message.message && message.message.execution_id) || '').trim();
        if (THGExecDedup.isDuplicate(execId, Date.now())) {
          sendResponse({ ok: false, error: 'duplicate_execution_suppressed', execution_id: execId });
          return;
        }
        if (activeMutation) {
          sendResponse({ ok: false, error: 'tab_busy_executing' });
          return;
        }
        THGExecDedup.markActive(execId);
        const work = (async () => {
          // Single comment entrypoint (Browser Automation Kit extraction).
          if (type === 'thg_execute_outbound' || type === 'thg_comment_in_group_feed' || type === 'thg_comment_via_rung2') {
            return THGCommentExecutor.execute(type, message.message || {});
          }
          return thgExecuteCommand(message.command || {});
        })();
        activeMutation = work.finally(() => { activeMutation = null; THGExecDedup.markDone(execId, Date.now()); });
        sendResponse(await activeMutation);
        return;
      }

      sendResponse({ ok: false, error: 'unknown content message' });
    } catch (err) {
      sendResponse({ ok: false, error: err?.message || String(err) });
    }
  })();
  return true;
});
}
