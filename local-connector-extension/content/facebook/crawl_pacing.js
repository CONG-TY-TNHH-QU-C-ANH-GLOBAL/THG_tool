// THG Facebook crawl — pacing & stop policy. Pure decision helpers extracted
// from content/crawl.js (PR-C2) so pacing is testable and tunable in one place.
// No DOM/window/location access: all inputs (pass, counters, innerHeight) are
// passed in explicit state objects. crawl.js stays the DOM orchestrator.
//
// SAFETY: these helpers only decide waits / scroll step / when to STOP. They do
// NOT bypass, solve, or wait-out any checkpoint/login/risk wall — that is
// handled by the risk probe in crawl.js which breaks the loop before pacing is
// consulted. Constants are named for their safety intent.
//
// PR-C2 commit 1: behavior-IDENTICAL to the previous inline constants. The
// conservative tuning lands in commit 2 (same file, same signatures).
globalThis.THGCrawlPacing = globalThis.THGCrawlPacing || (() => {
  const PACING = Object.freeze({
    // Pause after a scroll before re-reading the DOM, so FB can react.
    PRE_GRAB_PAUSE_MS: 300,
    // Cautious lazy-load waits: FB loads the next feed batch within this window.
    // Kept generous on barren/uncertain passes so we never out-run lazy-load.
    EARLY_WAIT_MS: 2200,
    DEEP_WAIT_MS: 3600,
    DEEP_PASS_THRESHOLD: 8, // pass index at/after which the deep wait applies

    // Scroll geometry (unchanged from the original inline math).
    SCROLL_VIEWPORT_FRACTION: 0.95,
    SCROLL_MIN_STEP_PX: 700,
    SCROLL_BIG_PUSH_EVERY: 6, // every Nth pass do a double-size push

    // Stop thresholds — evidence required before ending the crawl.
    NO_PROGRESS_STAGNANT_STOP: 10, // consecutive stagnant passes → no_progress
    NO_NEW_AFTER_SCROLL_STOP: 16,  // passes since last new item → no_new_items_after_scroll

    // Pass-budget shape.
    MAX_PASSES_MIN: 70,
    MAX_PASSES_MAX: 260,
    MAX_PASSES_PER_ITEM: 3,
    MIN_STOP_FLOOR: 18,
    MIN_STOP_FRACTION: 0.7,
  });

  // Pass budget for a target item count.
  function crawlPacingBounds(maxItems) {
    const maxPasses = Math.max(PACING.MAX_PASSES_MIN,
      Math.min(PACING.MAX_PASSES_MAX, Math.ceil(maxItems * PACING.MAX_PASSES_PER_ITEM)));
    const minPassesBeforeStop = Math.min(maxPasses - 1,
      Math.max(PACING.MIN_STOP_FLOOR, Math.ceil(maxItems * PACING.MIN_STOP_FRACTION)));
    return { maxPasses, minPassesBeforeStop };
  }

  // How long to wait after this pass's scroll before the next read.
  // s: { pass, producedNewItems, risk }  (producedNewItems/risk used in commit 2)
  function nextCrawlWaitMs(s) {
    return s.pass < PACING.DEEP_PASS_THRESHOLD ? PACING.EARLY_WAIT_MS : PACING.DEEP_WAIT_MS;
  }

  // Scroll distance for this pass (bigger occasional push wakes lazy loading).
  // s: { pass, innerHeight }
  function crawlScrollDeltaY(s) {
    const base = Math.max(Math.floor(s.innerHeight * PACING.SCROLL_VIEWPORT_FRACTION), PACING.SCROLL_MIN_STEP_PX);
    const isBigPush = s.pass % PACING.SCROLL_BIG_PUSH_EVERY === PACING.SCROLL_BIG_PUSH_EVERY - 1;
    return isBigPush ? base * 2 : base;
  }

  // Decide whether to stop, returning the exit_reason ('' = keep going).
  // s: { stagnantPasses, pass, minPassesBeforeStop, itemsLength, lastNewItemPass,
  //      scrollMovedEver, duplicateCount }  (last two used in commit 2)
  function crawlStopReason(s) {
    if (s.stagnantPasses >= PACING.NO_PROGRESS_STAGNANT_STOP && s.pass >= s.minPassesBeforeStop) {
      return 'no_progress';
    }
    if (s.pass >= s.minPassesBeforeStop && s.itemsLength > 0 &&
        s.pass - s.lastNewItemPass >= PACING.NO_NEW_AFTER_SCROLL_STOP) {
      return 'no_new_items_after_scroll';
    }
    return '';
  }

  return Object.freeze({ PACING, crawlPacingBounds, nextCrawlWaitMs, crawlScrollDeltaY, crawlStopReason });
})();
// CommonJS export for the node test harness. No-op in the extension.
if (typeof module !== 'undefined' && module.exports) module.exports = globalThis.THGCrawlPacing;
