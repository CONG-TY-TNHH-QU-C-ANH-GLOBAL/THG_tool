// PR1 characterization — executeOutbound FACADE dispatch, RUNTIME-SURFACE guard,
// require-under-fake-globals SMOKE test, and MANIFEST load-order invariant.
//   Run: node local-connector-extension/test/outbound_facade.test.js
//   CI:  node --test (auto-discovered)
//
// Loaded via loadOutboundWithGlobals with the real comment/proof/navreport modules so the
// dispatch lands in each layer's REAL body. Pins the connector-protocol contract: the
// Chrome runtime global exposes EXACTLY the four public methods and never the test seam.
//
// Sequential by construction (no concurrency; shared globalThis mutation).
const assert = require('assert');
const fs = require('fs');
const path = require('path');
const { loadOutboundWithGlobals } = require('./outbound_test_env');

const REAL_MODULES = [
  '../content/comment_composer', '../content/comment_button', '../content/comment_constants',
  '../content/proof', '../content/navreport',
];

(async () => {
  // SMOKE: require('../content/outbound.js') must succeed under the minimal fake browser
  // global setup (no DOM library). If this throws, the stack names the offending global.
  const { O, api, restore } = loadOutboundWithGlobals({ realModules: REAL_MODULES });
  try {
    assert.ok(O && typeof O.executeOutbound === 'function', 'smoke: module requires + exposes executeOutbound');

    // ---- RUNTIME-SURFACE guard: Chrome global = exactly the 4 public methods ----------
    assert.deepStrictEqual(
      Object.keys(api).sort((a, b) => a.localeCompare(b)),
      ['executeCommentInFeed', 'executeCommentViaRung2', 'executeOutbound', 'probeRung2Click'],
      'Chrome runtime global must expose exactly the four public methods');
    assert.ok(!('_test' in api), 'runtime global must NOT expose _test');
    assert.ok(Object.keys(api).every((k) => !k.startsWith('_')), 'runtime global must NOT expose any _ helper key');
    assert.strictEqual(global.THGContentOutbound, api, 'globalThis.THGContentOutbound IS the 4-key api object');

    // ---- _test exists only on module.exports (Node), with the selected helpers --------
    assert.ok(O._test && typeof O._test === 'object', 'module.exports._test exists in Node');
    for (const h of ['extractPostIdFromUrl', 'labelMatchesDismiss', 'clickLikeUser', 'setEditableText', 'dismissBlockingOverlays']) {
      assert.strictEqual(typeof O._test[h], 'function', 'module.exports._test.' + h + ' is a function');
    }

    // ---- Content guards (return BEFORE any type dispatch / DOM work) -------------------
    assert.deepStrictEqual(await O.executeOutbound({ type: 'comment', content: '' }),
      { ok: false, error: 'outbox_content_empty' }, 'empty content rejected');
    assert.deepStrictEqual(await O.executeOutbound({ type: 'comment', content: '   ' }),
      { ok: false, error: 'outbox_content_empty' }, 'whitespace-only content rejected');
    assert.deepStrictEqual(await O.executeOutbound({ type: 'comment', content: 'x'.repeat(3001) }),
      { ok: false, error: 'outbox_content_too_long' }, 'over-3000 content rejected');

    // ---- Unknown type + type normalization (trim + lowercase) -------------------------
    assert.deepStrictEqual(await O.executeOutbound({ type: 'frobnicate', content: 'hi' }),
      { ok: false, error: 'unsupported_outbox_type:frobnicate' }, 'unknown type reported');
    assert.deepStrictEqual(await O.executeOutbound({ type: '  COMMENT_X ', content: 'hi' }),
      { ok: false, error: 'unsupported_outbox_type:comment_x' }, 'type is trimmed + lowercased');

    // ---- Positive routing: each type reaches its OWN layer's distinct not-found error -
    const r1 = await O.executeOutbound({ type: 'comment', content: 'hi' });
    assert.strictEqual(r1.error, 'comment_box_not_found', 'comment routes to executeComment');
    const r2 = await O.executeOutbound({ type: 'inbox', content: 'hi' });
    assert.strictEqual(r2.error, 'message_button_not_found', 'inbox routes to executeInbox');
    const r3 = await O.executeOutbound({ type: 'group_post', content: 'hi' });
    assert.strictEqual(r3.error, 'post_composer_not_found', 'group_post routes to executePost');
    const r4 = await O.executeOutbound({ type: 'profile_post', content: 'hi' });
    assert.strictEqual(r4.error, 'post_composer_not_found', 'profile_post routes to executePost');

    console.log('outbound facade-dispatch + runtime-surface characterization: PASS');
  } finally {
    restore();
  }

  // ---- MANIFEST load-order invariant — the real boundary enforcer for PR2+ modules ---
  // Synchronous, no globals; the reusable assertLoadsBefore pins provider→consumer order
  // so PR2 can extend it for outbound_dom.js / outbound_identity.js / outbound_diag.js.
  const manifest = JSON.parse(fs.readFileSync(path.join(__dirname, '..', 'manifest.json'), 'utf8'));
  const js = manifest.content_scripts[0].js;
  const idx = (needle) => js.findIndex((p) => p.indexOf(needle) !== -1);
  function assertLoadsBefore(provider, consumer) {
    const p = idx(provider); const c = idx(consumer);
    assert.ok(p !== -1, 'manifest lists provider ' + provider);
    assert.ok(c !== -1, 'manifest lists consumer ' + consumer);
    assert.ok(p < c, 'load order: ' + provider + ' must precede ' + consumer + ' (got ' + p + ' vs ' + c + ')');
  }
  assertLoadsBefore('comment_constants.js', 'comment_composer.js');
  assertLoadsBefore('comment_composer.js', 'comment_button.js');
  assertLoadsBefore('comment_button.js', 'outbound.js');
  assertLoadsBefore('comment_constants.js', 'outbound.js');
  assertLoadsBefore('comment_submit.js', 'outbound.js');
  assertLoadsBefore('comment_state_machine.js', 'outbound.js');
  assertLoadsBefore('comment_composer_guard.js', 'outbound.js');
  assertLoadsBefore('proof.js', 'outbound.js');
  assertLoadsBefore('navreport.js', 'outbound.js');
  assertLoadsBefore('forensics.js', 'outbound.js');
  assertLoadsBefore('outbound.js', 'comment_executor.js');
  assertLoadsBefore('outbound.js', 'bridge.js');
  console.log('outbound manifest load-order invariant: PASS');
})().catch((err) => { console.error(err); process.exit(1); });
