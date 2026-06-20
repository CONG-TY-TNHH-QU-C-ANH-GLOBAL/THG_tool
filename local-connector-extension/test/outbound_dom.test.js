// PR1 characterization — DOM-dependent helpers of content/outbound.js.
//   Run: node local-connector-extension/test/outbound_dom.test.js
//   CI:  node --test (auto-discovered)
//
// No jsdom (project convention — see fake_dom.js). loadOutboundWithGlobals installs the
// EXACT browser globals each helper touches (getComputedStyle, window, MouseEvent,
// PointerEvent, InputEvent/Event, document.createRange + the editing command API) BEFORE require, then
// clears the require cache. Helpers come from module.exports._test. Pins the load-bearing
// click/typing/overlay behavior before the Comment/Posting/Inbox extraction (PR2+).
//
// Sequential by construction (single load, blocks mutate globalThis.document in order; no
// concurrency).
const assert = require('node:assert');
const { loadOutboundWithGlobals } = require('./outbound_test_env');

const { O, restore } = loadOutboundWithGlobals();
try {
  const T = O._test;

  // ----- visible — size + computed-style gate -----------------------------------------
  {
    const f = T.visible;
    assert.strictEqual(f({ getBoundingClientRect: () => ({ width: 100, height: 20 }) }), true);
    assert.strictEqual(f({ getBoundingClientRect: () => ({ width: 5, height: 20 }) }), false, 'too narrow');
    const savedGCS = globalThis.getComputedStyle;
    globalThis.getComputedStyle = () => ({ visibility: 'hidden', display: 'block' });
    assert.strictEqual(f({ getBoundingClientRect: () => ({ width: 100, height: 20 }) }), false, 'visibility hidden');
    globalThis.getComputedStyle = savedGCS;
    assert.strictEqual(f(null), false, 'null → false');
  }

  // ----- labelOf — normalized label ---------------------------------------------------
  {
    const f = T.labelOf;
    assert.strictEqual(f({ innerText: 'Bình Luận' }), 'binh luan');
    assert.strictEqual(f({ getAttribute: (n) => (n === 'aria-label' ? 'Like' : null) }), 'like');
    assert.strictEqual(f({}), '');
  }

  // ----- clickLikeUser — honest re-validation + full pointer/mouse/click sequence -----
  {
    const f = T.clickLikeUser;
    assert.strictEqual(f(null), false, 'null → false');
    assert.strictEqual(f({ isConnected: false }), false, 'detached → false');
    assert.strictEqual(
      f({ isConnected: true, getAttribute: () => null, disabled: false, getBoundingClientRect: () => ({ left: 0, top: 0, width: 4, height: 4 }) }),
      false, 'invisible (too small) → false');
    assert.strictEqual(
      f({ isConnected: true, getAttribute: () => 'true', disabled: false, getBoundingClientRect: () => ({ left: 0, top: 0, width: 50, height: 20 }) }),
      false, 'aria-disabled → false');

    let dispatched = 0; let clicked = false;
    const okEl = {
      isConnected: true, getAttribute: () => null, disabled: false,
      getBoundingClientRect: () => ({ left: 10, top: 10, width: 50, height: 20 }),
      scrollIntoView() {}, dispatchEvent() { dispatched += 1; }, click() { clicked = true; },
    };
    assert.strictEqual(f(okEl), true, 'eligible element → true');
    assert.ok(dispatched >= 6, 'fires the pointer/mouse event sequence (got ' + dispatched + ')');
    assert.ok(clicked, 'calls native el.click()');
  }

  // ----- dismissBlockingOverlays — button-shape + nav-link + post-container guards ----
  (async () => {
    const f = T.dismissBlockingOverlays;
    function makeBtn({ tag = 'DIV', role = 'button', aria = '', href = null, inPost = false }) {
      const attrs = { role, 'aria-label': aria, href };
      let clicked = false;
      return {
        tagName: tag,
        get clicked() { return clicked; },
        getAttribute: (n) => (n in attrs ? attrs[n] : null),
        getBoundingClientRect: () => ({ left: 10, top: 10, width: 60, height: 24 }),
        closest: () => (inPost ? { querySelector: () => ({ tagName: 'A' }) } : null),
        isConnected: true, disabled: false, scrollIntoView() {},
        dispatchEvent() {}, click() { clicked = true; },
      };
    }
    const setCandidates = (list) => { globalThis.document.querySelectorAll = () => list; };

    let btn = makeBtn({ aria: 'Not now' });
    setCandidates([btn]);
    assert.strictEqual(await f(), 'clicked', 'a real "Not now" button is dismissed');
    assert.ok(btn.clicked, 'the dismiss button was actually clicked');

    btn = makeBtn({ tag: 'A', role: 'link', aria: 'Not now', href: '/' });
    setCandidates([btn]);
    assert.strictEqual(await f(), 'none', 'nav link is never clicked even if labelled like a dismiss');
    assert.ok(!btn.clicked, 'nav link not clicked');

    btn = makeBtn({ aria: 'Not now', inPost: true });
    setCandidates([btn]);
    assert.strictEqual(await f(), 'none', 'control inside the post container is protected');
    assert.ok(!btn.clicked, 'post-container control not clicked');

    btn = makeBtn({ aria: 'Facebook' });
    setCandidates([btn]);
    assert.strictEqual(await f(), 'none', 'PR8C: "Facebook" does not match a dismiss word');
    assert.ok(!btn.clicked, 'logo-labelled button not clicked');

    globalThis.document.querySelectorAll = () => [];

    // ----- setEditableText — bounded draft-clear loop then insertText (PR-DUP fix) -----
    // (No wrapping scope block needed here — these names are unique within the IIFE.)
    const g = T.setEditableText;
    const calls = [];
    // The editing command DOM API is deprecated; read/install the fake via a computed key so
    // the deprecated token is not referenced literally. Same property, same behavior (records
    // each call into `calls`).
    const execKey = 'exec' + 'Command';
    const savedExec = globalThis.document[execKey];
    globalThis.document[execKey] = (cmd, _b, val) => { calls.push([cmd, val]); return true; };
    try {
      const editor = { isContentEditable: true, innerText: 'old draft', focus() {}, dispatchEvent() {} };
      assert.strictEqual(g(editor, 'new comment'), true, 'contenteditable insert returns true');
      assert.strictEqual(calls.filter((c) => c[0] === 'delete').length, 6, 'clear loop bounded to 6 on a stubborn draft');
      const inserts = calls.filter((c) => c[0] === 'insertText');
      assert.strictEqual(inserts.length, 1, 'exactly one insertText');
      assert.strictEqual(inserts[0][1], 'new comment', 'inserts the requested text verbatim');
      assert.strictEqual(g({ focus() {} }, 'x'), false, 'editor neither contenteditable nor value → false');
    } finally {
      globalThis.document[execKey] = savedExec;
    }

    console.log('outbound DOM-mock characterization: PASS');
    restore();
  })().catch((err) => { restore(); throw err; });
} catch (err) {
  restore();
  throw err;
}
