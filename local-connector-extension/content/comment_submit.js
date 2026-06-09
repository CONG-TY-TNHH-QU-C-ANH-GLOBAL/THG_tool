// Comment submit machinery (Browser Automation Kit — comment executor extraction).
// The send-button finder + Enter fallback for the Facebook comment composer, moved
// out of the outbound.js god file so the "typed but not submitted" bug can be fixed
// here, in one place, instead of scattered across the executor paths.
//
// Refactor-only: functions moved VERBATIM from outbound.js. Shared DOM predicates
// (labelOf/norm/hasAny/visible/enabledButton) are threaded in via a `deps` object so
// the module stays DRY (no duplicated primitives) and unit-testable.
var THGCommentSubmit = globalThis.THGCommentSubmit || (() => {
  const SUBMIT_KEYS = ['comment', 'post', 'send', 'binh luan', 'dang', 'gui'];
  const REJECT_KEYS = [
    'share', 'like', 'cancel', 'photo', 'gif', 'emoji', 'sticker', 'anh', 'huy', 'thich', 'chia se',
    'nhan dan', 'bieu tuong cam xuc', 'cam xuc', 'avatar', 'may anh', 'hinh anh', 'dinh kem', 'tep', 'tap tin',
  ];

  function rejectActionLabel(label, d) { return d.hasAny(label, REJECT_KEYS); }

  function submitScore(editor, button, d) {
    const er = editor.getBoundingClientRect();
    const br = button.getBoundingClientRect();
    const ey = er.top + er.height / 2;
    const by = br.top + br.height / 2;
    let score = Math.abs(ey - by) + Math.max(0, er.left - br.left) / 3;
    const label = d.labelOf(button);
    const text = d.norm(button.innerText || '');
    if (!text) score -= 20;
    if (text && d.hasAny(text, ['comment', 'binh luan'])) score += 80;
    if (!d.hasAny(label, SUBMIT_KEYS)) score += 100;
    return score;
  }

  function submitCandidateSpatial(editor, button) {
    const er = editor.getBoundingClientRect();
    const br = button.getBoundingClientRect();
    const verticallyNear = br.bottom >= er.top - 28 && br.top <= er.bottom + 42;
    const toRight = br.left >= er.left - 10;
    const compact = br.width <= 110 && br.height <= 72;
    return verticallyNear && toRight && compact;
  }

  function findSubmitButtons(editor, excluded, d) {
    excluded = excluded || [];
    const scopes = [];
    const form = editor.closest('form');
    if (form) scopes.push(form);
    let parent = editor.parentElement;
    for (let i = 0; parent && i < 8; i += 1) { scopes.push(parent); parent = parent.parentElement; }
    scopes.push(editor.closest('[role="dialog"], [role="article"]') || document);
    const seen = new Set(excluded.filter(Boolean));
    const candidates = [];
    for (const scope of scopes) {
      if (!scope) continue;
      for (const el of Array.from(scope.querySelectorAll('div[role="button"], button, [aria-label]'))) {
        if (seen.has(el)) continue;
        seen.add(el);
        const label = d.labelOf(el);
        const hasSubmitLabel = d.hasAny(label, SUBMIT_KEYS);
        const spatial = submitCandidateSpatial(editor, el);
        if (!d.visible(el) || !d.enabledButton(el)) continue;
        if (label && rejectActionLabel(label, d)) continue;
        if (!hasSubmitLabel && !spatial) continue;
        if (el === editor || el.contains(editor)) continue;
        candidates.push(el);
      }
      if (candidates.length >= 3) break;
    }
    // Try LABELED submit buttons ("Bình luận"/"Gửi") before spatial-only toolbar
    // icons so the executor never clicks a sticker/avatar icon first.
    const labeled = candidates.filter(el => d.hasAny(d.labelOf(el), SUBMIT_KEYS));
    const spatialOnly = candidates.filter(el => !d.hasAny(d.labelOf(el), SUBMIT_KEYS));
    labeled.sort((a, b) => submitScore(editor, a, d) - submitScore(editor, b, d));
    spatialOnly.sort((a, b) => submitScore(editor, a, d) - submitScore(editor, b, d));
    return labeled.concat(spatialOnly).slice(0, 5);
  }

  function pressEnter(editor) {
    if (!editor) return false;
    try { editor.focus({ preventScroll: true }); } catch (_) { try { editor.focus(); } catch (_) {} }
    const init = { key: 'Enter', code: 'Enter', keyCode: 13, which: 13, bubbles: true, cancelable: true, composed: true };
    try {
      editor.dispatchEvent(new KeyboardEvent('keydown', init));
      editor.dispatchEvent(new KeyboardEvent('keypress', init));
      editor.dispatchEvent(new KeyboardEvent('keyup', init));
      return true;
    } catch (_) { return false; }
  }

  return { findSubmitButtons, pressEnter, _rejectActionLabel: rejectActionLabel, _submitCandidateSpatial: submitCandidateSpatial };
})();

if (typeof module !== 'undefined' && module.exports) module.exports = THGCommentSubmit;
