// Characterization — comment IDENTITY/misc helpers. These were extracted to the commenting
// layer in PR5: identity/target helpers now live on THGCommentingTarget, and abbreviate/
// editorContainsContent on THGCommentingOutbound._test (Node-only). Assertions are unchanged
// from when they lived in outbound.js — only the source module moved.
//   Run: node local-connector-extension/test/outbound_helpers.test.js
//   CI:  node --test (auto-discovered)
//
// Sequential by construction (single load, no concurrency).
const assert = require('node:assert');
const { loadCommentingOutbound } = require('./outbound_test_env');

const { CMT, restore } = loadCommentingOutbound();
try {
  const T = globalThis.THGCommentingTarget; // identity/target helpers (PR5)
  const X = CMT._test;                       // abbreviate / editorContainsContent (Node-only)

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

  // ----- abbreviate -------------------------------------------------------------------
  assert.strictEqual(X.abbreviate(''), '<missing>');
  assert.strictEqual(X.abbreviate(null), '<missing>');
  assert.strictEqual(X.abbreviate('short'), 'short');
  assert.strictEqual(X.abbreviate('a'.repeat(20)), 'a'.repeat(16) + '…', 'long id truncated to 16 + ellipsis');

  // ----- editorContainsContent — 60-char sample match, detached-editor guard ----------
  {
    const f = X.editorContainsContent;
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

  console.log('commenting identity/misc helper characterization: PASS');
} finally {
  restore();
}
