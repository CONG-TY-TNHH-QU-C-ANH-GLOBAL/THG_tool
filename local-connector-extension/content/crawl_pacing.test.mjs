// PR-C2 crawl pacing policy tests. THGCrawlPacing (platforms/facebook/crawl_pacing.js)
// owns waits / scroll step / stop decisions as pure functions.
//   Run: node --test content/crawl_pacing.test.mjs
import { test } from 'node:test';
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const P = require('../platforms/facebook/crawl_pacing.js');

// ── commit 1: behavior-IDENTICAL extraction (pins the pre-PR-C2 constants) ──
test('crawlPacingBounds matches the original clamp math', () => {
  assert.deepStrictEqual(P.crawlPacingBounds(50), { maxPasses: 150, minPassesBeforeStop: 35 });
  assert.deepStrictEqual(P.crawlPacingBounds(20), { maxPasses: 70, minPassesBeforeStop: 18 });
  assert.deepStrictEqual(P.crawlPacingBounds(1), { maxPasses: 70, minPassesBeforeStop: 18 });
  assert.deepStrictEqual(P.crawlPacingBounds(200), { maxPasses: 260, minPassesBeforeStop: 140 });
});

test('nextCrawlWaitMs: barren passes keep the cautious tiered wait', () => {
  // No new items reported → the original tiered wait (2200 early / 3600 deep).
  // These hold in both commit 1 and commit 2 (only productive passes speed up).
  assert.strictEqual(P.nextCrawlWaitMs({ pass: 0 }), 2200);
  assert.strictEqual(P.nextCrawlWaitMs({ pass: 7 }), 2200);
  assert.strictEqual(P.nextCrawlWaitMs({ pass: 8 }), 3600);
  assert.strictEqual(P.nextCrawlWaitMs({ pass: 40 }), 3600);
});

test('crawlScrollDeltaY matches the original viewportStep math', () => {
  // base = max(floor(innerHeight*0.95), 700); doubled every 6th pass (pass%6===5)
  assert.strictEqual(P.crawlScrollDeltaY({ pass: 0, innerHeight: 1000 }), 950);
  assert.strictEqual(P.crawlScrollDeltaY({ pass: 5, innerHeight: 1000 }), 1900);
  assert.strictEqual(P.crawlScrollDeltaY({ pass: 0, innerHeight: 500 }), 700); // floor(475) < 700 → floor to 700
  assert.strictEqual(P.crawlScrollDeltaY({ pass: 11, innerHeight: 1000 }), 1900); // 11%6===5
});

test('crawlStopReason: original no_progress / no_new thresholds', () => {
  const base = { stagnantPasses: 0, pass: 40, minPassesBeforeStop: 35, itemsLength: 5, lastNewItemPass: 40, scrollMovedEver: true, duplicateCount: 0 };
  // no_progress needs 10 stagnant AND pass >= min
  assert.strictEqual(P.crawlStopReason({ ...base, stagnantPasses: 10 }), 'no_progress');
  assert.strictEqual(P.crawlStopReason({ ...base, stagnantPasses: 10, pass: 34, minPassesBeforeStop: 35 }), ''); // before min
  assert.strictEqual(P.crawlStopReason({ ...base, stagnantPasses: 9 }), ''); // under threshold
  // no_new needs pass>=min, items>0, pass-lastNew>=16
  assert.strictEqual(P.crawlStopReason({ ...base, lastNewItemPass: 40 - 16 }), 'no_new_items_after_scroll');
  assert.strictEqual(P.crawlStopReason({ ...base, lastNewItemPass: 40 - 15 }), ''); // one short
  assert.strictEqual(P.crawlStopReason({ ...base, itemsLength: 0, lastNewItemPass: 0 }), ''); // no items → no no_new
  // healthy pass keeps going
  assert.strictEqual(P.crawlStopReason(base), '');
});

// ── commit 2: conservative safe tuning ──
test('C2 #1: a productive safe pass waits less than the old baseline', () => {
  // Productive waits are exact, AND strictly shorter than the barren wait at the
  // same pass (compare against the actual function output, not a literal).
  assert.strictEqual(P.nextCrawlWaitMs({ pass: 3, producedNewItems: true }), 1500);
  assert.ok(P.nextCrawlWaitMs({ pass: 3, producedNewItems: true }) < P.nextCrawlWaitMs({ pass: 3 }),
    'productive early wait must be shorter than the barren early wait');
  assert.strictEqual(P.nextCrawlWaitMs({ pass: 20, producedNewItems: true }), 1500);
  assert.ok(P.nextCrawlWaitMs({ pass: 20, producedNewItems: true }) < P.nextCrawlWaitMs({ pass: 20 }),
    'productive deep wait must be shorter than the barren deep wait');
});

