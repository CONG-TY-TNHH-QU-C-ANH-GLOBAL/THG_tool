// THGCommentingResult — comment RESULT/proof plumbing (commentResult + forensics fold +
// target-not-reached override + abbreviate + editorContainsContent + submitDeps), split verbatim
// from commenting_outbound.js (Workstream A · PR7): move-only, behavior-preserving. Consumes
// THGOutboundDom; reads THGContentProof / THGForensics as bare globals at call time (preserved).
// Chrome: globalThis.THGCommentingResult (loaded after commenting_diag.js, before execute/*.js);
// Node: module.exports.
globalThis.THGCommentingResult = globalThis.THGCommentingResult || (() => {
  const THGDom = globalThis.THGOutboundDom
    || (typeof require === 'function' ? require('../../dom/outbound_dom.js') : null);
  if (!THGDom) {
    throw new Error('THGOutboundDom is required before execute/result.js');
  }
  const { norm, textOfEditable, labelOf, hasAny, visible, enabledButton } = THGDom;
  // Debug-gated swallow for best-effort browser calls (silent at normal runtime).
  function ignoreErr(e, ctx) { if (globalThis.__THG_COMMENTING_DEBUG__) console.debug(`[THGCommentingResult] ${ctx}`, e); }

  function editorContainsContent(editor, content) {
    if (!editor || !document.contains(editor)) return false;
    const current = norm(textOfEditable(editor)).replace(/\s+/g, ' ');
    const expected = norm(content).replace(/\s+/g, ' ');
    if (!expected) return false;
    const sample = expected.slice(0, Math.min(60, expected.length));
    return current.includes(sample);
  }

  // submitDeps threads outbound.js's shared DOM predicates into the extracted
  // THGCommentSubmit module (which owns findSubmitButtons / pressEnter / the reject
  // list) — keeps those primitives DRY without polluting globals.
  const submitDeps = { labelOf, norm, hasAny, visible, enabledButton };

  // abbreviate keeps identity-gate failure notes short. pfbid tokens run
  // ~60 chars; first 16 is enough to tell two entities apart in the
  // operator-replay UI without bloating evidence_json. Matches the
  // backend's abbreviateID convention so notes from the extension and
  // backend layers read uniformly.
  function abbreviate(id) {
    if (!id) return '<missing>';
    if (id.length <= 16) return id;
    return id.slice(0, 16) + '…';
  }

  // commentResult bundles the executor's verdict + the DOM proof the
  // backend's ClassifyExtensionReport consumes. ok=false routes through
  // the proof builder too so platform-reject banners (rate_limited /
  // blocked / redirected_feed) override the executor's generic error code.
  //
  // notes (optional, ok=false only): a short stage-specific reason
  // string for the operator-replay UI. Appended onto proof.notes after
  // the proof builder has set its own annotation, so platform-detected
  // failures (banner, redirect) still take precedence in the notes
  // ordering. Used by the identity-gate aborts in executeComment to
  // distinguish "no article on page" from "post-click swap" from
  // "editor drift" in the dashboard.
  // PR8C-Forensics: fold the content-script interaction timeline into the diagnostic, then
  // disarm. Only when this executeComment call armed the recorder (feed/rung2 paths do not).
  function foldForensicsInto(proof) {
    if (!(proof && globalThis.THGForensics && THGForensics.isArmed())) return;
    try {
      const snap = THGForensics.snapshot();
      proof.nav_diagnostic = proof.nav_diagnostic || {};
      proof.nav_diagnostic.forensics = snap;
    } catch (e) { ignoreErr(e, 'forensics'); } // forensics must never break delivery
    THGForensics.uninstall();
  }

  // PR8A: the pre-type landing gate is AUTHORITATIVE over the proof builder's post-submit
  // feedish heuristic — force target_not_reached UNLESS a real platform banner was detected
  // (more specific; keeps precedence). redirected_feed is the heuristic we override.
  function applyTargetNotReachedOverride(proof, errorCode) {
    if (!(proof && errorCode === 'target_not_reached')) return;
    const banner = proof.failure_reason && proof.failure_reason !== 'redirected_feed';
    if (!banner) proof.failure_reason = 'target_not_reached';
  }

  function commentResult(ok, errorCode, detail, ctx, notes, navDiag) {
    const proof = THGContentProof ? THGContentProof.buildCommentProof({
      ok, errorCode, content: ctx.content, userID: ctx.userID, preCount: ctx.preCount, duplicate: ctx.duplicate
    }) : null;
    if (proof && notes) {
      proof.notes = proof.notes ? proof.notes + ' · ' + notes : notes;
    }
    if (proof && navDiag) proof.nav_diagnostic = navDiag; // PR8A landing telemetry → evidence_json
    foldForensicsInto(proof);
    applyTargetNotReachedOverride(proof, errorCode);
    // Echo the server-issued execution_id back so the backend terminal-state CAS can gate on it.
    if (proof && ctx?.executionId) proof.execution_id = ctx.executionId;
    const base = ok
      ? { ok: true, detail: detail || 'sent_comment' }
      : { ok: false, error: errorCode || 'comment_failed' };
    return proof ? { ...base, proof } : base;
  }

  const api = { commentResult, abbreviate, editorContainsContent, submitDeps };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  return api;
})();
