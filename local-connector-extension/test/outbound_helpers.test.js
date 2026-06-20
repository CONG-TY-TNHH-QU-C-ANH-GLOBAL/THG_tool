// PR1 characterization — PURE helpers of content/outbound.js.
//   Run: node local-connector-extension/test/outbound_helpers.test.js
//   CI:  node --test (auto-discovered)
//
// Loaded via loadOutboundWithGlobals (fake browser globals installed BEFORE require,
// require cache cleared) so the module is exercised exactly as the extension would, and
// helpers are read from module.exports._test — NOT from the Chrome runtime globalThis (which
// must stay at the four public methods). These pin CURRENT behavior before the
// Comment/Posting/Inbox extraction; they are the regression net for PR2.
//
// Sequential by construction (single load, no concurrency).
const assert = require('node:assert');
const { loadOutboundWithGlobals } = require('./outbound_test_env');

const { O, api, restore } = loadOutboundWithGlobals();
try {
  const T = O._test;

  // Runtime-surface guard belongs everywhere the module loads: the Chrome globalThis must
  // never carry test helpers (see also outbound_facade.test.js for the full assertion).
  assert.ok(!('_test' in api), 'runtime globalThis must not expose _test');

  // ----- extractPostIdFromUrl — identity-gate URL parser + foreign-host guard ---------
  {
    const f = T.extractPostIdFromUrl;
    assert.strictEqual(f('/groups/100/posts/123456/'), '123456', 'relative group post path');
    assert.strictEqual(f('https://www.facebook.com/someuser/posts/pfbidABC123def'), 'pfbidABC123def', 'pfbid wins over numeric');
    assert.strictEqual(f('https://www.facebook.com/page/permalink/987654/'), '987654', 'numeric permalink');
    assert.strictEqual(f('https://www.facebook.com/watch/?v=555666'), '555666', 'watch ?v= query');
    assert.strictEqual(f('https://www.facebook.com/photo.php?fbid=777888'), '777888', 'photo.php fbid');
    assert.strictEqual(f('https://www.facebook.com/story.php?story_fbid=444555&id=1'), '444555', 'story_fbid query');
    assert.strictEqual(f('https://www.facebook.com/groups/1/?multi_permalinks=111222,333'), '111222', 'multi_permalinks first only');
    assert.strictEqual(f('https://evil.example.com/posts/123456'), '', 'foreign host rejected');
    assert.strictEqual(f('https://shortener.evil/posts/999999'), '', 'foreign host rejected (2)');
    assert.strictEqual(f(''), '', 'empty');
    assert.strictEqual(f(null), '', 'null');
    assert.strictEqual(f('https://www.facebook.com/groups/1/'), '', 'group home, no post id');
  }

  // ----- labelMatchesDismiss — word-boundary matcher (PR8C logo-redirect root cause) --
  {
    const f = T.labelMatchesDismiss;
    assert.strictEqual(f('facebook', ['ok']), false, 'PR8C: "facebook" must NOT match key "ok"');
    assert.strictEqual(f('booklet', ['ok']), false, 'substring "ok" inside a word must not match');
    assert.strictEqual(f('ok', ['ok']), true, 'standalone "ok" matches');
    assert.strictEqual(f('not now', ['not now']), true, 'multi-word decline label');
    assert.strictEqual(f('maybe later', ['later']), true, 'trailing word boundary');
    assert.strictEqual(f('remember password', ['remember password']), true, 'remember-password decline');
  }

  // ----- extractArticleCanonicalEntityId — first post-shape anchor in DOM order wins --
  {
    const f = T.extractArticleCanonicalEntityId;
    const anchor = (href) => ({ getAttribute: (n) => (n === 'href' ? href : null), href });
    const article = (anchors) => ({ querySelectorAll: () => anchors });
    assert.strictEqual(f(article([anchor('/groups/1/posts/123456/')])), '123456', 'first anchor is the identity');
    assert.strictEqual(f(article([anchor('#'), anchor('/posts/789012/')])), '789012', 'skip non-id anchor, take next');
    assert.strictEqual(f(article([])), '', 'no anchors → identity unverifiable');
    assert.strictEqual(f(null), '', 'null article → ""');
  }

  // ----- isInsidePostContainer — protects the target post's own Close/X from dismissal-
  {
    const f = T.isInsidePostContainer;
    assert.strictEqual(f({ closest: () => ({ querySelector: () => ({ tagName: 'A' }) }) }), true, 'inside a post container → protected');
    assert.strictEqual(f({ closest: () => ({ querySelector: () => null }) }), false, 'dialog without permalink → dismissable');
    assert.strictEqual(f({ closest: () => null }), false, 'no enclosing container → false');
  }

  // ----- abbreviate / norm / hasAny / enabledButton / textOfEditable ------------------
  {
    assert.strictEqual(T.abbreviate(''), '<missing>');
    assert.strictEqual(T.abbreviate(null), '<missing>');
    assert.strictEqual(T.abbreviate('short'), 'short');
    assert.strictEqual(T.abbreviate('a'.repeat(20)), 'a'.repeat(16) + '…', 'long id truncated to 16 + ellipsis');

    assert.strictEqual(T.norm('Bình Luận'), 'binh luan', 'diacritics stripped + lowercased');
    assert.strictEqual(T.norm('  ĐÂY '), 'day', 'Đ -> d + trim');

    assert.strictEqual(T.hasAny('hello world', ['xyz', 'wor']), true);
    assert.strictEqual(T.hasAny('hello world', ['xyz']), false);

    assert.strictEqual(T.enabledButton({ getAttribute: () => null, disabled: false }), true);
    assert.strictEqual(T.enabledButton({ getAttribute: () => 'true', disabled: false }), false, 'aria-disabled');
    assert.strictEqual(T.enabledButton({ getAttribute: () => null, disabled: true }), false, 'disabled');
    assert.ok(!T.enabledButton(null), 'null button not enabled');

    assert.strictEqual(T.textOfEditable({ value: 'hi' }), 'hi', 'value wins');
    assert.strictEqual(T.textOfEditable({ innerText: 'yo' }), 'yo', 'innerText fallback');
    assert.strictEqual(T.textOfEditable({ textContent: 'tc' }), 'tc', 'textContent fallback');
    assert.strictEqual(T.textOfEditable(null), '', 'null → ""');
  }

  // ----- editorContainsContent — 60-char sample match, detached-editor guard ----------
  {
    const f = T.editorContainsContent;
    globalThis.document.contains = () => true;
    assert.strictEqual(f({ value: 'Hello there friend' }, 'Hello there'), true, 'sample prefix matches');
    assert.strictEqual(f({ value: 'Hello there friend' }, ''), false, 'empty expected → false');
    globalThis.document.contains = () => false;
    assert.strictEqual(f({ value: 'Hello there friend' }, 'Hello there'), false, 'detached editor → false');
    assert.strictEqual(f(null, 'x'), false, 'null editor → false');
    globalThis.document.contains = () => true;
  }

  // ----- onTargetPermalinkPage — URL pins identity on the post's own page -------------
  {
    const f = T.onTargetPermalinkPage;
    const savedHref = globalThis.location.href;
    globalThis.location.href = 'https://www.facebook.com/groups/1/posts/123456/';
    assert.strictEqual(f('123456'), true, 'URL post id === target → true');
    assert.strictEqual(f('999999'), false, 'different id → false');
    assert.strictEqual(f(''), false, 'empty target → false');
    globalThis.location.href = savedHref;
  }

  console.log('outbound pure-helper characterization: PASS');
} finally {
  restore();
}
