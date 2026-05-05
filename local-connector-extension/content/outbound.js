var THGContentOutbound = globalThis.THGContentOutbound || (() => {
  const wait = (ms) => new Promise(resolve => setTimeout(resolve, ms));
  const norm = (value) => String(value || '').trim().toLowerCase();
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

  function clickLikeUser(el) {
    if (!el) return false;
    try { el.scrollIntoView({ block: 'center', inline: 'center' }); } catch (_) {}
    try { el.click(); return true; } catch (_) { return false; }
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

  function setEditableText(editor, text) {
    try { editor.focus({ preventScroll: true }); } catch (_) { try { editor.focus(); } catch (_) {} }
    if (editor.isContentEditable) {
      document.execCommand('insertText', false, text);
    } else if ('value' in editor) {
      const proto = editor instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
      const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
      if (setter) setter.call(editor, text);
      else editor.value = text;
    } else {
      return false;
    }
    try {
      editor.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText', data: text }));
    } catch (_) {
      editor.dispatchEvent(new Event('input', { bubbles: true }));
    }
    return true;
  }

  async function executeComment(content) {
    await dismissBlockingOverlays();
    const commentKeys = ['comment', 'write a comment', 'binh luan', 'viet binh luan'];
    const submitKeys = ['comment', 'post', 'send', 'binh luan', 'dang', 'gui'];
    const buttons = Array.from(document.querySelectorAll('div[role="button"], button, a[role="button"], span[role="button"]')).filter(visible);
    const commentButton = buttons.find(el => {
      const label = labelOf(el);
      return hasAny(label, commentKeys) && !label.includes('share') && !label.includes('like');
    });
    if (commentButton) {
      clickLikeUser(commentButton);
      await wait(900);
    }
    const editors = Array.from(document.querySelectorAll('[contenteditable="true"], textarea, input[type="text"]')).filter(visible);
    let editor = editors.find(el => hasAny(labelOf(el), commentKeys));
    if (!editor) editor = editors.find(el => norm(el.getAttribute('role')) === 'textbox');
    if (!editor) editor = editors[0];
    if (!editor) return { ok: false, error: 'comment_box_not_found' };
    if (!setEditableText(editor, content)) return { ok: false, error: 'comment_text_insert_failed' };
    await wait(700);
    const scope = editor.closest('[role="dialog"], form, [role="article"]') || document;
    const submit = Array.from(scope.querySelectorAll('div[role="button"], button, [aria-label]')).filter(visible).find(el => {
      const label = labelOf(el);
      return hasAny(label, submitKeys) && !label.includes('share') && !label.includes('like') && !label.includes('cancel') &&
        el.getAttribute('aria-disabled') !== 'true' && !el.disabled;
    });
    if (!submit || !clickLikeUser(submit)) return { ok: false, error: 'comment_submit_not_found' };
    await wait(1000);
    return { ok: true, detail: 'sent_comment_button' };
  }

  async function executeInbox(content) {
    await dismissBlockingOverlays();
    const messageKeys = ['message', 'messenger', 'send message', 'nhan tin'];
    const sendKeys = ['send', 'press enter to send', 'gui'];
    let editors = Array.from(document.querySelectorAll('[contenteditable="true"], textarea')).filter(visible);
    if (!editors.length) {
      const messageButton = Array.from(document.querySelectorAll('div[role="button"], button, a[role="button"]')).filter(visible)
        .find(el => hasAny(labelOf(el), messageKeys));
      if (!messageButton || !clickLikeUser(messageButton)) return { ok: false, error: 'message_button_not_found' };
      await wait(1800);
      editors = Array.from(document.querySelectorAll('[contenteditable="true"], textarea')).filter(visible);
    }
    let editor = editors.find(el => hasAny(labelOf(el), messageKeys) || norm(el.getAttribute('role')) === 'textbox');
    if (!editor) editor = editors[editors.length - 1];
    if (!editor) return { ok: false, error: 'message_box_not_found' };
    if (!setEditableText(editor, content)) return { ok: false, error: 'inbox_text_insert_failed' };
    await wait(700);
    const scope = editor.closest('[role="dialog"], form, div[aria-label]') || document;
    const send = Array.from(scope.querySelectorAll('div[role="button"], button, [aria-label]')).filter(visible).find(el => {
      const label = labelOf(el);
      return hasAny(label, sendKeys) && el.getAttribute('aria-disabled') !== 'true' && !el.disabled;
    });
    if (!send || !clickLikeUser(send)) return { ok: false, error: 'inbox_submit_not_found' };
    await wait(1000);
    return { ok: true, detail: 'sent_inbox_button' };
  }

  async function executePost(content) {
    await dismissBlockingOverlays();
    const composerKeys = ["what's on your mind", 'write something', 'create a public post', 'ban dang nghi gi', 'viet gi do'];
    const postKeys = ['post', 'dang'];
    const composer = Array.from(document.querySelectorAll('div[role="button"], button, textarea, [contenteditable="true"], [aria-label]'))
      .filter(visible)
      .find(el => hasAny(labelOf(el), composerKeys));
    if (!composer || !clickLikeUser(composer)) return { ok: false, error: 'post_composer_not_found' };
    await wait(1500);
    const editors = Array.from(document.querySelectorAll('[contenteditable="true"], textarea')).filter(visible);
    let editor = editors.find(el => norm(el.getAttribute('role')) === 'textbox') || editors[editors.length - 1];
    if (!editor) return { ok: false, error: 'post_editor_not_found' };
    if (!setEditableText(editor, content)) return { ok: false, error: 'post_text_insert_failed' };
    await wait(900);
    const scope = editor.closest('[role="dialog"], form') || document;
    const postButton = Array.from(scope.querySelectorAll('div[role="button"], button, [aria-label]')).filter(visible).reverse().find(el => {
      const label = labelOf(el);
      return hasAny(label, postKeys) && !label.includes('comment') && !label.includes('cancel') &&
        el.getAttribute('aria-disabled') !== 'true' && !el.disabled;
    });
    if (!postButton || !clickLikeUser(postButton)) return { ok: false, error: 'post_submit_not_found' };
    await wait(1500);
    return { ok: true, detail: 'sent_post_button' };
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
