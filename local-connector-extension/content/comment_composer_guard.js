// Comment Composer Guard — the SINGLE place that clears / inserts / equality-checks
// the Facebook comment composer (kept out of the outbound.js god file).
// Telemetry (0.5.45) proved: composer starts EMPTY, yet our OLD multi-method fallback
// chain inserted 3× (A+A+A = 191×3) because each method APPENDED and the read-back
// === gave a false-negative at matching length. So now: ONE insert method (no
// compounding) + Unicode-robust comparison (NFC + strip invisibles) + keyboard
// select-all so the insert REPLACES. Telemetry behind DEBUG_COMPOSER logs every step.
var THGCommentGuard = globalThis.THGCommentGuard || (() => {
  const wait = ms => new Promise(r => setTimeout(r, ms));

  const DEBUG_COMPOSER = true; // flip false to silence telemetry after the incident
  const EXT_VERSION = (() => { try { return chrome.runtime.getManifest().version; } catch (_) { return ''; } })();

  function clog(ctx, phase, f) {
    if (!DEBUG_COMPOSER) return;
    f = f || {};
    try {
      console.log('[THG composer]', {
        extension_version: EXT_VERSION, outbound_id: (ctx && ctx.outboundId) || 0,
        executor_path: (ctx && ctx.executorPath) || 'unknown', method: f.method || '-',
        expected_length: f.expected_length != null ? f.expected_length : -1,
        actual_length: f.actual_length != null ? f.actual_length : -1,
        equals_expected: !!f.equals_expected, is_duplicate: !!f.is_duplicate,
        phase, reason: f.reason || '', ...(f.extra || {}),
      });
    } catch (_) { /* never break delivery on a log */ }
  }

  // Unicode-robust: NFC-normalise + drop zero-width / soft-hyphen invisibles so a
  // composer rendering of A compares EQUAL to the queued A (the false-negative that
  // made the old chain over-insert).
  function normalizeCommentText(t) {
    return String(t == null ? '' : t).normalize('NFC').replace(/[​-‍﻿­]/g, '').replace(/\s+/g, ' ').trim();
  }

  function isExactRepeatedText(actual, expected) {
    const a = normalizeCommentText(actual);
    const e = normalizeCommentText(expected);
    if (!e || e.length < 6 || !a) return false;
    if (a === e + e || a === e + ' ' + e) return true;
    const first = a.indexOf(e);
    if (first !== -1 && a.indexOf(e, first + e.length) !== -1) return true;
    return a.length >= Math.floor(e.length * 1.6) && a.startsWith(e) && a.slice(e.length).includes(e.slice(0, Math.min(24, e.length)));
  }

  function readComposerText(composer) {
    if (!composer) return '';
    const raw = composer.isContentEditable || composer.innerText != null
      ? (composer.innerText != null ? composer.innerText : composer.textContent)
      : (composer.value != null ? composer.value : composer.textContent);
    return normalizeCommentText(raw);
  }

  function assertComposerExactlyExpected(composer, expected) {
    const actual = readComposerText(composer);
    const e = normalizeCommentText(expected);
    if (actual === e) return { ok: true, duplicate: false, mismatch: false, actual_length: actual.length, expected_length: e.length };
    const duplicate = isExactRepeatedText(actual, e);
    return { ok: false, duplicate, mismatch: !duplicate, actual_length: actual.length, expected_length: e.length };
  }

  // selectAllRobust: dispatch a Ctrl/Cmd+A keydown (Lexical's command handler selects
  // all in its MODEL — DOM execCommand('selectAll') alone may not sync Lexical), then
  // execCommand('selectAll') as belt-and-braces. Makes a following insert/delete act
  // on the WHOLE content (replace/clear) instead of appending.
  function selectAllRobust(el) {
    try { el.focus({ preventScroll: true }); } catch (_) { try { el.focus(); } catch (_) {} }
    const k = { key: 'a', code: 'KeyA', keyCode: 65, which: 65, ctrlKey: true, metaKey: true, bubbles: true, cancelable: true };
    try { el.dispatchEvent(new KeyboardEvent('keydown', k)); } catch (_) {}
    try { document.execCommand('selectAll', false, null); } catch (_) {}
  }

  // insertTextInto: ONE method only — select-all then execCommand('insertText') to
  // REPLACE. Never falls through to appending methods (that produced A+A+A). Returns
  // { method, actual }; the caller asserts + retries via clear.
  async function insertTextInto(composer, expected) {
    selectAllRobust(composer);
    // execCommand('insertText') ALREADY fires the native input event that Lexical
    // consumes to insert the text ONCE. Do NOT also dispatch a synthetic
    // InputEvent({inputType:'insertText', data: expected}) — Lexical reads its .data
    // and inserts the text a SECOND time → A+A (telemetry: 526 = 263×2 on one insert).
    try { document.execCommand('insertText', false, expected); } catch (_) {}
    await wait(160);
    return { method: 'kbd_selectall_insertText', actual: readComposerText(composer) };
  }

  // clearComposerUntilEmpty: settle (let FB re-mount any draft) → select-all + delete
  // until stable-empty.
  async function clearComposerUntilEmpty(composer, { maxRounds = 8, stableMs = 1000, settleMs = 600 } = {}) {
    try { composer.focus({ preventScroll: true }); } catch (_) { try { composer.focus(); } catch (_) {} }
    await wait(settleMs);
    for (let i = 0; i < maxRounds; i += 1) {
      if (readComposerText(composer).length === 0) {
        await wait(stableMs);
        if (readComposerText(composer).length === 0) return { ok: true, rounds: i };
      }
      selectAllRobust(composer);
      try { document.execCommand('delete', false, null); } catch (_) {}
      try { composer.dispatchEvent(new Event('input', { bubbles: true })); } catch (_) {}
      await wait(200);
    }
    return { ok: readComposerText(composer).length === 0, rounds: maxRounds };
  }

  async function waitForStableComposerText(composer, { stableMs = 800, timeoutMs = 3000 } = {}) {
    const start = Date.now();
    let last = readComposerText(composer);
    let stableSince = Date.now();
    while (Date.now() - start < timeoutMs) {
      await wait(150);
      const cur = readComposerText(composer);
      if (cur !== last) { last = cur; stableSince = Date.now(); }
      else if (Date.now() - stableSince >= stableMs) return cur;
    }
    return readComposerText(composer);
  }

  function snip(s) { s = String(s || ''); return { head: s.slice(0, 28), tail: s.slice(-28) }; }

  // prepareComposerForComment: each attempt CLEARS-OR-ABORTS then INSERTS once;
  // doubled/mismatch → clear + retry at most once, else abort.
  async function prepareComposerForComment(composer, expected, ctx = {}) {
    const exp = normalizeCommentText(expected);
    const ed = composer || {};
    const diag = { outbound_id: ctx.outboundId || 0, executor_path: ctx.executorPath || 'unknown', expected_length: exp.length, clear_attempts: 0, insert_attempts: 0 };
    clog(ctx, 'prepare_start', { expected_length: exp.length, actual_length: readComposerText(composer).length,
      extra: { editor_tag: ed.tagName, editor_class: String(ed.className || '').slice(0, 60), editor_editable: !!ed.isContentEditable } });

    const abort = async (reason) => { try { await clearComposerUntilEmpty(composer); } catch (_) {} return { ok: false, reason, diagnostic: diag }; };

    // 1. CLEAR-OR-ABORT — never insert into a composer that still holds text.
    const cleared = await clearComposerUntilEmpty(composer);
    diag.clear_attempts = cleared.rounds;
    diag.composer_empty_stable = cleared.ok;
    clog(ctx, 'after_clear', { expected_length: exp.length, actual_length: readComposerText(composer).length, equals_expected: cleared.ok, reason: cleared.ok ? '' : 'composer_clear_failed' });
    if (!cleared.ok) return { ok: false, reason: 'composer_clear_failed', diagnostic: diag };

    // 2. ONE insert (single method — no cumulative fallback).
    diag.insert_attempts = 1;
    const before_length = readComposerText(composer).length;
    const ins = await insertTextInto(composer, expected);
    await waitForStableComposerText(composer);
    const check = assertComposerExactlyExpected(composer, expected);
    diag.method = ins.method;
    diag.composer_after_insert_length = check.actual_length;
    diag.composer_after_insert_equals_expected = check.ok;
    diag.composer_after_insert_is_duplicate_of_expected = check.duplicate;
    const ea = snip(exp); const aa = snip(readComposerText(composer));
    clog(ctx, 'after_insert', { method: ins.method, expected_length: exp.length, actual_length: check.actual_length, equals_expected: check.ok, is_duplicate: check.duplicate,
      extra: { attempt_no: 1, before_length, after_length: check.actual_length, exp_head: ea.head, exp_tail: ea.tail, act_head: aa.head, act_tail: aa.tail } });

    // 3. STRICT: doubled / over-length / mismatch → abort, no fallback, no submit.
    if (check.duplicate || check.actual_length > exp.length) return abort('comment_text_doubled');
    if (check.mismatch) return abort('comment_text_mismatch');

    // 4. ok — LATE-RESTORE re-CHECK (re-assert only; never a second insert).
    await wait(900);
    const late = assertComposerExactlyExpected(composer, expected);
    diag.composer_after_settle_length = late.actual_length;
    diag.composer_after_settle_is_duplicate_of_expected = late.duplicate;
    clog(ctx, 'after_settle', { method: ins.method, expected_length: exp.length, actual_length: late.actual_length, equals_expected: late.ok, is_duplicate: late.duplicate });
    if (late.ok) return { ok: true, diagnostic: diag };
    return abort(late.duplicate || late.actual_length > exp.length ? 'comment_text_doubled' : 'comment_text_mismatch');
  }

  return {
    normalizeCommentText, isExactRepeatedText, readComposerText,
    assertComposerExactlyExpected, clearComposerUntilEmpty,
    waitForStableComposerText, prepareComposerForComment, insertTextInto,
  };
})();

if (typeof module !== 'undefined' && module.exports) module.exports = THGCommentGuard;
