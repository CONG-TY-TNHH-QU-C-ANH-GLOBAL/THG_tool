// PR-C1C characterization — THGCrawlResult builders. This is the crawl_result
// WIRE shape sent to /api/connectors/crawl-result, so these assert the EXACT
// object (field names + values) to prove the extraction is byte-identical to the
// prior inline literals in crawl.js.
//   Run: node --test content/crawl_result.test.mjs
import { test } from 'node:test';
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const RES = require('../platforms/facebook/crawl_result.js');

const VER = 'scroll-target-v3-cursor';
const LANDED = 'https://www.facebook.com/groups/1312868109620530';

test('buildScrollDiag maps snake_case fields exactly', () => {
  assert.deepStrictEqual(RES.buildScrollDiag({
    passes: 12, maxArticlesSeen: 30, maxScrollY: 4000, maxDocHeight: 9000,
    scrollMovedEver: true, finalScrollTarget: 'document', landedUrl: LANDED,
  }), {
    passes: 12, max_articles_seen: 30, max_scroll_y: 4000, max_doc_height: 9000,
    scroll_moved_ever: true, final_scroll_target: 'document', landed_url: LANDED,
  });
});

test('buildDirectPostResult: found target — exact shape, no error key', () => {
  const r = RES.buildDirectPostResult({
    crawlerVersion: VER, task: { task_id: 't1', intent: 'facebook_crawl' },
    items: [{ id: 'dp:1' }], exitReason: 'direct_post_target_found', error: '',
    passes: 3, maxArticles: 5, landedUrl: LANDED,
  });
  assert.deepStrictEqual(r, {
    ok: true,
    crawl_result: {
      crawler_version: VER,
      task_id: 't1',
      intent: 'facebook_crawl',
      keywords: [],
      items: [{ id: 'dp:1' }],
      exit_reason: 'direct_post_target_found',
      direct_post: true,
      scroll_diag: {
        passes: 3, max_articles_seen: 5, max_scroll_y: 0, max_doc_height: 0,
        scroll_moved_ever: false, final_scroll_target: '', landed_url: LANDED,
      },
    },
  });
  assert.ok(!('error' in r.crawl_result), 'no error key when error is empty');
});

test('buildDirectPostResult: typed failure carries the error key', () => {
  const r = RES.buildDirectPostResult({
    crawlerVersion: VER, task: null, items: [], exitReason: 'direct_post_boilerplate_content',
    error: 'direct_post_boilerplate_content', passes: 12, maxArticles: 2, landedUrl: LANDED,
  });
  assert.strictEqual(r.crawl_result.error, 'direct_post_boilerplate_content');
  assert.strictEqual(r.crawl_result.task_id, '');
  assert.strictEqual(r.crawl_result.intent, 'facebook_crawl');
  assert.strictEqual(r.crawl_result.direct_post, true);
});

test('buildBroadCrawlResult: exact shape incl. keywords passthrough + cursor_reached', () => {
  const r = RES.buildBroadCrawlResult({
    crawlerVersion: VER, task: { task_id: 't2', intent: 'facebook_crawl', keywords: ['a', 'b'] },
    items: [{ id: 'c:1' }, { id: 'c:2' }], exitReason: 'maxItems', cursorReached: false,
    scrollDiag: {
      passes: 20, maxArticlesSeen: 40, maxScrollY: 8000, maxDocHeight: 12000,
      scrollMovedEver: true, finalScrollTarget: 'div[role=feed]', landedUrl: LANDED,
    },
  });
  assert.deepStrictEqual(r, {
    ok: true,
    crawl_result: {
      crawler_version: VER,
      task_id: 't2',
      intent: 'facebook_crawl',
      keywords: ['a', 'b'],
      items: [{ id: 'c:1' }, { id: 'c:2' }],
      exit_reason: 'maxItems',
      cursor_reached: false,
      scroll_diag: {
        passes: 20, max_articles_seen: 40, max_scroll_y: 8000, max_doc_height: 12000,
        scroll_moved_ever: true, final_scroll_target: 'div[role=feed]', landed_url: LANDED,
      },
    },
  });
});

test('buildBroadCrawlResult: non-array keywords → [] (matches old literal)', () => {
  const r = RES.buildBroadCrawlResult({
    crawlerVersion: VER, task: { task_id: 't3' }, items: [], exitReason: 'no_progress',
    cursorReached: true,
    scrollDiag: { passes: 1, maxArticlesSeen: 0, maxScrollY: 0, maxDocHeight: 0, scrollMovedEver: false, finalScrollTarget: '', landedUrl: LANDED },
  });
  assert.deepStrictEqual(r.crawl_result.keywords, []);
  assert.strictEqual(r.crawl_result.cursor_reached, true);
});