test('C2 #5: risk state never gets the speed-up (checkpoint/login/risk)', () => {
  for (const risk of ['checkpoint', 'login', 'rate_limited', 'blocked']) {
    assert.strictEqual(P.nextCrawlWaitMs({ pass: 3, producedNewItems: true, risk }), 3600,
      `risk=${risk} must keep the cautious wait, never PRODUCTIVE_WAIT_MS`);
  }
});

test('C2 #3/#4: scroll_not_moving requires zero collected; never when items>0', () => {
  const stuck = { stagnantPasses: 0, pass: 8, minPassesBeforeStop: 35, itemsLength: 0, lastNewItemPass: 0, scrollMovedEver: false, duplicateCount: 0 };
  assert.strictEqual(P.crawlStopReason(stuck), 'scroll_not_moving'); // items===0, no scroll, pass>=8
  assert.strictEqual(P.crawlStopReason({ ...stuck, pass: 7 }), '');   // before SCROLL_STUCK_STOP_PASSES
  assert.strictEqual(P.crawlStopReason({ ...stuck, scrollMovedEver: true }), ''); // scroll did move
  // new_count > 0 → no scroll_not_moving, and (pass 8 < min) no other stop → ''
  assert.strictEqual(P.crawlStopReason({ ...stuck, itemsLength: 3 }), '');
});

test('C2 #2: duplicate-heavy stops earlier than no-new, only on strong evidence', () => {
  const base = { stagnantPasses: 0, pass: 40, minPassesBeforeStop: 35, itemsLength: 5, scrollMovedEver: true };
  // 12 passes since new + 40 dupes → duplicate_heavy (earlier than the 16 window)
  assert.strictEqual(P.crawlStopReason({ ...base, lastNewItemPass: 40 - 12, duplicateCount: 40 }), 'duplicate_heavy');
  // dupes below the strong-evidence floor → NOT duplicate_heavy (and 12 < 16 → keep going)
  assert.strictEqual(P.crawlStopReason({ ...base, lastNewItemPass: 40 - 12, duplicateCount: 39 }), '');
  // fewer passes-since-new → not yet
  assert.strictEqual(P.crawlStopReason({ ...base, lastNewItemPass: 40 - 11, duplicateCount: 100 }), '');
  // the generic no-new window still fires at 16 regardless of dupes
  assert.strictEqual(P.crawlStopReason({ ...base, lastNewItemPass: 40 - 16, duplicateCount: 0 }), 'no_new_items_after_scroll');
});

test('C2 #6: min-pass guard respected for duplicate_heavy / no_new (not scroll-stuck)', () => {
  const early = { stagnantPasses: 0, pass: 20, minPassesBeforeStop: 35, itemsLength: 5, lastNewItemPass: 0, scrollMovedEver: true, duplicateCount: 100 };
  // pass 20 < min 35 → neither duplicate_heavy nor no_new fires
  assert.strictEqual(P.crawlStopReason(early), '');
});

test('C2 #7: every returned exit reason is in the safe, stable set', () => {
  const allowed = new Set(['', 'no_progress', 'no_new_items_after_scroll', 'scroll_not_moving', 'duplicate_heavy']);
  const samples = [
    { stagnantPasses: 10, pass: 40, minPassesBeforeStop: 35, itemsLength: 5, lastNewItemPass: 40, scrollMovedEver: true, duplicateCount: 0 },
    { stagnantPasses: 0, pass: 8, minPassesBeforeStop: 35, itemsLength: 0, lastNewItemPass: 0, scrollMovedEver: false, duplicateCount: 0 },
    { stagnantPasses: 0, pass: 40, minPassesBeforeStop: 35, itemsLength: 5, lastNewItemPass: 28, scrollMovedEver: true, duplicateCount: 40 },
    { stagnantPasses: 0, pass: 40, minPassesBeforeStop: 35, itemsLength: 5, lastNewItemPass: 24, scrollMovedEver: true, duplicateCount: 0 },
    { stagnantPasses: 0, pass: 5, minPassesBeforeStop: 35, itemsLength: 3, lastNewItemPass: 5, scrollMovedEver: true, duplicateCount: 0 },
  ];
  for (const s of samples) {
    const reason = P.crawlStopReason(s);
    assert.ok(allowed.has(reason), `reason must be in the safe set: got ${reason}`);
  }
});
