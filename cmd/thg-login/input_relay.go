package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	cdpinput "github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

func executePendingCommands(serverURL, token string, bridges map[int64]*chromeBridge) bool {
	commands, err := fetchConnectorCommands(serverURL, token)
	if err != nil {
		if isDeviceTokenRejected(err) {
			printDeviceTokenRejected(err)
			return false
		}
		fmt.Println("[warn] input command sync failed:", err)
		return true
	}
	if len(commands) > 0 {
		fmt.Printf("[Input] received %d dashboard command(s)\n", len(commands))
	}
	for _, cmd := range commands {
		errText := ""
		result, err := executeConnectorCommand(serverURL, token, cmd, bridges)
		if err != nil {
			errText = err.Error()
			fmt.Printf("[warn] input command %d failed: %s\n", cmd.ID, errText)
		} else {
			fmt.Printf("[Input] command %d (%s) sent to account %d -> %s\n", cmd.ID, cmd.Type, cmd.AccountID, result)
		}
		if err := completeConnectorCommand(serverURL, token, cmd.ID, errText); err != nil {
			if isDeviceTokenRejected(err) {
				printDeviceTokenRejected(err)
				return false
			}
			fmt.Printf("[warn] input command %d completion failed: %v\n", cmd.ID, err)
		}
	}
	return true
}

func executeConnectorCommand(serverURL, token string, cmd connectorCommand, bridges map[int64]*chromeBridge) (string, error) {
	bridge := bridges[cmd.AccountID]
	if bridge == nil || bridge.ctx == nil || bridge.err != nil {
		return "", fmt.Errorf("Chrome profile for account %d is not ready", cmd.AccountID)
	}
	switch strings.ToLower(strings.TrimSpace(cmd.Type)) {
	case "crawl":
		return executeLocalCrawlCommand(serverURL, token, cmd, bridge)
	case "click":
		cmdCtx, cancel := context.WithTimeout(bridge.ctx, 5*time.Second)
		defer cancel()
		var payload struct {
			X           float64 `json:"x"`
			Y           float64 `json:"y"`
			ImageWidth  float64 `json:"image_width"`
			ImageHeight float64 `json:"image_height"`
			Button      string  `json:"button"`
			Clicks      int64   `json:"clicks"`
		}
		if err := json.Unmarshal([]byte(defaultString(cmd.PayloadJSON, "{}")), &payload); err != nil {
			return "", err
		}
		x, y := scaleInputPoint(cmdCtx, payload.X, payload.Y, payload.ImageWidth, payload.ImageHeight)
		var result string
		err := chromedp.Run(cmdCtx, chromedp.Evaluate(clickElementAtPointJS(x, y, mouseButtonNumber(payload.Button)), &result))
		return result, err
	case "scroll":
		cmdCtx, cancel := context.WithTimeout(bridge.ctx, 5*time.Second)
		defer cancel()
		var payload struct {
			X           float64 `json:"x"`
			Y           float64 `json:"y"`
			ImageWidth  float64 `json:"image_width"`
			ImageHeight float64 `json:"image_height"`
			DeltaX      float64 `json:"delta_x"`
			DeltaY      float64 `json:"delta_y"`
		}
		if err := json.Unmarshal([]byte(defaultString(cmd.PayloadJSON, "{}")), &payload); err != nil {
			return "", err
		}
		if payload.DeltaY == 0 {
			payload.DeltaY = 400
		}
		x, y := scaleInputPoint(cmdCtx, payload.X, payload.Y, payload.ImageWidth, payload.ImageHeight)
		var result string
		err := chromedp.Run(cmdCtx, chromedp.Evaluate(scrollAtPointJS(x, y, payload.DeltaX, payload.DeltaY), &result))
		return result, err
	case "text":
		cmdCtx, cancel := context.WithTimeout(bridge.ctx, 5*time.Second)
		defer cancel()
		var payload struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(defaultString(cmd.PayloadJSON, "{}")), &payload); err != nil {
			return "", err
		}
		if payload.Text == "" {
			return "empty_text", nil
		}
		if len([]rune(payload.Text)) > 256 {
			return "", fmt.Errorf("text command is too long")
		}
		var result string
		err := chromedp.Run(cmdCtx, chromedp.Evaluate(insertTextIntoActiveElementJS(payload.Text), &result))
		return result, err
	case "key":
		cmdCtx, cancel := context.WithTimeout(bridge.ctx, 5*time.Second)
		defer cancel()
		var payload struct {
			Key     string `json:"key"`
			Code    string `json:"code"`
			CtrlKey bool   `json:"ctrl_key"`
			AltKey  bool   `json:"alt_key"`
			Shift   bool   `json:"shift_key"`
			MetaKey bool   `json:"meta_key"`
		}
		if err := json.Unmarshal([]byte(defaultString(cmd.PayloadJSON, "{}")), &payload); err != nil {
			return "", err
		}
		if len([]rune(payload.Key)) == 1 && !payload.CtrlKey && !payload.AltKey && !payload.MetaKey {
			var result string
			err := chromedp.Run(cmdCtx, chromedp.Evaluate(insertTextIntoActiveElementJS(payload.Key), &result))
			return result, err
		}
		if payload.Key == "Backspace" || payload.Key == "Tab" || payload.Key == "Enter" {
			var result string
			err := chromedp.Run(cmdCtx, chromedp.Evaluate(specialKeyJS(payload.Key, payload.Shift), &result))
			return result, err
		}
		key, code, vk := normalizeKey(payload.Key, payload.Code)
		if key == "" {
			return "empty_key", nil
		}
		modifiers := keyModifiers(payload.CtrlKey, payload.AltKey, payload.Shift, payload.MetaKey)
		err := chromedp.Run(cmdCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				return cdpinput.DispatchKeyEvent(cdpinput.KeyRawDown).
					WithKey(key).
					WithCode(code).
					WithWindowsVirtualKeyCode(vk).
					WithNativeVirtualKeyCode(vk).
					WithModifiers(modifiers).
					Do(ctx)
			}),
			chromedp.ActionFunc(func(ctx context.Context) error {
				return cdpinput.DispatchKeyEvent(cdpinput.KeyUp).
					WithKey(key).
					WithCode(code).
					WithWindowsVirtualKeyCode(vk).
					WithNativeVirtualKeyCode(vk).
					WithModifiers(modifiers).
					Do(ctx)
			}),
		)
		return "cdp_key", err
	default:
		return "", fmt.Errorf("unsupported command type %q", cmd.Type)
	}
}

