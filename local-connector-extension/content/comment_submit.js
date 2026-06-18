// Comment submit machinery (Browser Automation Kit — comment executor extraction).
// The send-button finder + settle gate + Enter fallback for the Facebook comment
// composer. Owns the answer to "which control is the real send button, and has it
// finished mounting?" so the executor never blind-clicks a pre-mount ghost node or
// a toolbar icon.
//
// Shared DOM predicates (labelOf/norm/hasAny/visible/enabledButton) are threaded in
// via a `deps` object; keyword sets + geometry + timings come from THGCommentConstants
// so nothing here is duplicated or a magic number.
var THGCommentSubmit = globalThis.THGCommentSubmit || (() => {
  const K = globalThis.THGCommentConstants
    || (typeof require === 'undefined' ? null : require('./comment_constants.js'));
  const { SUBMIT_KEYS, REJECT_KEYS, SPATIAL, TIMING } = K;

  function rejectActionLabel(label, d) { return d.hasAny(label, REJECT_KEYS); }

  // spatialDistance is the LEGITIMATE proximity signal: vertical-centre delta plus a
  // small penalty for sitting left of the editor. Lower = closer to the composer's
  // send corner. This is the only geometric term used for ranking — the old
  // submitScore additionally carried two INVERTED heuristics (it rewarded text-less
  // icon buttons and penalised the labelled submit button), which is exactly why the
  // executor used to click stickers/avatars before the real send control. Deleted.
  function spatialDistance(editor, button) {
    const er = editor.getBoundingClientRect();
    const br = button.getBoundingClientRect();
    const ey = er.top + er.height / 2;
    const by = br.top + br.height / 2;
    return Math.abs(ey - by) + Math.max(0, er.left - br.left) / SPATIAL.leftPenaltyDivisor;
  }

  // submitRank assigns a STRICT deterministic tier (lower = preferred):
  //   0 — labelled ("Bình luận"/"Send"/…) AND enabled → the real submit button
  //   1 — labelled but not currently enabled (text typed-but-not-flushed yet)
  //   2 — spatial-only (no submit label; a compact control next to the composer)
  // Replaces submitScore. Ties inside a tier break by spatial proximity, so the
  // ordering is total and reproducible.
  function submitRank(button, d) {
    if (!d.hasAny(d.labelOf(button), SUBMIT_KEYS)) return 2;
    return d.enabledButton(button) ? 0 : 1;
  }

  function compareSubmitCandidates(editor, a, b, d) {
    const rankDelta = submitRank(a, d) - submitRank(b, d);
    if (rankDelta !== 0) return rankDelta;
    return spatialDistance(editor, a) - spatialDistance(editor, b);
  }

  function submitCandidateSpatial(editor, button) {
    const er = editor.getBoundingClientRect();
    const br = button.getBoundingClientRect();
    const verticallyNear = br.bottom >= er.top - SPATIAL.aboveEditorPx && br.top <= er.bottom + SPATIAL.belowEditorPx;
    const toRight = br.left >= er.left - SPATIAL.leftSlackPx;
    const compact = br.width <= SPATIAL.maxWidthPx && br.height <= SPATIAL.maxHeightPx;
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
    // Single deterministic ordering: labelled-and-enabled, then labelled, then
    // spatial-only — each tier by spatial proximity. The executor never reaches a
    // sticker/avatar icon before the labelled send button.
    candidates.sort((a, b) => compareSubmitCandidates(editor, a, b, d));
    return candidates.slice(0, TIMING.maxSubmitCandidates);
  }

  // waitForStableSubmitTarget replaces the old fixed 150ms "React flush" guess.
  // After execCommand('insertText') activates Lexical's EditorState, Facebook
  // attaches / enables / RE-MOUNTS the real send button on a React flush whose
  // timing varies (50–200ms+). A fixed wait either clicks a pre-mount GHOST node
  // (too early) or wastes time (too late). Instead we poll findSubmitButtons until
  // the top-ranked candidate is the SAME element for settleStableMs continuous ms
  // (it has mounted and stopped being re-created), or until settleTimeoutMs. Returns
  // the settled button, or the freshest candidate at timeout (never blocks forever).
  async function waitForStableSubmitTarget(editor, excluded, d, opts) {
    const o = opts || {};
    const wait = o.wait;
    const now = o.now;
    const timeoutMs = typeof o.timeoutMs === 'number' ? o.timeoutMs : TIMING.settleTimeoutMs;
    const pollMs = typeof o.pollMs === 'number' ? o.pollMs : TIMING.settlePollMs;
    const stableMs = typeof o.stableMs === 'number' ? o.stableMs : TIMING.settleStableMs;
    const deadline = now() + timeoutMs;
    let last = null;
    let stableSince = 0;
    while (now() < deadline) {
      const top = findSubmitButtons(editor, excluded, d)[0] || null;
      if (top && top === last) {
        if (stableSince === 0) stableSince = now();
        if (now() - stableSince >= stableMs) return top;
      } else {
        last = top;
        stableSince = 0; // target changed (or vanished) — restart the stability window
      }
      await wait(pollMs);
    }
    return findSubmitButtons(editor, excluded, d)[0] || null;
  }

  function pressEnter(editor) {
    if (!editor) return false;
    try { editor.focus({ preventScroll: true }); } catch (_) { try { editor.focus(); } catch (_) { /* focus is best-effort */ } }
    const init = { key: 'Enter', code: 'Enter', keyCode: 13, which: 13, bubbles: true, cancelable: true, composed: true };
    try {
      editor.dispatchEvent(new KeyboardEvent('keydown', init));
      editor.dispatchEvent(new KeyboardEvent('keypress', init));
      editor.dispatchEvent(new KeyboardEvent('keyup', init));
      return true;
    } catch (_) { return false; }
  }

  return {
    findSubmitButtons, waitForStableSubmitTarget, pressEnter,
    _rejectActionLabel: rejectActionLabel, _submitCandidateSpatial: submitCandidateSpatial,
    _submitRank: submitRank, _spatialDistance: spatialDistance,
  };
})();
globalThis.THGCommentSubmit = THGCommentSubmit;

if (typeof module !== 'undefined' && module.exports) module.exports = THGCommentSubmit;
