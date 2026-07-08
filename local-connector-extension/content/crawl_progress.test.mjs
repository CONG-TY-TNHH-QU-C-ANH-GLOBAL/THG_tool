// Crawl telemetry/policy unit tests. The helpers under test live in
// content/facebook/crawl_progress.js (THGCrawlProgress) — pure, dependency-
// injected, so they test in node without stubbing page globals.
//   Run: node --test content/crawl_progress.test.mjs
import { test } from 'node:test';
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const C = require('./facebook/crawl_progress.js');

const VER = 'scroll-target-v3-cursor'; // crawl.js CRAWLER_VERSION, injected in prod
const SRC = 'https://www.facebook.com/groups/1312868109620530';

// Allowlists — the only values these enum fields may ever carry. Guards raw page
// text from hiding inside a value field, not just a key name.
const PHASE_ALLOW = ['scrolling', 'stalled', 'blocked', 'completed', 'unknown'];
const REASON_ALLOW = [
  'scrolling', 'no_new_posts', 'duplicate_heavy', 'scroll_not_moving',
  'login_required', 'checkpoint_suspected', 'risk_blocked', 'completed', 'unknown',
];

// Exact payload today when no diagnostics: type + crawler_version + task_id +
// intent + account_id + stage + fetched + max + source_url. Nothing else.
test('buildCrawlProgressMessage keeps the byte-identical base payload shape', () => {
  const task = { task_id: 'autocrawl-27-495423', intent: 'facebook_crawl' };
  const msg = C.buildCrawlProgressMessage(VER, task, 50, 'scraping', 5, 50, SRC);
  assert.deepStrictEqual(msg, {
    type: 'thg_crawl_progress',
    crawler_version: VER,
    task_id: 'autocrawl-27-495423',
    intent: 'facebook_crawl',
    account_id: 50,
    stage: 'scraping',
    fetched: 5,
    max: 50,
    source_url: SRC,
  }, 'base payload must stay byte-identical to the pre-diagnostics literal');
});

test('buildCrawlProgressMessage exposes no unexpected keys when no diag passed', () => {
  const msg = C.buildCrawlProgressMessage(VER, { task_id: 't' }, 50, 'scraping', 5, 50, SRC);
  assert.deepStrictEqual(Object.keys(msg).sort(), [
    'account_id', 'crawler_version', 'fetched', 'intent', 'max', 'source_url', 'stage', 'task_id', 'type',
  ], 'no diagnostics keys unless a diag is provided');
});

