// PR-C1A boundary characterization — buildCrawlProgressMessage.
// Pins the thg_crawl_progress payload shape BEFORE any telemetry (PR-C1B) or
// checkpoint phase (PR-C2) fields are added, so a future change that alters the
// existing wire fields is caught here. Behavior-preserving refactor guard only.
//   Run: node --test content/crawl_progress.test.mjs
import { test } from 'node:test';
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const C = require('./crawl.js');

const SRC = 'https://www.facebook.com/groups/1312868109620530';

// Exact payload today: type + crawler_version + task_id + intent + account_id
// + stage + fetched + max + source_url. Nothing else (no telemetry yet).
test('buildCrawlProgressMessage keeps the byte-identical payload shape', () => {
  const task = { task_id: 'autocrawl-27-495423', intent: 'facebook_crawl' };
  const msg = C.buildCrawlProgressMessage(task, 50, 'scraping', 5, 50, SRC);
  assert.deepStrictEqual(msg, {
    type: 'thg_crawl_progress',
    crawler_version: 'scroll-target-v3-cursor',
    task_id: 'autocrawl-27-495423',
    intent: 'facebook_crawl',
    account_id: 50,
    stage: 'scraping',
    fetched: 5,
    max: 50,
    source_url: SRC,
  }, 'progress payload shape must stay byte-identical to the pre-refactor literal');
});

// No stray/new keys leak in (guards against an accidental telemetry field).
test('buildCrawlProgressMessage exposes no unexpected keys (no telemetry yet)', () => {
  const task = { task_id: 'autocrawl-27-495423', intent: 'facebook_crawl' };
  const msg = C.buildCrawlProgressMessage(task, 50, 'scraping', 5, 50, SRC);
  assert.deepStrictEqual(Object.keys(msg).sort(), [
    'account_id', 'crawler_version', 'fetched', 'intent', 'max', 'source_url', 'stage', 'task_id', 'type',
  ], 'no unexpected keys in the C1A progress payload');
});

// Defaults mirror the old inline literal exactly: missing task/account → ''/0.
test('buildCrawlProgressMessage falls back to the same defaults as the old literal', () => {
  assert.deepStrictEqual(
    C.buildCrawlProgressMessage(null, 0, 'started', 0, 20, ''),
    {
      type: 'thg_crawl_progress',
      crawler_version: 'scroll-target-v3-cursor',
      task_id: '',
      intent: 'facebook_crawl',
      account_id: 0,
      stage: 'started',
      fetched: 0,
      max: 20,
      source_url: '',
    },
    'nil task / zero account fall back to the same defaults as the old literal',
  );
});

// --- PR-C1B: additive diagnostics ---
const DIAG = {
  phase: 'stalled', found_count: 30, new_count: 5, duplicate_count: 12,
  scroll_count: 8, no_progress_rounds: 6, scroll_moved_ever: true,
  seconds_since_last_new: 42, safe_reason_code: 'no_new_posts',
};

test('buildCrawlProgressMessage includes diagnostics only when a diag is passed', () => {
  const task = { task_id: 't1', intent: 'facebook_crawl' };
  const msg = C.buildCrawlProgressMessage(task, 50, 'scraping', 5, 50, SRC, DIAG);
  assert.strictEqual(msg.phase, 'stalled');
  assert.strictEqual(msg.new_count, 5);
  assert.strictEqual(msg.duplicate_count, 12);
  assert.strictEqual(msg.no_progress_rounds, 6);
  assert.strictEqual(msg.scroll_moved_ever, true);
  assert.strictEqual(msg.safe_reason_code, 'no_new_posts');
  assert.deepStrictEqual(Object.keys(msg).sort(), [
    'account_id', 'crawler_version', 'duplicate_count', 'fetched', 'found_count',
    'intent', 'max', 'new_count', 'no_progress_rounds', 'phase', 'safe_reason_code',
    'scroll_count', 'scroll_moved_ever', 'seconds_since_last_new', 'source_url',
    'stage', 'task_id', 'type',
  ], 'C1B payload = the 9 base fields + exactly the 9 whitelisted diagnostics');
});

test('progress payload never exposes raw page text / DOM / secrets', () => {
  const msg = C.buildCrawlProgressMessage({ task_id: 't1' }, 50, 'finished', 0, 0, SRC, {
    ...DIAG, safe_reason_code: 'checkpoint_suspected', phase: 'blocked',
  });
  const forbidden = ['text', 'html', 'dom', 'body', 'innertext', 'cookie', 'session', 'content', 'token'];
  for (const k of Object.keys(msg)) {
    for (const bad of forbidden) {
      assert.ok(!k.toLowerCase().includes(bad), `payload key ${k} must not expose ${bad}`);
    }
  }
});

test('classifyCrawlProgress: a risk signal always wins and maps to blocked phase', () => {
  assert.deepStrictEqual(C.classifyCrawlProgress({ risk: 'checkpoint' }), { phase: 'blocked', safe_reason_code: 'checkpoint_suspected' });
  assert.deepStrictEqual(C.classifyCrawlProgress({ risk: 'login' }), { phase: 'blocked', safe_reason_code: 'login_required' });
  assert.deepStrictEqual(C.classifyCrawlProgress({ risk: 'rate_limited' }), { phase: 'blocked', safe_reason_code: 'risk_blocked' });
  assert.deepStrictEqual(C.classifyCrawlProgress({ risk: 'blocked' }), { phase: 'blocked', safe_reason_code: 'risk_blocked' });
  // risk beats a would-be "scrolling" verdict
  assert.deepStrictEqual(C.classifyCrawlProgress({ risk: 'checkpoint', newCount: 9 }), { phase: 'blocked', safe_reason_code: 'checkpoint_suspected' });
});

test('classifyCrawlProgress: non-risk states', () => {
  assert.deepStrictEqual(C.classifyCrawlProgress({ done: true, reachedMax: true }), { phase: 'completed', safe_reason_code: 'completed' });
  assert.deepStrictEqual(C.classifyCrawlProgress({ scrollCount: 3, scrollMovedEver: false }), { phase: 'stalled', safe_reason_code: 'scroll_not_moving' });
  assert.deepStrictEqual(C.classifyCrawlProgress({ newCount: 0, duplicateCount: 5, scrollCount: 2, scrollMovedEver: true }), { phase: 'stalled', safe_reason_code: 'duplicate_heavy' });
  assert.deepStrictEqual(C.classifyCrawlProgress({ newCount: 0, duplicateCount: 0, noProgressRounds: 4, scrollCount: 2, scrollMovedEver: true }), { phase: 'stalled', safe_reason_code: 'no_new_posts' });
  assert.deepStrictEqual(C.classifyCrawlProgress({ newCount: 3, scrollCount: 2, scrollMovedEver: true }), { phase: 'scrolling', safe_reason_code: 'scrolling' });
});

test('crawlRiskToReason maps raw classifier signals to stable codes', () => {
  assert.strictEqual(C.crawlRiskToReason('login'), 'login_required');
  assert.strictEqual(C.crawlRiskToReason('checkpoint'), 'checkpoint_suspected');
  assert.strictEqual(C.crawlRiskToReason('rate_limited'), 'risk_blocked');
  assert.strictEqual(C.crawlRiskToReason('blocked'), 'risk_blocked');
  assert.strictEqual(C.crawlRiskToReason(''), '');
  assert.strictEqual(C.crawlRiskToReason('something_unknown'), '');
});