func mouseButton(value string) cdpinput.MouseButton {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "right":
		return cdpinput.Right
	case "middle":
		return cdpinput.Middle
	default:
		return cdpinput.Left
	}
}

func mouseButtonNumber(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "right":
		return 2
	case "middle":
		return 1
	default:
		return 0
	}
}

func scaleInputPoint(ctx context.Context, x, y, imageWidth, imageHeight float64) (float64, float64) {
	if imageWidth <= 0 || imageHeight <= 0 {
		return x, y
	}
	var viewportWidth, viewportHeight float64
	scaleCtx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
	defer cancel()
	var dims []float64
	err := chromedp.Run(scaleCtx, chromedp.Evaluate(`(() => [
		window.innerWidth || document.documentElement.clientWidth || 0,
		window.innerHeight || document.documentElement.clientHeight || 0
	])()`, &dims))
	if err != nil || viewportWidth <= 0 || viewportHeight <= 0 {
		if len(dims) >= 2 {
			viewportWidth = dims[0]
			viewportHeight = dims[1]
		}
	}
	if viewportWidth <= 0 || viewportHeight <= 0 {
		return x, y
	}
	return x * viewportWidth / imageWidth, y * viewportHeight / imageHeight
}

func clickElementAtPointJS(x, y float64, button int) string {
	return fmt.Sprintf(`(() => {
  const x = %s, y = %s, button = %d;
  const raw = document.elementFromPoint(x, y);
  if (!raw) return 'no_element';
  let target = (raw.closest && raw.closest('input,textarea,button,a,[role="button"],[contenteditable="true"],label,select')) || raw;
  const isTypingTarget = (node) => node && (node.matches && node.matches('input,textarea,[contenteditable="true"]'));
  if (!isTypingTarget(target)) {
    const inputs = Array.from(document.querySelectorAll('input,textarea,[contenteditable="true"]'))
      .filter(node => {
        const r = node.getBoundingClientRect();
        return r.width > 20 && r.height > 10 && r.bottom >= 0 && r.right >= 0 && r.top <= innerHeight && r.left <= innerWidth;
      })
      .map(node => {
        const r = node.getBoundingClientRect();
        const cx = Math.max(r.left, Math.min(x, r.right));
        const cy = Math.max(r.top, Math.min(y, r.bottom));
        return {node, d: Math.hypot(x - cx, y - cy)};
      })
      .sort((a, b) => a.d - b.d);
    if (inputs[0] && inputs[0].d <= 180) target = inputs[0].node;
  }
  const opts = {bubbles:true,cancelable:true,view:window,clientX:x,clientY:y,button,buttons:button === 2 ? 2 : button === 1 ? 4 : 1};
  try { target.dispatchEvent(new PointerEvent('pointerdown', opts)); } catch (_) {}
  try { target.dispatchEvent(new MouseEvent('mousedown', opts)); } catch (_) {}
  if (typeof target.focus === 'function') {
    try { target.focus({preventScroll:true}); } catch (_) { try { target.focus(); } catch (_) {} }
  }
  if (isTypingTarget(target)) window.__thgLastInput = target;
  try { target.dispatchEvent(new PointerEvent('pointerup', opts)); } catch (_) {}
  try { target.dispatchEvent(new MouseEvent('mouseup', opts)); } catch (_) {}
  try { target.dispatchEvent(new MouseEvent('click', opts)); } catch (_) {}
  try { if (typeof target.click === 'function') target.click(); } catch (_) {}
  if (target.tagName === 'LABEL') {
    const input = target.control || (target.getAttribute('for') ? document.getElementById(target.getAttribute('for')) : null);
    if (input && typeof input.focus === 'function') { input.focus(); window.__thgLastInput = input; }
  }
  return (target.tagName || 'element') + ':' + ((target.getAttribute && (target.getAttribute('name') || target.getAttribute('type') || target.id)) || '');
})()`, jsFloat(x), jsFloat(y), button)
}

