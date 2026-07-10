// THG Facebook crawl — post timestamp parser + canonical TimestampParse DTO.
// PR-M1 of the Multi-Group Fresh-Lead Crawl train
// (specs/facebook/MULTI_GROUP_FRESH_LEAD_CRAWL_SPEC.md §4).
//
// Facebook renders post age as localized relative text ("2 giờ", "5 phút",
// "hôm qua", "1d"), occasionally as a machine-readable datetime attribute, and
// sometimes not at all. This module turns whatever the article node exposes
// into ONE canonical DTO — never a silent guess.
//
// PURITY / CLOCK AUTHORITY: the parser never reads a clock. `now` is passed in
// (the server clock is the eligibility authority, spec §4 — wired in a later
// PR). It reads only the article node it is handed; no document walking. This
// PR emits the DTO as additive telemetry only: it does NOT decide lead
// eligibility, freshness, or any stop — those land in later PRs. `unknown` is
// an acceptable result; posts are never dropped here on a timestamp.
//
// PRIVACY: only typed fields leave the browser. Raw timestamp text is an
// implementation detail and is never part of the DTO (spec §4, PR-C0.5 §6).
globalThis.THGCrawlTime = globalThis.THGCrawlTime || (() => {
  const PARSER_VERSION = 'fb-ts-v1';

  const UNIT_MS = Object.freeze({
    minute: 60 * 1000,
    hour: 60 * 60 * 1000,
    day: 24 * 60 * 60 * 1000,
    week: 7 * 24 * 60 * 60 * 1000,
  });

  // Relative-unit vocabulary (vi + en). minute/hour are confidently
  // narrow → derived_relative; day/week are coarse (a day-granular truncation
  // is wider than a 24h freshness window) → ambiguous. Longer tokens precede
  // their single-letter forms so the anchored match is unambiguous.
  const UNITS = [
    { unit: 'minute', confidence: 'derived_relative', re: /^(phút|phut|minutes?|mins?|min|m)$/i },
    { unit: 'hour', confidence: 'derived_relative', re: /^(giờ|gio|hours?|hrs?|hr|h)$/i },
    { unit: 'day', confidence: 'ambiguous', re: /^(ngày|ngay|days?|d)$/i },
    { unit: 'week', confidence: 'ambiguous', re: /^(tuần|tuan|weeks?|wks?|wk|w)$/i },
  ];

  // Coarse day-level words carry no count → always ambiguous (spec §4).
  const COARSE_DAY_RE = /(hôm qua|hom qua|yesterday)/i;
  // "Just now" text → the freshest possible post. Modelled as a 0-minute
  // relative age (derived_relative), i.e. posted within the last minute.
  const JUST_NOW_RE = /^(vừa xong|vua xong|just now|now)$/i;

  function toMs(now) {
    if (typeof now === 'number') return now;
    if (now instanceof Date) return now.getTime();
    return Date.parse(String(now));
  }

  function iso(ms) {
    return new Date(ms).toISOString();
  }

  const unknown = () => ({
    posted_at: null, confidence: 'unknown', earliest_utc: null,
    latest_utc: null, raw_unit: 'none', parser_version: PARSER_VERSION,
  });

  // Relative units truncate: "23 giờ" means age in [23h, 24h), so the post is
  // between `latest` (newest, the literal reading now-N) and `earliest`
  // (oldest, now-(N+1)). The freshness gate (later PR) judges the worst case.
  function relativeInterval(unit, qty, confidence, nowMs) {
    const step = UNIT_MS[unit];
    const latest = nowMs - qty * step;
    const earliest = nowMs - (qty + 1) * step;
    return {
      posted_at: iso(latest), confidence,
      earliest_utc: iso(earliest), latest_utc: iso(latest),
      raw_unit: unit, parser_version: PARSER_VERSION,
    };
  }

  // Pure: relative text → interval DTO, or null when nothing parses.
  function parseRelativeAge(text, nowMs) {
    const s = String(text || '').trim().toLowerCase();
    if (!s) return null;
    if (JUST_NOW_RE.test(s)) return relativeInterval('minute', 0, 'derived_relative', nowMs);
    if (COARSE_DAY_RE.test(s)) return relativeInterval('day', 1, 'ambiguous', nowMs);
    const m = s.match(/^(\d{1,3})\s*(\p{L}+)/u);
    if (!m) return null;
    const qty = Number(m[1]);
    const found = UNITS.find(u => u.re.test(m[2]));
    if (!found) return null;
    return relativeInterval(found.unit, qty, found.confidence, nowMs);
  }

  // Pure core: an extracted { exactUtc, relativeText } signal → canonical DTO.
  // exactUtc (machine-readable datetime) wins over relative text (spec §4).
  function classifyTimestampSignal(signal, now) {
    const nowMs = toMs(now);
    if (!Number.isFinite(nowMs)) return unknown();
    const exactMs = Date.parse(signal && signal.exactUtc);
    if (Number.isFinite(exactMs)) {
      const at = iso(exactMs);
      return {
        posted_at: at, confidence: 'exact', earliest_utc: at,
        latest_utc: at, raw_unit: 'date', parser_version: PARSER_VERSION,
      };
    }
    return parseRelativeAge(signal && signal.relativeText, nowMs) || unknown();
  }

  // Machine-readable datetime on the node, if any. FB classic: <abbr
  // data-utime> (epoch seconds); <time datetime> where present.
  function extractExactUtc(node) {
    if (!node || typeof node.querySelector !== 'function') return null;
    const stamped = node.querySelector('[data-utime]');
    if (stamped) {
      const secs = Number(stamped.getAttribute('data-utime'));
      if (Number.isFinite(secs) && secs > 0) return iso(secs * 1000);
    }
    const timeEl = node.querySelector('time[datetime]');
    if (timeEl) {
      const ms = Date.parse(timeEl.getAttribute('datetime'));
      if (Number.isFinite(ms)) return iso(ms);
    }
    return null;
  }

  // A short candidate string that the grammar itself accepts as a post age.
  // Reusing parseRelativeAge keeps ONE grammar for both selection and parse
  // (no drifting hint regex); the length cap avoids matching body sentences
  // that merely start with a number + unit word ("3 days ago at the beach").
  function looksLikeAge(text) {
    return text.length < 25 && parseRelativeAge(text, 0) !== null;
  }

  // First candidate text on the node that reads as a post age.
  function extractRelativeText(node) {
    if (!node || typeof node.querySelectorAll !== 'function') return '';
    for (const el of Array.from(node.querySelectorAll('a[href], abbr, span'))) {
      const txt = String((el.textContent || '')).trim();
      if (txt && looksLikeAge(txt)) return txt;
      const ariaTxt = String((el.getAttribute && el.getAttribute('aria-label')) || '').trim();
      if (ariaTxt && looksLikeAge(ariaTxt)) return ariaTxt;
    }
    return '';
  }

  // DOM adapter — reads ONLY the handed article node, delegates to the pure
  // core. This is the entry point crawl.js calls per article.
  function parsePostTimestamp(node, now) {
    return classifyTimestampSignal(
      { exactUtc: extractExactUtc(node), relativeText: extractRelativeText(node) },
      now,
    );
  }

  return Object.freeze({
    PARSER_VERSION, parsePostTimestamp, classifyTimestampSignal, parseRelativeAge,
  });
})();
// CommonJS export for the node test harness. No-op in the extension.
if (typeof module !== 'undefined' && module.exports) module.exports = globalThis.THGCrawlTime;
