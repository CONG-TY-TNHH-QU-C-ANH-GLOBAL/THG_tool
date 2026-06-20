// THGPostingOutbound — group/profile POST execution (executePost + postResult), extracted
// verbatim from outbound.js (Workstream A · PR3): move-only, behavior-preserving. Consumes
// generic primitives from THGOutboundDom and the proof builder from globalThis.THGContentProof
// (read at call time). No comment/inbox/identity/diagnostics/nav logic. Chrome:
// globalThis.THGPostingOutbound (manifest-loaded after outbound_dom.js, before outbound.js);
// Node: module.exports (with a Node-only _test seam).
globalThis.THGPostingOutbound = globalThis.THGPostingOutbound || (() => {
  // Chrome relies on manifest load order (outbound_dom.js first); Node tests use the guarded
  // CommonJS fallback. Fail loudly if the primitives module is missing.
  const THGDom = globalThis.THGOutboundDom
    || (typeof require === 'function' ? require('./outbound_dom.js') : null);
  if (!THGDom) {
    throw new Error('THGOutboundDom is required before posting_outbound.js');
  }
  const { dismissBlockingOverlays, visible, labelOf, hasAny, clickLikeUser, wait, norm, setEditableText } = THGDom;

  async function executePost(content, executionId = '') {
    await dismissBlockingOverlays();
    const composerKeys = ["what's on your mind", 'write something', 'create a public post', 'ban dang nghi gi', 'viet gi do'];
    const postKeys = ['post', 'dang'];
    const ctx = { content, executionId };
    const composer = Array.from(document.querySelectorAll('div[role="button"], button, textarea, [contenteditable="true"], [aria-label]'))
      .filter(el => visible(el))
      .find(el => hasAny(labelOf(el), composerKeys));
    if (!composer || !clickLikeUser(composer)) return postResult(false, 'post_composer_not_found', null, ctx);
    await wait(1500);
    const editors = Array.from(document.querySelectorAll('[contenteditable="true"], textarea')).filter(el => visible(el));
    let editor = editors.find(el => norm(el.getAttribute('role')) === 'textbox') || editors.at(-1);
    if (!editor) return postResult(false, 'post_editor_not_found', null, ctx);
    if (!setEditableText(editor, content)) return postResult(false, 'post_text_insert_failed', null, ctx);
    await wait(900);
    const scope = editor.closest('[role="dialog"], form') || document;
    const postButton = Array.from(scope.querySelectorAll('div[role="button"], button, [aria-label]')).filter(el => visible(el)).reverse().find(el => {
      const label = labelOf(el);
      return hasAny(label, postKeys) && !label.includes('comment') && !label.includes('cancel') &&
        el.getAttribute('aria-disabled') !== 'true' && !el.disabled;
    });
    if (!postButton || !clickLikeUser(postButton)) return postResult(false, 'post_submit_not_found', null, ctx);
    // Generous settle — posting closes the composer dialog and re-renders
    // the feed; we need both to complete before walking the DOM for proof.
    await wait(2500);
    return postResult(true, '', 'sent_post_button', ctx);
  }

  function postResult(ok, errorCode, detail, ctx) {
    // Read the proof builder from globalThis at call time (not captured at init), matching the
    // original bare-global reference; absent builder → no proof (same shape as before).
    const P = globalThis.THGContentProof;
    const proof = P?.buildPostProof({
      ok, errorCode, content: ctx.content
    }) ?? null;
    const executionId = ctx?.executionId;
    if (proof && executionId) {
      proof.execution_id = executionId;
    }
    const base = ok
      ? { ok: true, detail: detail || 'sent_post' }
      : { ok: false, error: errorCode || 'post_failed' };
    return proof ? { ...base, proof } : base;
  }

  const api = { executePost };
  if (typeof module !== 'undefined' && module.exports) module.exports = { ...api, _test: { executePost, postResult } };
  return api;
})();