func scrollAtPointJS(x, y, deltaX, deltaY float64) string {
	return fmt.Sprintf(`(() => {
  const x = %s, y = %s;
  const target = document.elementFromPoint(x, y) || document.scrollingElement || document.documentElement;
  const scroller = target.closest && target.closest('[style*="overflow"], [data-pagelet], div') || document.scrollingElement || document.documentElement;
  try { scroller.scrollBy(%s, %s); } catch (_) { window.scrollBy(%s, %s); }
  return 'scrolled';
})()`, jsFloat(x), jsFloat(y), jsFloat(deltaX), jsFloat(deltaY), jsFloat(deltaX), jsFloat(deltaY))
}

func insertTextIntoActiveElementJS(text string) string {
	return fmt.Sprintf(`(() => {
  const text = %s;
  let el = document.activeElement;
  const usable = (node) => node && node.isConnected && (node.isContentEditable || ('value' in node));
  if (!usable(el) || el === document.body || el === document.documentElement) {
    if (usable(window.__thgLastInput)) el = window.__thgLastInput;
  }
  if (!usable(el) || el === document.body || el === document.documentElement) {
    el = Array.from(document.querySelectorAll('input,textarea,[contenteditable="true"]')).find(node => {
      const r = node.getBoundingClientRect();
      return r.width > 20 && r.height > 10 && r.bottom >= 0 && r.right >= 0 && r.top <= innerHeight && r.left <= innerWidth;
    });
  }
  if (!usable(el) || el === document.body || el === document.documentElement) return 'no_active_element';
  try { if (typeof el.focus === 'function') el.focus({preventScroll:true}); } catch (_) { try { el.focus(); } catch (_) {} }
  window.__thgLastInput = el;
  if (el.isContentEditable) {
    document.execCommand('insertText', false, text);
    return 'contenteditable';
  }
  if (!('value' in el)) return 'active_not_text';
  const value = String(el.value || '');
  const start = typeof el.selectionStart === 'number' ? el.selectionStart : value.length;
  const end = typeof el.selectionEnd === 'number' ? el.selectionEnd : start;
  const next = value.slice(0, start) + text + value.slice(end);
  const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, 'value') && Object.getOwnPropertyDescriptor(proto, 'value').set;
  if (setter) setter.call(el, next); else el.value = next;
  const pos = start + text.length;
  try { el.setSelectionRange(pos, pos); } catch (_) {}
  try { el.dispatchEvent(new InputEvent('input', {bubbles:true, inputType:'insertText', data:text})); } catch (_) { el.dispatchEvent(new Event('input', {bubbles:true})); }
  return 'text_inserted';
})()`, jsString(text))
}