test('buildCrawlProgressMessage falls back to the same defaults as the old literal', () => {
  assert.deepStrictEqual(
    C.buildCrawlProgressMessage(VER, null, 0, 'started', 0, 20, ''),
    {
      type: 'thg_crawl_progress',
      crawler_version: VER,
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

// --- additive diagnostics ---
const DIAG = {
  phase: 'stalled', found_count: 30, new_count: 5, duplicate_count: 12,
  scroll_count: 8, no_progress_rounds: 6, scroll_moved_ever: true,
  seconds_since_last_new: 42, safe_reason_code: 'no_new_posts',
};

test('buildCrawlProgressMessage includes diagnostics only when a diag is passed', () => {
  const msg = C.buildCrawlProgressMessage(VER, { task_id: 't1', intent: 'facebook_crawl' }, 50, 'scraping', 5, 50, SRC, DIAG);
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
  ], 'payload = the 9 base fields + exactly the 9 whitelisted diagnostics');
});

test('progress payload never exposes raw page text / DOM / secrets (key names)', () => {
  const msg = C.buildCrawlProgressMessage(VER, { task_id: 't1' }, 50, 'finished', 0, 0, SRC, {
    ...DIAG, safe_reason_code: 'checkpoint_suspected', phase: 'blocked',
  });
  const forbidden = ['text', 'html', 'dom', 'body', 'innertext', 'cookie', 'session', 'content', 'token'];
  for (const k of Object.keys(msg)) {
    for (const bad of forbidden) {
      assert.ok(!k.toLowerCase().includes(bad), `payload key ${k} must not expose ${bad}`);
    }
  }
});

// Value-level privacy: enum fields must only ever hold allowlisted tokens, so
// raw page/checkpoint text cannot ride inside phase/safe_reason_code, and the
// counter fields must be numbers / booleans (never a stringified banner).
test('diagnostics values are constrained: enums allowlisted, counters typed', () => {
  const riskInputs = [
    { risk: 'checkpoint' }, { risk: 'login' }, { risk: 'rate_limited' }, { risk: 'blocked' },
    { done: true, reachedMax: true }, { scrollCount: 3, scrollMovedEver: false },
    { newCount: 0, duplicateCount: 5, scrollCount: 2, scrollMovedEver: true },
    { newCount: 0, noProgressRounds: 4, scrollCount: 2, scrollMovedEver: true },
    { newCount: 3, scrollCount: 2, scrollMovedEver: true }, {},
  ];
  for (const s of riskInputs) {
    const { phase, safe_reason_code } = C.classifyCrawlProgress(s);
    assert.ok(PHASE_ALLOW.includes(phase), `phase ${phase} must be allowlisted`);
    assert.ok(REASON_ALLOW.includes(safe_reason_code), `code ${safe_reason_code} must be allowlisted`);
  }
  // Numeric / boolean fields on the built payload.
  const msg = C.buildCrawlProgressMessage(VER, { task_id: 't' }, 1, 'scraping', 1, 50, SRC, DIAG);
  for (const k of ['found_count', 'new_count', 'duplicate_count', 'scroll_count', 'no_progress_rounds', 'seconds_since_last_new']) {
    assert.strictEqual(typeof msg[k], 'number', `${k} must be a number`);
  }
  assert.strictEqual(typeof msg.scroll_moved_ever, 'boolean', 'scroll_moved_ever must be boolean');
  assert.ok(PHASE_ALLOW.includes(msg.phase) && REASON_ALLOW.includes(msg.safe_reason_code));
});

test('classifyCrawlProgress: a risk signal always wins and maps to blocked phase', () => {
  assert.deepStrictEqual(C.classifyCrawlProgress({ risk: 'checkpoint' }), { phase: 'blocked', safe_reason_code: 'checkpoint_suspected' });
  assert.deepStrictEqual(C.classifyCrawlProgress({ risk: 'login' }), { phase: 'blocked', safe_reason_code: 'login_required' });
  assert.deepStrictEqual(C.classifyCrawlProgress({ risk: 'rate_limited' }), { phase: 'blocked', safe_reason_code: 'risk_blocked' });
  assert.deepStrictEqual(C.classifyCrawlProgress({ risk: 'blocked' }), { phase: 'blocked', safe_reason_code: 'risk_blocked' });
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

// Dependency-injected probes: no new heuristic, just delegate to the injected
// classifier. Absent/!function dep → '' (defensive, never throws).
test('detectCrawlRisk delegates to the injected nav classifier only for login/checkpoint', () => {
  const nav = { classifyLanding: (u) => (u.includes('/checkpoint') ? 'checkpoint' : u.includes('/login') ? 'login' : 'feed') };
  assert.strictEqual(C.detectCrawlRisk(nav, 'https://www.facebook.com/checkpoint/123'), 'checkpoint');
  assert.strictEqual(C.detectCrawlRisk(nav, 'https://www.facebook.com/login/'), 'login');
  assert.strictEqual(C.detectCrawlRisk(nav, 'https://www.facebook.com/groups/1'), ''); // feed → no risk
  assert.strictEqual(C.detectCrawlRisk(null, 'https://x'), ''); // no classifier
  assert.strictEqual(C.detectCrawlRisk({}, 'https://x'), ''); // classifier absent
});

test('detectCrawlBanner delegates to the injected proof classifier', () => {
  assert.strictEqual(C.detectCrawlBanner({ detectPlatformReject: () => 'checkpoint' }), 'checkpoint');
  assert.strictEqual(C.detectCrawlBanner({ detectPlatformReject: () => '' }), '');
  assert.strictEqual(C.detectCrawlBanner(null), '');
  assert.strictEqual(C.detectCrawlBanner({}), '');
});
