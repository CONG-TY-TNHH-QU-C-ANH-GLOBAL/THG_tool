// THGInboxOutbound — inbox/Messenger message execution (executeInbox + inboxResult),
// extracted verbatim from outbound.js (Workstream A · PR4): move-only, behavior-preserving.
// Consumes generic primitives from THGOutboundDom and the proof builder from
// globalThis.THGContentProof (read at call time). No comment/posting/identity/diagnostics/nav
// logic. Chrome: globalThis.THGInboxOutbound (manifest-loaded after posting_outbound.js,
// before outbound.js); Node: module.exports (with a Node-only _test seam).
globalThis.THGInboxOutbound = globalThis.THGInboxOutbound || (() => {
  // Chrome relies on manifest load order (outbound_dom.js first); Node tests use the guarded
  // CommonJS fallback. Fail loudly if the primitives module is missing.
  const THGDom = globalThis.THGOutboundDom
    || (typeof require === 'function' ? require('./outbound_dom.js') : null);
  if (!THGDom) {
    throw new Error('THGOutboundDom is required before inbox_outbound.js');
  }
  const { dismissBlockingOverlays, visible, hasAny, labelOf, clickLikeUser, wait, norm, setEditableText } = THGDom;

  async function executeInbox(content, executionId = '') {
    await dismissBlockingOverlays();
    const proof = globalThis.THGContentProof || null;
    // Snapshot the last bubble pre-submit so the proof builder can detect
    // whether a NEW bubble appeared (vs. an existing one already matching
    // our text — the duplicate / idempotent case).
    const preBubbleHash = proof ? proof.snapshotLastBubble() : '';
    const ctx = { content, preBubbleHash, executionId };

    const messageKeys = ['message', 'messenger', 'send message', 'nhan tin'];
    const sendKeys = ['send', 'press enter to send', 'gui'];
    let editors = Array.from(document.querySelectorAll('[contenteditable="true"], textarea')).filter(el => visible(el));
    if (!editors.length) {
      const messageButton = Array.from(document.querySelectorAll('div[role="button"], button, a[role="button"]')).filter(el => visible(el))
        .find(el => hasAny(labelOf(el), messageKeys));
      if (!messageButton || !clickLikeUser(messageButton)) return inboxResult(false, 'message_button_not_found', null, ctx);
      await wait(1800);
      editors = Array.from(document.querySelectorAll('[contenteditable="true"], textarea')).filter(el => visible(el));
    }
    let editor = editors.find(el => hasAny(labelOf(el), messageKeys) || norm(el.getAttribute('role')) === 'textbox');
    if (!editor) editor = editors.at(-1);
    if (!editor) return inboxResult(false, 'message_box_not_found', null, ctx);
    if (!setEditableText(editor, content)) return inboxResult(false, 'inbox_text_insert_failed', null, ctx);
    await wait(700);
    const scope = editor.closest('[role="dialog"], form, div[aria-label]') || document;
    const send = Array.from(scope.querySelectorAll('div[role="button"], button, [aria-label]')).filter(el => visible(el)).find(el => {
      const label = labelOf(el);
      return hasAny(label, sendKeys) && el.getAttribute('aria-disabled') !== 'true' && !el.disabled;
    });
    if (!send || !clickLikeUser(send)) return inboxResult(false, 'inbox_submit_not_found', null, ctx);
    // Longer settle for bubble + timestamp to render — FB animates the
    // bubble in, and "Just now" copy can lag the bubble itself.
    await wait(1500);
    return inboxResult(true, '', 'sent_inbox_button', ctx);
  }

  function inboxResult(ok, errorCode, detail, ctx) {
    // Read the proof builder from globalThis at call time (matches the original bare-global,
    // call-time reference); absent builder → no proof (same shape as before).
    const P = globalThis.THGContentProof;
    const proof = P?.buildInboxProof({
      ok, errorCode, content: ctx.content, preBubbleHash: ctx.preBubbleHash
    }) ?? null;
    const executionId = ctx?.executionId;
    if (proof && executionId) {
      proof.execution_id = executionId;
    }
    const base = ok
      ? { ok: true, detail: detail || 'sent_inbox' }
      : { ok: false, error: errorCode || 'inbox_failed' };
    return proof ? { ...base, proof } : base;
  }

  const api = { executeInbox };
  if (typeof module !== 'undefined' && module.exports) module.exports = { ...api, _test: { executeInbox, inboxResult } };
  return api;
})();
