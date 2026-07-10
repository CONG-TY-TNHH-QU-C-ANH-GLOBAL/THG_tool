// PR-M1 Facebook post-timestamp parser tests. THGCrawlTime
// (platforms/facebook/crawl_time.js) owns the pure timestamp parse + the
// canonical TimestampParse DTO (spec §4). Table-driven, vi/en locales, no DOM,
// no clock beyond a passed-in `now`.
//   Run: node --test platforms/facebook/crawl_time.test.mjs
import { test } from 'node:test';
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const T = require('./crawl_time.js');

// Fixed server clock for every relative-interval assertion.
const NOW = Date.parse('2026-07-10T12:00:00.000Z');
const H = 3600 * 1000;
const MIN = 60 * 1000;
const D = 24 * H;
const rel = (text) => T.parseRelativeAge(text, NOW);

test('minutes → derived_relative with a [N, N+1) minute interval', () => {
  const p = rel('5 phút');
  assert.strictEqual(p.confidence, 'derived_relative');
  assert.strictEqual(p.raw_unit, 'minute');
  // latest = now-5min (literal reading); earliest = now-6min (truncation).
  assert.strictEqual(p.latest_utc, new Date(NOW - 5 * MIN).toISOString());
  assert.strictEqual(p.earliest_utc, new Date(NOW - 6 * MIN).toISOString());
  assert.strictEqual(p.posted_at, p.latest_utc);
});

test('hours → derived_relative (vi + en shorthand)', () => {
  for (const text of ['2 giờ', '2 hours', '2h', '2 hrs']) {
    const p = rel(text);
    assert.strictEqual(p.confidence, 'derived_relative', text);
    assert.strictEqual(p.raw_unit, 'hour', text);
    assert.strictEqual(p.latest_utc, new Date(NOW - 2 * H).toISOString(), text);
  }
});

test('23 giờ → window [23h,24h): earliest=now-24h, latest=now-23h (spec §4)', () => {
  const p = rel('23 giờ');
  assert.strictEqual(p.confidence, 'derived_relative');
  assert.strictEqual(p.earliest_utc, new Date(NOW - 24 * H).toISOString());
  assert.strictEqual(p.latest_utc, new Date(NOW - 23 * H).toISOString());
});

test('24 giờ → still derived_relative; parser exposes the interval, not eligibility', () => {
  // The parser must NOT decide freshness — it only reports the worst-case
  // interval [24h,25h). A later gate rules it stale; here it stays parseable.
  const p = rel('24 giờ');
  assert.strictEqual(p.confidence, 'derived_relative');
  assert.strictEqual(p.earliest_utc, new Date(NOW - 25 * H).toISOString());
  assert.strictEqual(p.latest_utc, new Date(NOW - 24 * H).toISOString());
});

test('just-now text → derived_relative, [now-1min, now] interval (vi + en)', () => {
  for (const text of ['vừa xong', 'vua xong', 'just now', 'now', 'Just Now']) {
    const p = rel(text);
    assert.strictEqual(p.confidence, 'derived_relative', text);
    assert.strictEqual(p.raw_unit, 'minute', text);
    assert.strictEqual(p.posted_at, new Date(NOW).toISOString(), text);
    assert.strictEqual(p.latest_utc, new Date(NOW).toISOString(), text);
    assert.strictEqual(p.earliest_utc, new Date(NOW - MIN).toISOString(), text);
  }
});

test('day-level text → ambiguous (1 ngày / hôm qua / yesterday / 1d)', () => {
  for (const text of ['1 ngày', 'hôm qua', 'yesterday', '1d']) {
    const p = rel(text);
    assert.strictEqual(p.confidence, 'ambiguous', text);
  }
});

test('weeks → ambiguous (coarser than a day)', () => {
  assert.strictEqual(rel('3 tuần').confidence, 'ambiguous');
  assert.strictEqual(rel('3w').confidence, 'ambiguous');
});

test('no parseable timestamp → unknown, all fields null/none', () => {
  for (const text of ['', '   ', 'See more', '12', 'Thích']) {
    const p = rel(text);
    assert.strictEqual(p, null, `${text} should not parse as a relative age`);
  }
  const u = T.classifyTimestampSignal({ relativeText: 'See more' }, NOW);
  assert.strictEqual(u.confidence, 'unknown');
  assert.strictEqual(u.posted_at, null);
  assert.strictEqual(u.earliest_utc, null);
  assert.strictEqual(u.latest_utc, null);
  assert.strictEqual(u.raw_unit, 'none');
});