func specialKeyJS(key string, shift bool) string {
	return fmt.Sprintf(`(() => {
  const key = %s, shift = %t;
  const el = document.activeElement;
  const fire = (type) => { try { el && el.dispatchEvent(new KeyboardEvent(type, {key, bubbles:true, cancelable:true, shiftKey:shift})); } catch (_) {} };
  if (key === 'Tab') {
    const nodes = Array.from(document.querySelectorAll('input,textarea,button,a[href],select,[tabindex]:not([tabindex="-1"]),[contenteditable="true"]'))
      .filter(n => !n.disabled && n.offsetParent !== null);
    if (!nodes.length) return 'tab_no_targets';
    const index = Math.max(0, nodes.indexOf(el));
    const next = nodes[(index + (shift ? -1 : 1) + nodes.length) %% nodes.length];
    next.focus();
    return 'tab_focus';
  }
  if (key === 'Backspace' && el && 'value' in el) {
    const value = String(el.value || '');
    const start = typeof el.selectionStart === 'number' ? el.selectionStart : value.length;
    const end = typeof el.selectionEnd === 'number' ? el.selectionEnd : start;
    const from = start === end ? Math.max(0, start - 1) : start;
    const nextValue = value.slice(0, from) + value.slice(end);
    const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    const setter = Object.getOwnPropertyDescriptor(proto, 'value') && Object.getOwnPropertyDescriptor(proto, 'value').set;
    if (setter) setter.call(el, nextValue); else el.value = nextValue;
    try { el.setSelectionRange(from, from); } catch (_) {}
    try { el.dispatchEvent(new InputEvent('input', {bubbles:true, inputType:'deleteContentBackward'})); } catch (_) { el.dispatchEvent(new Event('input', {bubbles:true})); }
    return 'backspace';
  }
  fire('keydown');
  if (key === 'Enter') {
    const active = document.activeElement;
    const clickable = active && active.closest && active.closest('button,a,[role="button"]');
    if (clickable && typeof clickable.click === 'function') clickable.click();
    else {
      const form = active && active.closest && active.closest('form');
      if (form) {
        if (typeof form.requestSubmit === 'function') form.requestSubmit();
        else form.submit();
      }
    }
  }
  fire('keyup');
  return 'key_' + key;
})()`, jsString(key), shift)
}

func jsString(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(data)
}

func jsFloat(value float64) string {
	if value != value || value > 1e9 || value < -1e9 {
		return "0"
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func mouseButtonsMask(button cdpinput.MouseButton) int64 {
	switch button {
	case cdpinput.Right:
		return 2
	case cdpinput.Middle:
		return 4
	case cdpinput.Left:
		return 1
	default:
		return 0
	}
}

func keyModifiers(ctrl, alt, shift, meta bool) cdpinput.Modifier {
	var out cdpinput.Modifier
	if alt {
		out |= 1
	}
	if ctrl {
		out |= 2
	}
	if meta {
		out |= 4
	}
	if shift {
		out |= 8
	}
	return out
}

func normalizeKey(key, code string) (string, string, int64) {
	key = strings.TrimSpace(key)
	code = strings.TrimSpace(code)
	if key == "" {
		key = code
	}
	switch key {
	case "Enter":
		return "Enter", defaultString(code, "Enter"), 13
	case "Backspace":
		return "Backspace", defaultString(code, "Backspace"), 8
	case "Tab":
		return "Tab", defaultString(code, "Tab"), 9
	case "Escape", "Esc":
		return "Escape", defaultString(code, "Escape"), 27
	case "Delete":
		return "Delete", defaultString(code, "Delete"), 46
	case "ArrowLeft":
		return "ArrowLeft", defaultString(code, "ArrowLeft"), 37
	case "ArrowUp":
		return "ArrowUp", defaultString(code, "ArrowUp"), 38
	case "ArrowRight":
		return "ArrowRight", defaultString(code, "ArrowRight"), 39
	case "ArrowDown":
		return "ArrowDown", defaultString(code, "ArrowDown"), 40
	case "Home":
		return "Home", defaultString(code, "Home"), 36
	case "End":
		return "End", defaultString(code, "End"), 35
	case "PageUp":
		return "PageUp", defaultString(code, "PageUp"), 33
	case "PageDown":
		return "PageDown", defaultString(code, "PageDown"), 34
	case " ":
		return " ", defaultString(code, "Space"), 32
	default:
		if len([]rune(key)) == 1 {
			upper := strings.ToUpper(key)
			if upper[0] >= 'A' && upper[0] <= 'Z' {
				if code == "" {
					code = "Key" + upper
				}
				return key, code, int64(upper[0])
			}
			if upper[0] >= '0' && upper[0] <= '9' {
				if code == "" {
					code = "Digit" + upper
				}
				return key, code, int64(upper[0])
			}
		}
		return key, code, 0
	}
}
