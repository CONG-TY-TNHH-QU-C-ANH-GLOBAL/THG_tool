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
    // PR-C2: a PRODUCTIVE pass (new posts arrived, no risk) waits less — still
    // well above zero so FB can lazy-load the next batch, just not the full
    // cautious wait. This is the main fix for feeds that ARE flowing but were
    // paying 2200/3600ms every pass to reach the target count.
    PRODUCTIVE_WAIT_MS: 1500,

    // Scroll geometry (unchanged from the original inline math).
    SCROLL_VIEWPORT_FRACTION: 0.95,
    SCROLL_MIN_STEP_PX: 700,
    SCROLL_BIG_PUSH_EVERY: 6, // every Nth pass do a double-size push

    // Stop thresholds — evidence required before ending the crawl.
    NO_PROGRESS_STAGNANT_STOP: 10, // consecutive stagnant passes → no_progress
    NO_NEW_AFTER_SCROLL_STOP: 16,  // passes since last new item → no_new_items_after_scroll
    // PR-C2 early stops (only fire on STRONG evidence, so a slow-but-live feed
    // is never cut short):
    // - our scroll never moved the viewport AND we collected nothing → the tab
    //   is throttled or the scroll target is wrong; stop early (nothing to lose).
    SCROLL_STUCK_STOP_PASSES: 8,
    // - the feed is just re-serving posts we already have: many duplicates and
    //   no new item for a while → confirm exhaustion earlier than the generic
    //   no-new window. Requires items already in hand (never a zero-yield stop).
    DUP_HEAVY_NO_NEW_STOP: 12,   // < NO_NEW_AFTER_SCROLL_STOP (16)
    DUP_HEAVY_MIN_DUPES: 40,     // strong "we've re-seen this feed" evidence
    // - VERY heavy duplicate evidence (PR crawl-UX): the feed re-served several
    //   times more posts than we collected AND nothing new for the full dup-heavy
    //   window. Exhaustion beyond doubt — waiting for the fraction-based min
    //   guard (0.7 × max_items passes) only buys minutes of duplicate churn, so
    //   this branch respects the absolute MIN_STOP_FLOOR instead. Non-risk path
    //   only; both duplicate gates must hold (absolute AND relative-to-yield).
    DUP_VERY_HEAVY_MIN_DUPES: 60,
    DUP_VERY_HEAVY_YIELD_RATIO: 3, // duplicates ≥ 3× collected items

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
  // s: { pass, producedNewItems, risk }
  function nextCrawlWaitMs(s) {
    // Never speed up when a risk/checkpoint/login signal is present. Belt-and-
    // suspenders: crawl.js already breaks the loop on risk before pacing runs,
    // so risk is normally '' here — but if it ever isn't, stay cautious.
    if (s.risk) return PACING.DEEP_WAIT_MS;
    // Productive safe pass → shorter (but lazy-load-safe) wait.
    if (s.producedNewItems) return PACING.PRODUCTIVE_WAIT_MS;
    // Barren/uncertain pass → the cautious tiered wait, unchanged.
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
  //      scrollMovedEver, duplicateCount }
  // Order = strongest/soonest evidence first. All post-min stops are unchanged
  // from before except where noted; the two PR-C2 early stops require strong
  // evidence so a slow-but-live feed is never cut short.
  function crawlStopReason(s) {
    // Nothing loads after repeated scrolls (unchanged).
    if (s.stagnantPasses >= PACING.NO_PROGRESS_STAGNANT_STOP && s.pass >= s.minPassesBeforeStop) {
      return 'no_progress';
    }
    // PR-C2: scroll never moved AND zero posts collected → throttled tab / wrong
    // scroll target. Fires only when itemsLength === 0 (nothing to lose) — never
    // when posts were collected. Independent of minPassesBeforeStop so a stuck
    // tab exits fast instead of grinding the full budget.
    const passesSinceNew = s.pass - s.lastNewItemPass;
    if (s.itemsLength === 0 && !s.scrollMovedEver && s.pass >= PACING.SCROLL_STUCK_STOP_PASSES) {
      return 'scroll_not_moving';
    }
    // Shared duplicate-heavy evidence: items in hand AND the full no-new window.
    // Both branches below add their own pass floor + duplicate thresholds.
    const dupHeavyNoNewWindow = s.itemsLength > 0 && passesSinceNew >= PACING.DUP_HEAVY_NO_NEW_STOP;
    // PR-C2: past the min guard with items in hand, the feed is only re-serving
    // duplicates and nothing new for a while → confirm exhaustion a little
    // earlier than the generic no-new window.
    if (dupHeavyNoNewWindow && s.pass >= s.minPassesBeforeStop &&
        s.duplicateCount >= PACING.DUP_HEAVY_MIN_DUPES) {
      return 'duplicate_heavy';
    }
    // VERY heavy duplicates (see PACING comment): same no-new window, but the
    // duplicate evidence is overwhelming (absolute floor AND ≥3× collected), so
    // only the absolute MIN_STOP_FLOOR applies — not the fraction-based guard
    // that made a 50-item crawl churn duplicates for extra minutes. Never fires
    // on zero yield and never touches the risk/checkpoint path (handled above
    // the pacing layer in crawl.js).
    if (dupHeavyNoNewWindow && s.pass >= PACING.MIN_STOP_FLOOR &&
        s.duplicateCount >= PACING.DUP_VERY_HEAVY_MIN_DUPES &&
        s.duplicateCount >= s.itemsLength * PACING.DUP_VERY_HEAVY_YIELD_RATIO) {
      return 'duplicate_heavy';
    }
    // Generic: scrolled but no new items for the full window (unchanged).
    if (s.pass >= s.minPassesBeforeStop && s.itemsLength > 0 &&
        passesSinceNew >= PACING.NO_NEW_AFTER_SCROLL_STOP) {
      return 'no_new_items_after_scroll';
    }
    return '';
  }

  return Object.freeze({ PACING, crawlPacingBounds, nextCrawlWaitMs, crawlScrollDeltaY, crawlStopReason });
})();
// CommonJS export for the node test harness. No-op in the extension.
if (typeof module !== 'undefined' && module.exports) module.exports = globalThis.THGCrawlPacing;
