// Comment Composer Guard — the SINGLE place that clears / inserts / equality-checks
// the Facebook comment composer (kept out of the outbound.js god file).
// FB's composer is Lexical: a Range selection does NOT drive execCommand insert/delete
// (insert APPENDS, delete no-ops → the visible A+A). So we select via browser-native
// execCommand('selectAll') and insert via a VERIFIED fallback chain (execCommand
// insertText → paste-replacement → textContent), reading the composer back after each
// method. Telemetry behind DEBUG_COMPOSER logs every step.
var THGCommentGuard = globalThis.THGCommentGuard || (() => {
  const wait = ms => new Promise(r => setTimeout(r, ms));

  // Flip to false to silence all composer telemetry after the incident.
  const DEBUG_COMPOSER = true;
  const EXT_VERSION = (() => { try { return chrome.runtime.getManifest().version; } catch (_) { return ''; } })();

  function clog(ctx, phase, f) {
    if (!DEBUG_COMPOSER) return;
    f = f || {};
    try {
      console.log('[THG composer]', {
        extension_version: EXT_VERSION,
        outbound_id: (ctx && ctx.outboundId) || 0,
        executor_path: (ctx && ctx.executorPath) || 'unknown',
        method: f.method || '-',
        expected_length: f.expected_length != null ? f.expected_length : -1,
        actual_length: f.actual_length != null ? f.actual_length : -1,
        equals_expected: !!f.equals_expected,
        is_duplicate: !!f.is_duplicate,
        phase,
        reason: f.reason || '',
        ...(f.extra || {}),
      });
    } catch (_) { /* never break delivery on a log */ }
  }

  function normalizeCommentText(t) {
    return String(t == null ? '' : t).replace(/\s+/g, ' ').trim();
  }

  // isExactRepeatedText: actual is expected repeated back-to-back (A+A).
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

  // assertComposerExactlyExpected: the hard pre-submit equality check.
  function assertComposerExactlyExpected(composer, expected) {
    const actual = readComposerText(composer);
    const e = normalizeCommentText(expected);
    if (actual === e) return { ok: true, duplicate: false, mismatch: false, actual_length: actual.length, expected_length: e.length };
    const duplicate = isExactRepeatedText(actual, e);
    return { ok: false, duplicate, mismatch: !duplicate, actual_length: actual.length, expected_length: e.length };
  }

  // selectAllIn: browser-native selectAll FIRST (drives Lexical's edit pipeline);
  // Range is only a last resort.
  function selectAllIn(el) {
    try { el.focus({ preventScroll: true }); } catch (_) { try { el.focus(); } catch (_) {} }
    try { if (document.execCommand('selectAll', false, null)) return true; } catch (_) {}
    try {
      const r = document.createRange();
      r.selectNodeContents(el);
      const s = window.getSelection();
      s.removeAllRanges();
      s.addRange(r);
      return true;
    } catch (_) { return false; }
  }

  function pasteInto(composer, text) {
    selectAllIn(composer);
    try {
      const dt = new DataTransfer();
      dt.setData('text/plain', text);
      composer.dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }));
      return true;
    } catch (_) { return false; }
  }

  // insertTextInto: verified fallback chain — each method reads the composer back and
  // a method only "counts" if the read-back equals expected. Returns { method, actual }.
  async function insertTextInto(composer, expected) {
    const exp = normalizeCommentText(expected);
    selectAllIn(composer);
    try { document.execCommand('insertText', false, expected); } catch (_) {}
    try { composer.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText', data: expected })); } catch (_) {}
    await wait(150);
    if (readComposerText(composer) === exp) return { method: 'execCommand_insertText', actual: exp };

    pasteInto(composer, expected);
    await wait(180);
    if (readComposerText(composer) === exp) return { method: 'paste', actual: exp };

    try {
      selectAllIn(composer);
      document.execCommand('delete', false, null);
      composer.textContent = expected;
      composer.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText', data: expected }));
    } catch (_) {}
    await wait(120);
    return { method: 'textContent_fallback', actual: readComposerText(composer) };
  }

  // clearComposerUntilEmpty: settle (let FB re-mount its draft) → selectAll+delete +
  // paste-empty (Lexical belt-and-braces), require stable-empty before declaring ok.
  async function clearComposerUntilEmpty(composer, { maxRounds = 8, stableMs = 1000, settleMs = 600 } = {}) {
    try { composer.focus({ preventScroll: true }); } catch (_) { try { composer.focus(); } catch (_) {} }
    await wait(settleMs);
    for (let i = 0; i < maxRounds; i += 1) {
      if (readComposerText(composer).length === 0) {
        await wait(stableMs);
        if (readComposerText(composer).length === 0) return { ok: true, rounds: i };
      }
      selectAllIn(composer);
      try { document.execCommand('delete', false, null); } catch (_) {}
      pasteInto(composer, '');
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

  // prepareComposerForComment: each attempt CLEARS-OR-ABORTS then INSERTS (verified);
  // a doubled result clears + retries at most once, else aborts. Never inserts into a
  // non-cleared composer; never returns ok for a doubled/mismatched composer.
  async function prepareComposerForComment(composer, expected, ctx = {}) {
    const exp = normalizeCommentText(expected);
    const ed = composer || {};
    const diag = { outbound_id: ctx.outboundId || 0, executor_path: ctx.executorPath || 'unknown', expected_length: exp.length, clear_attempts: 0, insert_attempts: 0 };
    clog(ctx, 'prepare_start', { expected_length: exp.length, actual_length: readComposerText(composer).length,
      extra: { editor_tag: ed.tagName, editor_class: String(ed.className || '').slice(0, 60), editor_editable: !!ed.isContentEditable } });

    for (let attempt = 1; attempt <= 2; attempt += 1) {
      const cleared = await clearComposerUntilEmpty(composer);
      diag.clear_attempts += cleared.rounds;
      diag.composer_empty_stable = cleared.ok;
      clog(ctx, 'after_clear', { expected_length: exp.length, actual_length: readComposerText(composer).length, equals_expected: cleared.ok, reason: cleared.ok ? '' : 'composer_clear_failed' });
      if (!cleared.ok) return { ok: false, reason: 'composer_clear_failed', diagnostic: diag };

      diag.insert_attempts = attempt;
      const ins = await insertTextInto(composer, expected);
      await waitForStableComposerText(composer);
      const check = assertComposerExactlyExpected(composer, expected);
      diag.method = ins.method;
      diag.composer_after_insert_length = check.actual_length;
      diag.composer_after_insert_equals_expected = check.ok;
      diag.composer_after_insert_is_duplicate_of_expected = check.duplicate;
      clog(ctx, 'after_insert', { method: ins.method, expected_length: exp.length, actual_length: check.actual_length, equals_expected: check.ok, is_duplicate: check.duplicate });

      if (check.mismatch) return { ok: false, reason: 'comment_text_mismatch', diagnostic: diag };
      if (check.ok) {
        await wait(900); // late-restore guard — FB can re-mount the draft a beat later
        const late = assertComposerExactlyExpected(composer, expected);
        diag.composer_after_settle_length = late.actual_length;
        diag.composer_after_settle_is_duplicate_of_expected = late.duplicate;
        clog(ctx, 'after_settle', { method: ins.method, expected_length: exp.length, actual_length: late.actual_length, equals_expected: late.ok, is_duplicate: late.duplicate });
        if (late.ok) return { ok: true, diagnostic: diag };
        if (late.mismatch) return { ok: false, reason: 'comment_text_mismatch', diagnostic: diag };
      }
      // doubled (immediate or late) → loop clears + retries once
    }
    clog(ctx, 'doubled_abort', { method: diag.method, expected_length: exp.length, actual_length: diag.composer_after_insert_length, is_duplicate: true, reason: 'comment_text_doubled' });
    return { ok: false, reason: 'comment_text_doubled', diagnostic: diag };
  }

  return {
    normalizeCommentText, isExactRepeatedText, readComposerText,
    assertComposerExactlyExpected, clearComposerUntilEmpty,
    waitForStableComposerText, prepareComposerForComment, insertTextInto,
  };
})();

if (typeof module !== 'undefined' && module.exports) module.exports = THGCommentGuard;
