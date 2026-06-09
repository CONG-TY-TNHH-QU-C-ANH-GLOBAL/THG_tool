// Comment Composer Guard (incident PR-1). The SINGLE place that owns clearing,
// inserting, stabilising and EQUALITY-CHECKING the Facebook comment composer, so no
// comment path can submit a doubled / mismatched composer. Kept out of the
// outbound.js god file: that file only CALLS these helpers.
//
// Pure helpers (normalizeCommentText / isExactRepeatedText / readComposerText /
// assertComposerExactlyExpected) are unit-tested in isolation. The DOM helpers are
// self-contained (use document/window directly) so any path can reuse them.
var THGCommentGuard = globalThis.THGCommentGuard || (() => {
  const wait = ms => new Promise(r => setTimeout(r, ms));

  function normalizeCommentText(t) {
    return String(t == null ? '' : t).replace(/\s+/g, ' ').trim();
  }

  // isExactRepeatedText: actual is expected repeated back-to-back (A+A), with or
  // without a separator between the copies.
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

  function selectAllIn(el) {
    try {
      const r = document.createRange();
      r.selectNodeContents(el);
      const s = window.getSelection();
      s.removeAllRanges();
      s.addRange(r);
      return true;
    } catch (_) {
      try { return document.execCommand('selectAll', false, null); } catch (_) { return false; }
    }
  }

  function insertTextInto(composer, text) {
    selectAllIn(composer);
    try { document.execCommand('insertText', false, text); } catch (_) { try { composer.textContent = text; } catch (_) {} }
    try { composer.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText', data: text })); }
    catch (_) { try { composer.dispatchEvent(new Event('input', { bubbles: true })); } catch (_) {} }
    try { composer.dispatchEvent(new Event('change', { bubbles: true })); } catch (_) {}
  }

  // clearComposerUntilEmpty: FB/Lexical can restore a draft after a single delete.
  // Clear, then require the editor to stay empty for stableMs before declaring it
  // clear; repeat up to maxRounds. Returns { ok, rounds }.
  async function clearComposerUntilEmpty(composer, { maxRounds = 6, stableMs = 800 } = {}) {
    try { composer.focus({ preventScroll: true }); } catch (_) { try { composer.focus(); } catch (_) {} }
    for (let i = 0; i < maxRounds; i += 1) {
      if (readComposerText(composer).length === 0) {
        await wait(stableMs);
        if (readComposerText(composer).length === 0) return { ok: true, rounds: i };
      }
      selectAllIn(composer);
      try { document.execCommand('delete', false, null); } catch (_) {}
      try { composer.dispatchEvent(new Event('input', { bubbles: true })); } catch (_) {}
      await wait(180);
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

  // prepareComposerForComment: clear → insert → stabilise → equality. On a doubled
  // insert it clears + re-inserts ONCE; never returns ok for a doubled/mismatched
  // composer. Returns { ok, reason, diagnostic }.
  async function prepareComposerForComment(composer, expected) {
    const diag = { clear_attempts: 0, insert_attempts: 0 };
    const cleared = await clearComposerUntilEmpty(composer);
    diag.clear_attempts = cleared.rounds;
    diag.composer_empty_stable = cleared.ok;
    if (!cleared.ok) return { ok: false, reason: 'composer_clear_failed', diagnostic: diag };

    for (let attempt = 1; attempt <= 2; attempt += 1) {
      diag.insert_attempts = attempt;
      insertTextInto(composer, expected);
      await waitForStableComposerText(composer);
      const check = assertComposerExactlyExpected(composer, expected);
      diag.composer_after_insert_length = check.actual_length;
      diag.composer_after_insert_equals_expected = check.ok;
      diag.composer_after_insert_is_duplicate_of_expected = check.duplicate;
      if (check.ok) return { ok: true, diagnostic: diag };
      if (check.mismatch) return { ok: false, reason: 'comment_text_mismatch', diagnostic: diag };
      // duplicate → clear and retry once
      await clearComposerUntilEmpty(composer);
    }
    return { ok: false, reason: 'comment_text_doubled', diagnostic: diag };
  }

  return {
    normalizeCommentText, isExactRepeatedText, readComposerText,
    assertComposerExactlyExpected, clearComposerUntilEmpty,
    waitForStableComposerText, prepareComposerForComment,
  };
})();

if (typeof module !== 'undefined' && module.exports) module.exports = THGCommentGuard;