test('no-guess: an age word embedded in a sentence is NOT a timestamp', () => {
  // Only a complete age string parses; a body sentence that merely starts
  // with (or contains) "2 giờ" / "hôm qua" must stay unknown.
  for (const text of [
    '2 hours at beach', '2 giờ tại bãi biển', '2h and counting',
    'hôm qua tôi đăng bài', 'yesterday I posted this', 'nowhere',
  ]) {
    assert.strictEqual(rel(text), null, `${text} must not parse as an age`);
  }
  // The complete forms still parse.
  assert.strictEqual(rel('2 hours').confidence, 'derived_relative');
  assert.strictEqual(rel('2 giờ').confidence, 'derived_relative');
  assert.strictEqual(rel('hôm qua').confidence, 'ambiguous');
  assert.strictEqual(rel('vừa xong').confidence, 'derived_relative');
  assert.strictEqual(rel('just now').confidence, 'derived_relative');
});

test('exact machine datetime → confidence exact, point interval, raw_unit date', () => {
  const at = '2026-07-10T09:30:00.000Z';
  const p = T.classifyTimestampSignal({ exactUtc: at, relativeText: 'hôm qua' }, NOW);
  assert.strictEqual(p.confidence, 'exact'); // exact wins over ambiguous text
  assert.strictEqual(p.posted_at, at);
  assert.strictEqual(p.earliest_utc, at);
  assert.strictEqual(p.latest_utc, at);
  assert.strictEqual(p.raw_unit, 'date');
});

test('exact FUTURE datetime still parses as exact — invalidity is a downstream gate concern', () => {
  // spec §4: future/invalid is decided by the freshness gate (later PR), not
  // by the parser. The parser only exposes what it read.
  const future = new Date(NOW + 3 * H).toISOString();
  const p = T.classifyTimestampSignal({ exactUtc: future }, NOW);
  assert.strictEqual(p.confidence, 'exact');
  assert.strictEqual(p.posted_at, future);
});

test('invalid now → unknown (parser never fabricates a clock)', () => {
  const p = T.classifyTimestampSignal({ relativeText: '5 phút' }, 'not-a-date');
  assert.strictEqual(p.confidence, 'unknown');
});

test('every DTO carries the parser_version tag', () => {
  assert.strictEqual(rel('2 giờ').parser_version, T.PARSER_VERSION);
  assert.strictEqual(T.classifyTimestampSignal({}, NOW).parser_version, T.PARSER_VERSION);
});

// ── DOM adapter: reads only the handed node, delegates to the pure core ──
function fakeNode({ dataUtime = null, datetime = null, texts = [] } = {}) {
  return {
    querySelector: (sel) => {
      if (sel.includes('data-utime') && dataUtime != null) {
        return { dataset: { utime: String(dataUtime) } };
      }
      if (sel.includes('datetime') && datetime != null) {
        return { getAttribute: () => datetime };
      }
      return null;
    },
    querySelectorAll: () => texts.map(t => ({ textContent: t, getAttribute: () => null })),
  };
}

test('parsePostTimestamp: data-utime (epoch seconds) → exact', () => {
  const secs = Math.floor((NOW - 4 * H) / 1000);
  const p = T.parsePostTimestamp(fakeNode({ dataUtime: secs, texts: ['hôm qua'] }), NOW);
  assert.strictEqual(p.confidence, 'exact');
  assert.strictEqual(p.posted_at, new Date(secs * 1000).toISOString());
});

test('parsePostTimestamp: <time datetime> → exact, normalized ISO point interval', () => {
  const at = '2026-07-10T09:30:00.000Z';
  const p = T.parsePostTimestamp(fakeNode({ datetime: at, texts: ['hôm qua'] }), NOW);
  assert.strictEqual(p.confidence, 'exact'); // machine datetime wins over text
  assert.strictEqual(p.posted_at, at);
  assert.strictEqual(p.earliest_utc, at);
  assert.strictEqual(p.latest_utc, at);
  assert.strictEqual(p.raw_unit, 'date');
});

test('parsePostTimestamp: relative anchor text when no machine datetime', () => {
  const p = T.parsePostTimestamp(fakeNode({ texts: ['Nguyễn Văn A', '2 giờ'] }), NOW);
  assert.strictEqual(p.confidence, 'derived_relative');
  assert.strictEqual(p.raw_unit, 'hour');
});

test('parsePostTimestamp: nothing timestamp-like → unknown', () => {
  const p = T.parsePostTimestamp(fakeNode({ texts: ['See more', 'Like', 'Comment'] }), NOW);
  assert.strictEqual(p.confidence, 'unknown');
});
