var THGContentCommands = globalThis.THGContentCommands || (() => {
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

  function pointFromPayload(payload) {
    const imageWidth = Number(payload.image_width || payload.imageWidth || window.innerWidth || 1);
    const imageHeight = Number(payload.image_height || payload.imageHeight || window.innerHeight || 1);
    const x = Math.max(0, Math.min(window.innerWidth - 1, Number(payload.x || 0) * (window.innerWidth / Math.max(1, imageWidth))));
    const y = Math.max(0, Math.min(window.innerHeight - 1, Number(payload.y || 0) * (window.innerHeight / Math.max(1, imageHeight))));
    return { x, y };
  }

  function clickAt(payload) {
    const { x, y } = pointFromPayload(payload);
    const el = document.elementFromPoint(x, y) || document.body;
    const buttonName = String(payload.button || 'left').toLowerCase();
    const button = buttonName === 'right' ? 2 : buttonName === 'middle' ? 1 : 0;
    el.dispatchEvent(new PointerEvent('pointerdown', eventInit(x, y, { button, pointerId: 1, pointerType: 'mouse', isPrimary: true })));
    el.dispatchEvent(new MouseEvent('mousedown', eventInit(x, y, { button })));
    el.dispatchEvent(new PointerEvent('pointerup', eventInit(x, y, { button, pointerId: 1, pointerType: 'mouse', isPrimary: true })));
    el.dispatchEvent(new MouseEvent('mouseup', eventInit(x, y, { button })));
    el.dispatchEvent(new MouseEvent('click', eventInit(x, y, { button })));
    if (typeof el.focus === 'function') el.focus();
    return { ok: true, detail: `${el.tagName || 'NODE'}:${el.getAttribute?.('name') || el.getAttribute?.('aria-label') || ''}` };
  }

  function insertText(payload) {
    const text = String(payload.text || '').slice(0, 2000);
    const el = document.activeElement;
    if (!text || !el) return { ok: true, detail: 'empty_text' };
    if (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) {
      const start = el.selectionStart ?? el.value.length;
      const end = el.selectionEnd ?? el.value.length;
      el.value = `${el.value.slice(0, start)}${text}${el.value.slice(end)}`;
      const next = start + text.length;
      el.setSelectionRange(next, next);
      el.dispatchEvent(new InputEvent('input', { bubbles: true, data: text, inputType: 'insertText' }));
      el.dispatchEvent(new Event('change', { bubbles: true }));
      return { ok: true, detail: 'text_inserted' };
    }
    if (el.isContentEditable) {
      document.execCommand('insertText', false, text);
      el.dispatchEvent(new InputEvent('input', { bubbles: true, data: text, inputType: 'insertText' }));
      return { ok: true, detail: 'contenteditable_text_inserted' };
    }
    return { ok: false, error: 'No editable field is focused' };
  }

  function pressKey(payload) {
    const el = document.activeElement || document.body;
    const init = {
      key: String(payload.key || ''),
      code: String(payload.code || ''),
      ctrlKey: Boolean(payload.ctrl_key || payload.ctrlKey),
      altKey: Boolean(payload.alt_key || payload.altKey),
      shiftKey: Boolean(payload.shift_key || payload.shiftKey),
      metaKey: Boolean(payload.meta_key || payload.metaKey),
      bubbles: true,
      cancelable: true
    };
    el.dispatchEvent(new KeyboardEvent('keydown', init));
    el.dispatchEvent(new KeyboardEvent('keyup', init));
    return { ok: true, detail: init.key || 'key' };
  }

  function scrollByPayload(payload) {
    const dx = Number(payload.delta_x || payload.deltaX || 0);
    const dy = Number(payload.delta_y || payload.deltaY || 0);
    window.scrollBy({ left: dx, top: dy, behavior: 'auto' });
    return { ok: true, detail: 'scrolled' };
  }

  function executeBasicCommand(command) {
    const payload = typeof command.payload_json === 'string' ? JSON.parse(command.payload_json || '{}') : (command.payload_json || {});
    switch (String(command.type || '').toLowerCase()) {
      case 'click':
        return clickAt(payload);
      case 'key':
        return pressKey(payload);
      case 'text':
        return insertText(payload);
      case 'scroll':
        return scrollByPayload(payload);
      default:
        return null;
    }
  }

  return { executeBasicCommand };
})();
globalThis.THGContentCommands = THGContentCommands;
