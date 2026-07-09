// PR-C2 crawl pacing policy tests. THGCrawlPacing (content/facebook/crawl_pacing.js)
// owns waits / scroll step / stop decisions as pure functions.
//   Run: node --test content/crawl_pacing.test.mjs
import { test } from 'node:test';
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const P = require('./facebook/crawl_pacing.js');

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
