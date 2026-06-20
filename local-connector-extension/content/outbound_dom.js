// THGOutboundDom — generic browser DOM/editable/click/wait/overlay primitives shared by every
// outbound layer (comment/posting/inbox). Extracted verbatim from outbound.js (Workstream A · PR2,
// move-only). No identity/diagnostics/proof/executor logic. Chrome: globalThis.THGOutboundDom
// (manifest-loaded before outbound.js); Node: module.exports. Guards below (PR8C/PR8D/PR-DUP) — do
// NOT loosen; full forensic rationale is in git history.
var THGOutboundDom = globalThis.THGOutboundDom || (() => {
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
    return { bubbles: true, cancelable: true, composed: true, clientX: x, clientY: y, ...extra };
  }

  function dispatchMouseLike(el, type, x, y, extra = {}) {
    try {
      el.dispatchEvent(new MouseEvent(type, eventInit(x, y, extra)));
    } catch (_) { /* synthetic event dispatch is best-effort; ignore */ }
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
    } catch (_) { /* synthetic event dispatch is best-effort; ignore */ }
  }

  function enabledButton(el) {
    return el && el.getAttribute?.('aria-disabled') !== 'true' && !el.disabled;
  }

  // clickLikeUser fires pointer→mouse→click at the element centre, RE-VALIDATING at click
  // time and returning false (not true) on null/detached/invisible/disabled — so the submit
  // SM never treats a no-op click on a ghost node as success. Do NOT revert to `return true`.
  function clickLikeUser(el) {
    if (!el) return false;
    if (el.isConnected === false) return false;
    if (!visible(el) || !enabledButton(el)) return false;
    try { el.scrollIntoView({ block: 'center', inline: 'center' }); } catch (_) { /* scroll is best-effort */ }
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
      // Synthetic dispatch threw — native click fallback; success only if it doesn't throw.
      try { el.click(); return true; } catch (_) { return false; }
    }
  }

  // WHOLE-WORD match, never raw substring. PR8C: `includes('ok')` matched "faceb-OO-k" → the
  // FB logo got clicked → home pushState → every comment failed target_not_reached.
  function labelMatchesDismiss(label, keys) {
    return keys.some((key) => {
      const escaped = key.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
      return new RegExp('(^|\\W)' + escaped + '($|\\W)').test(label);
    });
  }

  // PR8D.1: true if el is inside a dialog/article with a post-permalink anchor — never dismiss a control of the target post (its Close/X → home).
  function isInsidePostContainer(el) {
    const container = el.closest && el.closest('[role="dialog"], [role="article"]');
    if (!container) return false;
    return !!container.querySelector(
      'a[href*="/posts/"], a[href*="/permalink/"], a[href*="story_fbid="], a[href*="/videos/"], a[href*="/reel/"], a[href*="/share/"]'
    );
  }

  // Click only SPECIFIC decline labels on button-shaped controls. PR8C/PR8D.1 dropped generic
  // 'ok'/'close' + bare [aria-label] (matched the FB logo / a post Close → home). FOUR guards,
  // all required: word-boundary label + nav-link exclusion + post-container exclusion + button shape.
  async function dismissBlockingOverlays() {
    const labels = ['not now', 'later', 'maybe later', 'remember password', 'de sau', 'luc khac', 'khong phai bay gio'];
    const candidates = Array.from(document.querySelectorAll('div[role="button"], button, a[role="button"], span[role="button"]')).filter(visible);
    for (const el of candidates) {
      const role = norm(el.getAttribute?.('role'));
      const isNavLink = role === 'link' || (el.tagName === 'A' && !!el.getAttribute?.('href') && role !== 'button');
      if (isNavLink) continue;
      if (isInsidePostContainer(el)) continue;
      const label = labelOf(el);
      if (!label) continue;
      if (labelMatchesDismiss(label, labels)) {
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
    try { editor.dispatchEvent(new Event('change', { bubbles: true })); } catch (_) { /* change event is best-effort; ignore */ }
  }

  // PR8D + PR-DUP: clear any FB-restored draft (bounded 6× loop) BEFORE insertText, else it APPENDS → dup comment.
  function setEditableText(editor, text) {
    try { editor.focus({ preventScroll: true }); } catch (_) { try { editor.focus(); } catch (_) { /* focus is best-effort; ignore */ } }
    if (editor.isContentEditable) {
      for (let i = 0; i < 6; i += 1) {
        if (norm(textOfEditable(editor)).length === 0) break;
        selectEditableContents(editor);
        try { document.execCommand('delete', false, null); } catch (_) { /* draft clear is best-effort; ignore */ }
      }
      selectEditableContents(editor);
      document.execCommand('insertText', false, text);
    } else if ('value' in editor) {
      setInputValue(editor, '');
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

  return {
    wait, norm, hasAny, visible, labelOf, eventInit, dispatchMouseLike, dispatchPointerLike,
    enabledButton, clickLikeUser, labelMatchesDismiss, isInsidePostContainer, dismissBlockingOverlays,
    textOfEditable, setInputValue, selectEditableContents, emitEditableInput, setEditableText, waitFor,
  };
})();
globalThis.THGOutboundDom = THGOutboundDom;
if (typeof module !== 'undefined' && module.exports) module.exports = THGOutboundDom;
