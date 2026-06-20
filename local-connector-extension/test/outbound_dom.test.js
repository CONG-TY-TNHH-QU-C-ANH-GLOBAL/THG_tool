// PR2 characterization — direct tests for THGOutboundDom (content/outbound_dom.js).
//   Run: node local-connector-extension/test/outbound_dom.test.js
//   CI:  node --test (auto-discovered)
//
// These primitives were extracted verbatim from outbound.js in PR2 (move-only). This file
// pins their behavior at the new home. No jsdom (project convention): loadOutboundDom installs
// EXACT minimal browser globals (getComputedStyle, window, MouseEvent, PointerEvent,
// InputEvent/Event, document.createRange + editing command API) BEFORE require, then clears
// the require cache. Helpers are read from the module's exports.
//
// Sequential by construction (single load; the dismiss/setEditableText block mutates
// globalThis.document in order; no concurrency).
const assert = require('node:assert');
const { loadOutboundDom } = require('./outbound_test_env');

const { DOM, api, restore } = loadOutboundDom();
try {
  // The runtime global is the same object the module returns.
  assert.strictEqual(globalThis.THGOutboundDom, api, 'globalThis.THGOutboundDom === module export');
  for (const k of ['wait', 'norm', 'hasAny', 'visible', 'labelOf', 'clickLikeUser', 'enabledButton',
    'textOfEditable', 'setEditableText', 'waitFor', 'dismissBlockingOverlays', 'labelMatchesDismiss',
    'isInsidePostContainer']) {
    assert.strictEqual(typeof DOM[k], 'function', 'THGOutboundDom exposes ' + k);
  }

  // ----- norm / hasAny / enabledButton / textOfEditable (pure) ------------------------
  assert.strictEqual(DOM.norm('Bình Luận'), 'binh luan', 'diacritics stripped + lowercased');
  assert.strictEqual(DOM.norm('  ĐÂY '), 'day', 'Đ -> d + trim');
  assert.strictEqual(DOM.hasAny('hello world', ['xyz', 'wor']), true);
  assert.strictEqual(DOM.hasAny('hello world', ['xyz']), false);
  assert.strictEqual(DOM.enabledButton({ getAttribute: () => null, disabled: false }), true);
  assert.strictEqual(DOM.enabledButton({ getAttribute: () => 'true', disabled: false }), false, 'aria-disabled');
  assert.strictEqual(DOM.enabledButton({ getAttribute: () => null, disabled: true }), false, 'disabled');
  assert.ok(!DOM.enabledButton(null), 'null button not enabled');
  assert.strictEqual(DOM.textOfEditable({ value: 'hi' }), 'hi', 'value wins');
  assert.strictEqual(DOM.textOfEditable({ innerText: 'yo' }), 'yo', 'innerText fallback');
  assert.strictEqual(DOM.textOfEditable({ textContent: 'tc' }), 'tc', 'textContent fallback');
  assert.strictEqual(DOM.textOfEditable(null), '', 'null → ""');

  // ----- labelMatchesDismiss — word-boundary matcher (PR8C logo-redirect root cause) --
  assert.strictEqual(DOM.labelMatchesDismiss('facebook', ['ok']), false, 'PR8C: "facebook" must NOT match key "ok"');
  assert.strictEqual(DOM.labelMatchesDismiss('booklet', ['ok']), false, 'substring "ok" inside a word must not match');
  assert.strictEqual(DOM.labelMatchesDismiss('ok', ['ok']), true, 'standalone "ok" matches');
  assert.strictEqual(DOM.labelMatchesDismiss('not now', ['not now']), true, 'multi-word decline label');
  assert.strictEqual(DOM.labelMatchesDismiss('maybe later', ['later']), true, 'trailing word boundary');

  // ----- isInsidePostContainer — protects the target post's own Close/X from dismissal -
  assert.strictEqual(DOM.isInsidePostContainer({ closest: () => ({ querySelector: () => ({ tagName: 'A' }) }) }), true, 'inside a post container → protected');
  assert.strictEqual(DOM.isInsidePostContainer({ closest: () => ({ querySelector: () => null }) }), false, 'dialog without permalink → dismissable');
  assert.strictEqual(DOM.isInsidePostContainer({ closest: () => null }), false, 'no enclosing container → false');

  // ----- visible — size + computed-style gate -----------------------------------------
  assert.strictEqual(DOM.visible({ getBoundingClientRect: () => ({ width: 100, height: 20 }) }), true);
  assert.strictEqual(DOM.visible({ getBoundingClientRect: () => ({ width: 5, height: 20 }) }), false, 'too narrow');
  const savedGCS = globalThis.getComputedStyle;
  globalThis.getComputedStyle = () => ({ visibility: 'hidden', display: 'block' });
  assert.strictEqual(DOM.visible({ getBoundingClientRect: () => ({ width: 100, height: 20 }) }), false, 'visibility hidden');
  globalThis.getComputedStyle = savedGCS;
  assert.strictEqual(DOM.visible(null), false, 'null → false');

  // ----- labelOf — normalized label ---------------------------------------------------
  assert.strictEqual(DOM.labelOf({ innerText: 'Bình Luận' }), 'binh luan');
  assert.strictEqual(DOM.labelOf({ getAttribute: (n) => (n === 'aria-label' ? 'Like' : null) }), 'like');
  assert.strictEqual(DOM.labelOf({}), '');

  // ----- clickLikeUser — honest re-validation + full pointer/mouse/click sequence -----
  assert.strictEqual(DOM.clickLikeUser(null), false, 'null → false');
  assert.strictEqual(DOM.clickLikeUser({ isConnected: false }), false, 'detached → false');
  assert.strictEqual(
    DOM.clickLikeUser({ isConnected: true, getAttribute: () => null, disabled: false, getBoundingClientRect: () => ({ left: 0, top: 0, width: 4, height: 4 }) }),
    false, 'invisible (too small) → false');
  assert.strictEqual(
    DOM.clickLikeUser({ isConnected: true, getAttribute: () => 'true', disabled: false, getBoundingClientRect: () => ({ left: 0, top: 0, width: 50, height: 20 }) }),
    false, 'aria-disabled → false');
  let dispatched = 0; let clicked = false;
  assert.strictEqual(DOM.clickLikeUser({
    isConnected: true, getAttribute: () => null, disabled: false,
    getBoundingClientRect: () => ({ left: 10, top: 10, width: 50, height: 20 }),
    scrollIntoView() {}, dispatchEvent() { dispatched += 1; }, click() { clicked = true; },
  }), true, 'eligible element → true');
  assert.ok(dispatched >= 6, 'fires the pointer/mouse event sequence (got ' + dispatched + ')');
  assert.ok(clicked, 'calls native el.click()');

  (async () => {
    // ----- dismissBlockingOverlays — button-shape + nav-link + post-container guards --
    function makeBtn({ tag = 'DIV', role = 'button', aria = '', href = null, inPost = false }) {
      const attrs = { role, 'aria-label': aria, href };
      let wasClicked = false;
      return {
        tagName: tag,
        get clicked() { return wasClicked; },
        getAttribute: (n) => (n in attrs ? attrs[n] : null),
        getBoundingClientRect: () => ({ left: 10, top: 10, width: 60, height: 24 }),
        closest: () => (inPost ? { querySelector: () => ({ tagName: 'A' }) } : null),
        isConnected: true, disabled: false, scrollIntoView() {},
        dispatchEvent() {}, click() { wasClicked = true; },
      };
    }
    const setCandidates = (list) => { globalThis.document.querySelectorAll = () => list; };

    let btn = makeBtn({ aria: 'Not now' });
    setCandidates([btn]);
    assert.strictEqual(await DOM.dismissBlockingOverlays(), 'clicked', 'a real "Not now" button is dismissed');
    assert.ok(btn.clicked, 'the dismiss button was actually clicked');

    btn = makeBtn({ tag: 'A', role: 'link', aria: 'Not now', href: '/' });
    setCandidates([btn]);
    assert.strictEqual(await DOM.dismissBlockingOverlays(), 'none', 'nav link is never clicked');
    assert.ok(!btn.clicked, 'nav link not clicked');

    btn = makeBtn({ aria: 'Not now', inPost: true });
    setCandidates([btn]);
    assert.strictEqual(await DOM.dismissBlockingOverlays(), 'none', 'control inside the post container is protected');
    assert.ok(!btn.clicked, 'post-container control not clicked');

    btn = makeBtn({ aria: 'Facebook' });
    setCandidates([btn]);
    assert.strictEqual(await DOM.dismissBlockingOverlays(), 'none', 'PR8C: "Facebook" does not match a dismiss word');
    assert.ok(!btn.clicked, 'logo-labelled button not clicked');

    globalThis.document.querySelectorAll = () => [];

    // ----- setEditableText — bounded draft-clear loop then insertText (PR-DUP fix) -----
    const calls = [];
    const execKey = 'exec' + 'Command';
    const savedExec = globalThis.document[execKey];
    globalThis.document[execKey] = (cmd, _b, val) => { calls.push([cmd, val]); return true; };
    try {
      const editor = { isContentEditable: true, innerText: 'old draft', focus() {}, dispatchEvent() {} };
      assert.strictEqual(DOM.setEditableText(editor, 'new comment'), true, 'contenteditable insert returns true');
      assert.strictEqual(calls.filter((c) => c[0] === 'delete').length, 6, 'clear loop bounded to 6 on a stubborn draft');
      const inserts = calls.filter((c) => c[0] === 'insertText');
      assert.strictEqual(inserts.length, 1, 'exactly one insertText');
      assert.strictEqual(inserts[0][1], 'new comment', 'inserts the requested text verbatim');
      assert.strictEqual(DOM.setEditableText({ focus() {} }, 'x'), false, 'editor neither contenteditable nor value → false');
    } finally {
      globalThis.document[execKey] = savedExec;
    }

    console.log('outbound_dom (THGOutboundDom) characterization: PASS');
    restore();
  })().catch((err) => { restore(); throw err; });
} catch (err) {
  restore();
  throw err;
}
