// Execution idempotency (incident root fix). The background outbox uses at-least-once
// delivery: when chrome.tabs.sendMessage drops mid-execution (Facebook SPA reset
// closes the message channel during a 10-20s comment run), it re-injects the content
// scripts and RESENDS the same command. Without idempotency that resend re-runs the
// SAME execution_id → a second composer insert → the doubled "A+A" comment.
//
// THGExecDedup tracks the in-flight + recently-completed execution_ids so the bridge
// can reject a resend. Pure (clock injected via nowMs) so it is unit-testable; kept
// out of bridge.js, which only WIRES it.
var THGExecDedup = globalThis.THGExecDedup || (() => {
  const WINDOW_MS = 90000; // a resend lands within seconds; 90s is a safe ceiling
  let activeId = '';
  const recent = new Map(); // execId -> completedAt(ms)

  function isDuplicate(execId, nowMs) {
    if (!execId) return false;
    if (execId === activeId) return true;
    const done = recent.get(execId);
    return done != null && (nowMs - done) < WINDOW_MS;
  }

  function markActive(execId) { if (execId) activeId = execId; }

  function markDone(execId, nowMs) {
    activeId = '';
    if (!execId) return;
    recent.set(execId, nowMs);
    if (recent.size > 50) {
      for (const [k, t] of recent) if (nowMs - t > WINDOW_MS) recent.delete(k);
    }
  }

  return { isDuplicate, markActive, markDone, _reset() { activeId = ''; recent.clear(); } };
})();

if (typeof module !== 'undefined' && module.exports) module.exports = THGExecDedup;
