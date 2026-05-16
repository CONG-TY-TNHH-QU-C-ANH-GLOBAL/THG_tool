var THGContentOutbound = globalThis.THGContentOutbound || (() => {
  const wait = (ms) => new Promise(resolve => setTimeout(resolve, ms));
  const norm = (value) => String(value || '')
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[đĐ]/g, 'd')
    .trim()
    .toLowerCase();
  const hasAny = (value, keys) => keys.some(key => value.includes(key));

  function visible(el) {
    if (!el) return false;
    const rect = el.getBoundingClientRect();
    const style = getComputedStyle(el);
    return rect.width > 8 && rect.height > 8 && style.visibility !== 'hidden' && style.display !== 'none';
  }

  function labelOf(el) {
    return norm(el?.innerText || el?.getAttribute?.('aria-label') || el?.getAttribute?.('placeholder') || el?.title);
  }

  function eventInit(x, y, extra = {}) {
    return {
      bubbles: true,
      cancelable: true,
      composed: true,
      clientX: x,
      clientY: y,
      ...extra
    };
  }

  function dispatchMouseLike(el, type, x, y, extra = {}) {
    try {
      el.dispatchEvent(new MouseEvent(type, eventInit(x, y, extra)));
    } catch (_) {}
  }

  function dispatchPointerLike(el, type, x, y, extra = {}) {
    try {
      el.dispatchEvent(new PointerEvent(type, eventInit(x, y, {
        pointerId: 1,
        pointerType: 'mouse',
        isPrimary: true,
        button: 0,
        buttons: type.endsWith('down') ? 1 : 0,
        ...extra
      })));
    } catch (_) {}
  }

  function clickLikeUser(el) {
    if (!el) return false;
    try { el.scrollIntoView({ block: 'center', inline: 'center' }); } catch (_) {}
    const rect = el.getBoundingClientRect();
    const x = Math.max(0, Math.min(window.innerWidth - 1, rect.left + rect.width / 2));
    const y = Math.max(0, Math.min(window.innerHeight - 1, rect.top + rect.height / 2));
    try {
      dispatchPointerLike(el, 'pointerover', x, y);
      dispatchPointerLike(el, 'pointermove', x, y);
      dispatchPointerLike(el, 'pointerdown', x, y);
      dispatchMouseLike(el, 'mousedown', x, y);
      dispatchPointerLike(el, 'pointerup', x, y);
      dispatchMouseLike(el, 'mouseup', x, y);
      dispatchMouseLike(el, 'click', x, y);
      el.click();
      return true;
    } catch (_) {
      try { el.click(); return true; } catch (_) { return false; }
    }
  }

  async function dismissBlockingOverlays() {
    const labels = ['not now', 'ok', 'close', 'later', 'maybe later', 'remember password', 'de sau', 'luc khac', 'khong phai bay gio'];
    const candidates = Array.from(document.querySelectorAll('div[role="button"], button, [aria-label]')).filter(visible);
    for (const el of candidates) {
      const label = labelOf(el);
      if (!label) continue;
      if (labels.some(key => label.includes(key))) {
        if (clickLikeUser(el)) {
          await wait(500);
          return 'clicked';
        }
      }
    }
    return 'none';
  }

  function textOfEditable(editor) {
    if (!editor) return '';
    if ('value' in editor) return String(editor.value || '');
    return String(editor.innerText || editor.textContent || '');
  }

  function setInputValue(editor, value) {
    const proto = editor instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
    if (setter) setter.call(editor, value);
    else editor.value = value;
  }

  function selectEditableContents(editor) {
    try {
      const range = document.createRange();
      range.selectNodeContents(editor);
      const selection = window.getSelection();
      selection.removeAllRanges();
      selection.addRange(range);
      return true;
    } catch (_) {
      try {
        document.execCommand('selectAll', false, null);
        return true;
      } catch (_) {
        return false;
      }
    }
  }

  function emitEditableInput(editor, text = '') {
    try {
      editor.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText', data: text }));
    } catch (_) {
      editor.dispatchEvent(new Event('input', { bubbles: true }));
    }
    try { editor.dispatchEvent(new Event('change', { bubbles: true })); } catch (_) {}
  }

  function setEditableText(editor, text) {
    try { editor.focus({ preventScroll: true }); } catch (_) { try { editor.focus(); } catch (_) {} }
    if (editor.isContentEditable) {
      selectEditableContents(editor);
      document.execCommand('insertText', false, text);
    } else if ('value' in editor) {
      setInputValue(editor, text);
    } else {
      return false;
    }
    emitEditableInput(editor, text);
    return true;
  }

  async function waitFor(predicate, timeoutMs = 2500, stepMs = 150) {
    const started = Date.now();
    while (Date.now() - started < timeoutMs) {
      if (predicate()) return true;
      await wait(stepMs);
    }
    return predicate();
  }

  function editorContainsContent(editor, content) {
    if (!editor || !document.contains(editor)) return false;
    const current = norm(textOfEditable(editor)).replace(/\s+/g, ' ');
    const expected = norm(content).replace(/\s+/g, ' ');
    if (!expected) return false;
    const sample = expected.slice(0, Math.min(60, expected.length));
    return current.includes(sample);
  }

  function pressEnter(editor) {
    if (!editor) return false;
    try { editor.focus({ preventScroll: true }); } catch (_) { try { editor.focus(); } catch (_) {} }
    const init = {
      key: 'Enter',
      code: 'Enter',
      keyCode: 13,
      which: 13,
      bubbles: true,
      cancelable: true,
      composed: true
    };
    try {
      editor.dispatchEvent(new KeyboardEvent('keydown', init));
      editor.dispatchEvent(new KeyboardEvent('keypress', init));
      editor.dispatchEvent(new KeyboardEvent('keyup', init));
      return true;
    } catch (_) {
      return false;
    }
  }

  function enabledButton(el) {
    return el && el.getAttribute?.('aria-disabled') !== 'true' && !el.disabled;
  }

  function rejectActionLabel(label) {
    return hasAny(label, ['share', 'like', 'cancel', 'photo', 'gif', 'emoji', 'sticker', 'anh', 'huy', 'thich', 'chia se']);
  }

  function findCommentEditor(scope) {
    const commentKeys = ['comment', 'write a comment', 'binh luan', 'viet binh luan'];
    const badKeys = ['search', 'tim kiem', 'message', 'messenger', 'nhan tin'];
    const root = scope || document;
    const editors = Array.from(root.querySelectorAll('[contenteditable="true"], textarea, input[type="text"]'))
      .filter(el => visible(el) && !hasAny(labelOf(el), badKeys));
    return editors.find(el => hasAny(labelOf(el), commentKeys))
      || editors.find(el => norm(el.getAttribute('role')) === 'textbox')
      || editors[0]
      || null;
  }

  function submitScore(editor, button) {
    const er = editor.getBoundingClientRect();
    const br = button.getBoundingClientRect();
    const ey = er.top + er.height / 2;
    const by = br.top + br.height / 2;
    let score = Math.abs(ey - by) + Math.max(0, er.left - br.left) / 3;
    const label = labelOf(button);
    const text = norm(button.innerText || '');
    if (!text) score -= 20;
    if (text && hasAny(text, ['comment', 'binh luan'])) score += 80;
    if (!hasAny(label, ['comment', 'post', 'send', 'binh luan', 'dang', 'gui'])) score += 100;
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

  function findSubmitButtons(editor, excluded = []) {
    const submitKeys = ['comment', 'post', 'send', 'binh luan', 'dang', 'gui'];
    const scopes = [];
    const form = editor.closest('form');
    if (form) scopes.push(form);
    let parent = editor.parentElement;
    for (let i = 0; parent && i < 8; i += 1) {
      scopes.push(parent);
      parent = parent.parentElement;
    }
    scopes.push(editor.closest('[role="dialog"], [role="article"]') || document);
    const seen = new Set(excluded.filter(Boolean));
    const candidates = [];
    for (const scope of scopes) {
      if (!scope) continue;
      for (const el of Array.from(scope.querySelectorAll('div[role="button"], button, [aria-label]'))) {
        if (seen.has(el)) continue;
        seen.add(el);
        const label = labelOf(el);
        const hasSubmitLabel = hasAny(label, submitKeys);
        const spatial = submitCandidateSpatial(editor, el);
        if (!visible(el) || !enabledButton(el)) continue;
        if (label && rejectActionLabel(label)) continue;
        if (!hasSubmitLabel && !spatial) continue;
        if (el === editor || el.contains(editor)) continue;
        candidates.push(el);
      }
      if (candidates.length >= 3) break;
    }
    candidates.sort((a, b) => submitScore(editor, a) - submitScore(editor, b));
    return candidates.slice(0, 5);
  }

  async function executeComment(content) {
    await dismissBlockingOverlays();

    // Step 3b — snapshot pre-submit state so the proof builder can
    // compute count_increased and duplicate without relying on the
    // executor's own beliefs.
    const proof = THGContentProof || null;
    const fbUID = proof?.currentFBUserID() || '';
    const preCount = proof ? proof.snapshotCommentCount() : 0;
    const preMatched = proof ? !!proof.findCommentNode(content, fbUID) : false;

    const commentKeys = ['comment', 'write a comment', 'binh luan', 'viet binh luan'];
    const buttons = Array.from(document.querySelectorAll('div[role="button"], button, a[role="button"], span[role="button"]')).filter(visible);
    const commentButton = buttons.find(el => {
      const label = labelOf(el);
      return hasAny(label, commentKeys) && !label.includes('share') && !label.includes('like');
    });
    if (commentButton) {
      clickLikeUser(commentButton);
      await wait(900);
    }
    const scope = commentButton?.closest('[role="article"], [role="dialog"]') || document;
    let editor = findCommentEditor(scope);
    if (!editor) {
      window.scrollBy({ top: 420, behavior: 'smooth' });
      await wait(900);
      editor = findCommentEditor(scope) || findCommentEditor(document);
    }
    if (!editor) return commentResult(false, 'comment_box_not_found', null, { content, userID: fbUID, preCount, duplicate: preMatched });
    if (!setEditableText(editor, content)) return commentResult(false, 'comment_text_insert_failed', null, { content, userID: fbUID, preCount, duplicate: preMatched });
    const inserted = await waitFor(() => editorContainsContent(editor, content), 1800, 150);
    if (!inserted) return commentResult(false, 'comment_text_not_confirmed', null, { content, userID: fbUID, preCount, duplicate: preMatched });

    const submitButtons = findSubmitButtons(editor, [commentButton]);
    for (const submit of submitButtons) {
      if (submit && clickLikeUser(submit)) {
        const cleared = await waitFor(() => !editorContainsContent(editor, content), 7000, 250);
        if (cleared) {
          // Settle delay — give the DOM a moment to render the new comment
          // node before we walk it for proof. Without this, the verifier
          // often misses the node and downgrades to optimistic_success.
          await wait(700);
          return commentResult(true, '', 'sent_comment_button', { content, userID: fbUID, preCount, duplicate: preMatched });
        }
      }
      await wait(400);
    }

    if (pressEnter(editor)) {
      const cleared = await waitFor(() => !editorContainsContent(editor, content), 7000, 250);
      if (cleared) {
        await wait(700);
        return commentResult(true, '', 'sent_comment_enter', { content, userID: fbUID, preCount, duplicate: preMatched });
      }
    }

    return commentResult(false, submitButtons.length ? 'comment_submit_not_confirmed' : 'comment_submit_not_found', null, { content, userID: fbUID, preCount, duplicate: preMatched });
  }

  // commentResult bundles the executor's verdict + the DOM proof the
  // backend's ClassifyExtensionReport consumes. ok=false routes through
  // the proof builder too so platform-reject banners (rate_limited /
  // blocked / redirected_feed) override the executor's generic error code.
  function commentResult(ok, errorCode, detail, ctx) {
    const proof = THGContentProof ? THGContentProof.buildCommentProof({
      ok, errorCode, content: ctx.content, userID: ctx.userID, preCount: ctx.preCount, duplicate: ctx.duplicate
    }) : null;
    const base = ok
      ? { ok: true, detail: detail || 'sent_comment' }
      : { ok: false, error: errorCode || 'comment_failed' };
    return proof ? { ...base, proof } : base;
  }

  async function executeInbox(content) {
    await dismissBlockingOverlays();
    const proof = THGContentProof || null;
    // Snapshot the last bubble pre-submit so the proof builder can detect
    // whether a NEW bubble appeared (vs. an existing one already matching
    // our text — the duplicate / idempotent case).
    const preBubbleHash = proof ? proof.snapshotLastBubble() : '';

    const messageKeys = ['message', 'messenger', 'send message', 'nhan tin'];
    const sendKeys = ['send', 'press enter to send', 'gui'];
    let editors = Array.from(document.querySelectorAll('[contenteditable="true"], textarea')).filter(visible);
    if (!editors.length) {
      const messageButton = Array.from(document.querySelectorAll('div[role="button"], button, a[role="button"]')).filter(visible)
        .find(el => hasAny(labelOf(el), messageKeys));
      if (!messageButton || !clickLikeUser(messageButton)) return inboxResult(false, 'message_button_not_found', null, { content, preBubbleHash });
      await wait(1800);
      editors = Array.from(document.querySelectorAll('[contenteditable="true"], textarea')).filter(visible);
    }
    let editor = editors.find(el => hasAny(labelOf(el), messageKeys) || norm(el.getAttribute('role')) === 'textbox');
    if (!editor) editor = editors[editors.length - 1];
    if (!editor) return inboxResult(false, 'message_box_not_found', null, { content, preBubbleHash });
    if (!setEditableText(editor, content)) return inboxResult(false, 'inbox_text_insert_failed', null, { content, preBubbleHash });
    await wait(700);
    const scope = editor.closest('[role="dialog"], form, div[aria-label]') || document;
    const send = Array.from(scope.querySelectorAll('div[role="button"], button, [aria-label]')).filter(visible).find(el => {
      const label = labelOf(el);
      return hasAny(label, sendKeys) && el.getAttribute('aria-disabled') !== 'true' && !el.disabled;
    });
    if (!send || !clickLikeUser(send)) return inboxResult(false, 'inbox_submit_not_found', null, { content, preBubbleHash });
    // Longer settle for bubble + timestamp to render — FB animates the
    // bubble in, and "Just now" copy can lag the bubble itself.
    await wait(1500);
    return inboxResult(true, '', 'sent_inbox_button', { content, preBubbleHash });
  }

  function inboxResult(ok, errorCode, detail, ctx) {
    const proof = THGContentProof ? THGContentProof.buildInboxProof({
      ok, errorCode, content: ctx.content, preBubbleHash: ctx.preBubbleHash
    }) : null;
    const base = ok
      ? { ok: true, detail: detail || 'sent_inbox' }
      : { ok: false, error: errorCode || 'inbox_failed' };
    return proof ? { ...base, proof } : base;
  }

  async function executePost(content) {
    await dismissBlockingOverlays();
    const composerKeys = ["what's on your mind", 'write something', 'create a public post', 'ban dang nghi gi', 'viet gi do'];
    const postKeys = ['post', 'dang'];
    const composer = Array.from(document.querySelectorAll('div[role="button"], button, textarea, [contenteditable="true"], [aria-label]'))
      .filter(visible)
      .find(el => hasAny(labelOf(el), composerKeys));
    if (!composer || !clickLikeUser(composer)) return postResult(false, 'post_composer_not_found', null, { content });
    await wait(1500);
    const editors = Array.from(document.querySelectorAll('[contenteditable="true"], textarea')).filter(visible);
    let editor = editors.find(el => norm(el.getAttribute('role')) === 'textbox') || editors[editors.length - 1];
    if (!editor) return postResult(false, 'post_editor_not_found', null, { content });
    if (!setEditableText(editor, content)) return postResult(false, 'post_text_insert_failed', null, { content });
    await wait(900);
    const scope = editor.closest('[role="dialog"], form') || document;
    const postButton = Array.from(scope.querySelectorAll('div[role="button"], button, [aria-label]')).filter(visible).reverse().find(el => {
      const label = labelOf(el);
      return hasAny(label, postKeys) && !label.includes('comment') && !label.includes('cancel') &&
        el.getAttribute('aria-disabled') !== 'true' && !el.disabled;
    });
    if (!postButton || !clickLikeUser(postButton)) return postResult(false, 'post_submit_not_found', null, { content });
    // Generous settle — posting closes the composer dialog and re-renders
    // the feed; we need both to complete before walking the DOM for proof.
    await wait(2500);
    return postResult(true, '', 'sent_post_button', { content });
  }

  function postResult(ok, errorCode, detail, ctx) {
    const proof = THGContentProof ? THGContentProof.buildPostProof({
      ok, errorCode, content: ctx.content
    }) : null;
    const base = ok
      ? { ok: true, detail: detail || 'sent_post' }
      : { ok: false, error: errorCode || 'post_failed' };
    return proof ? { ...base, proof } : base;
  }

  async function executeOutbound(message) {
    const content = String(message?.content || '').trim();
    if (!content) return { ok: false, error: 'outbox_content_empty' };
    if (content.length > 3000) return { ok: false, error: 'outbox_content_too_long' };
    const type = String(message?.type || '').trim().toLowerCase();
    if (type === 'comment') return executeComment(content);
    if (type === 'inbox') return executeInbox(content);
    if (type === 'group_post' || type === 'profile_post') return executePost(content);
    return { ok: false, error: `unsupported_outbox_type:${type}` };
  }

  return { executeOutbound };
})();
globalThis.THGContentOutbound = THGContentOutbound;
