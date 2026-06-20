// Gate1 composer discovery regression (the 0.5.51 live false-negative).
//   Run: node local-connector-extension/test/comment_composer.test.js
//
// Root cause guarded here: the strict `host === article` identity check returned wrong_post
// for a legitimate group "Write an answer…" composer whose nearest [role=article] was a
// NON-target node (comment item / wrapper), short-circuiting before the answer/reply keyword
// acceptance. The fix injects a channel verdict deps.classifyHost(host) and only hard-rejects
// wrong_post on a positive 'foreign' id match.
const assert = require('assert');
const { makeEl, makeArticle, sizeVisible } = require('./fake_dom');
const C = require('../content/facebook/commenting/composer/comment_composer');

// The exact live shape from the operator's manual DOM probe.
const liveAnswerBox = () => makeEl({
  tag: 'DIV', role: 'textbox', ce: 'true', aria: 'Write an answer…', parentText: 'Write an answer…', w: 450, h: 20,
});

// classifyHost stubs: 'unknown' = comment item / wrapper (no own permalink); 'foreign' = a
// positively different post; 'target' = same post.
const hostUnknown = () => 'unknown';
const hostForeign = () => 'foreign';
const hostTarget = () => 'target';

// 1) LIVE CASE — answer composer outside the article, host identity unknown → accepted by shape
//    ("answer" keyword), NOT rejected wrong_post. This is the case 0.5.51 false-failed.
{
  const el = liveAnswerBox();
  const r = C.classify(el, makeArticle(), { visible: sizeVisible, closestArticle: () => ({}), classifyHost: hostUnknown });
  assert.strictEqual(r.accepted, true, 'live answer composer must be accepted');
  assert.strictEqual(r.reason, 'target_discussion_region', 'reason should be target_discussion_region');
}

// 2) Same composer, host positively resolves to the TARGET post → in_target_article.
{
  const r = C.classify(liveAnswerBox(), makeArticle(), { visible: sizeVisible, closestArticle: () => ({}), classifyHost: hostTarget });
  assert.strictEqual(r.accepted, true);
  assert.strictEqual(r.reason, 'in_target_article');
}

// 3) Neighbouring post's comment composer (host id != target) → wrong_post, even though it
//    reads like a comment composer. The injected 'foreign' verdict must win.
{
  const el = makeEl({ role: 'textbox', ce: 'true', aria: 'Write a comment…', parentText: 'Write a comment…' });
  const r = C.classify(el, makeArticle(), { visible: sizeVisible, closestArticle: () => ({}), classifyHost: hostForeign });
  assert.strictEqual(r.accepted, false);
  assert.strictEqual(r.reason, 'wrong_post');
}

// 4) Global create-post composer → create_post_composer (never satisfies a post gate).
{
  const el = makeEl({ role: 'textbox', ce: 'true', aria: "What's on your mind?", parentText: "What's on your mind?" });
  const r = C.classify(el, makeArticle(), { visible: sizeVisible, closestArticle: () => null, classifyHost: hostUnknown });
  assert.strictEqual(r.accepted, false);
  assert.strictEqual(r.reason, 'create_post_composer');
}

// 5) Invisible composer → invisible.
{
  const el = makeEl({ role: 'textbox', ce: 'true', aria: 'Write an answer…', w: 0, h: 0 });
  const r = C.classify(el, makeArticle(), { visible: sizeVisible, closestArticle: () => ({}), classifyHost: hostUnknown });
  assert.strictEqual(r.accepted, false);
  assert.strictEqual(r.reason, 'invisible');
}

// 6) Editable nested INSIDE the target article subtree (contains) → in_target_article,
//    robust to nested inner articles regardless of closestArticle.
{
  const el = liveAnswerBox();
  const art = makeArticle({ containsList: [el] });
  const r = C.classify(el, art, { visible: sizeVisible, closestArticle: () => ({}), classifyHost: hostForeign });
  assert.strictEqual(r.accepted, true, 'subtree containment must win over a foreign closest-article');
  assert.strictEqual(r.reason, 'in_target_article');
}

// 7) Non-editable element → unsupported_editable_shape.
{
  const el = makeEl({ tag: 'DIV', role: 'button', aria: 'Like' });
  const r = C.classify(el, makeArticle(), { visible: sizeVisible, closestArticle: () => null, classifyHost: hostUnknown });
  assert.strictEqual(r.accepted, false);
  assert.strictEqual(r.reason, 'unsupported_editable_shape');
}

// 8) findComposerEntry end-to-end: article subtree empty, page-wide docEditables holds the live
//    answer box → returns it with per-candidate diagnostics.
{
  const el = liveAnswerBox();
  const out = C.findComposerEntry(makeArticle(), {
    visible: sizeVisible, closestArticle: () => ({}), classifyHost: hostUnknown, docEditables: () => [el],
  });
  assert.strictEqual(out.el, el, 'findComposerEntry must return the live answer box');
  assert.strictEqual(out.reason, 'target_discussion_region');
  assert.strictEqual(out.candidates.length, 1);
  assert.strictEqual(out.candidates[0].accepted, true);
  assert.strictEqual(out.candidates[0].aria, 'Write an answer…');
}

// 9) hostVerdict (the live permalink fix): on the target's OWN permalink page (urlPinsIdentity),
//    a host article that extracts a DIFFERENT id is a nested comment/answer item → 'unknown'
//    (NOT 'foreign'), so the answer composer is no longer false-rejected wrong_post. On a FEED
//    page the same differing id is a genuinely different post → 'foreign'.
{
  const T = '2040078973566103';
  assert.strictEqual(C.hostVerdict({ hostId: T, targetId: T, urlPinsIdentity: true }), 'target');
  assert.strictEqual(C.hostVerdict({ hostId: T, targetId: T, urlPinsIdentity: false }), 'target');
  // The exact live failure: foreign host id, but we are on the target's own permalink page.
  assert.strictEqual(C.hostVerdict({ hostId: '999000111', targetId: T, urlPinsIdentity: true }), 'unknown',
    'permalink page must downgrade a foreign host to unknown (the 204 fix)');
  // Feed page keeps strict wrong-post protection.
  assert.strictEqual(C.hostVerdict({ hostId: '999000111', targetId: T, urlPinsIdentity: false }), 'foreign');
  // No id resolvable → unknown either way.
  assert.strictEqual(C.hostVerdict({ hostId: '', targetId: T, urlPinsIdentity: false }), 'unknown');
  assert.strictEqual(C.hostVerdict({ hostId: T, targetId: '', urlPinsIdentity: true }), 'unknown');
}

// 10) End-to-end of the live fix path: foreign-host candidate on a permalink page resolves via
//     hostVerdict('unknown') → answer keyword → accepted (this is what makes gate1 pass now).
{
  const T = '2040078973566103';
  const el = liveAnswerBox();
  const permalinkVerdict = (host) => C.hostVerdict({ hostId: '999000111', targetId: T, urlPinsIdentity: true });
  const r = C.classify(el, makeArticle(), { visible: sizeVisible, closestArticle: () => ({}), classifyHost: permalinkVerdict });
  assert.strictEqual(r.accepted, true, 'foreign-host answer composer on permalink page must be accepted');
  assert.strictEqual(r.reason, 'target_discussion_region');
  // Same candidate on a FEED page (foreign) stays rejected.
  const feedVerdict = () => C.hostVerdict({ hostId: '999000111', targetId: T, urlPinsIdentity: false });
  const r2 = C.classify(liveAnswerBox(), makeArticle(), { visible: sizeVisible, closestArticle: () => ({}), classifyHost: feedVerdict });
  assert.strictEqual(r2.accepted, false);
  assert.strictEqual(r2.reason, 'wrong_post');
}

// 11) Negative: on the target permalink page, a truly UNRELATED composer (no comment/answer/
//     reply shape, non-target host) must NOT be accepted just because the URL pins identity.
{
  const T = '2040078973566103';
  const el = makeEl({ role: 'textbox', ce: 'true', aria: 'Add a poll option', parentText: 'Add a poll option' });
  const verdict = () => C.hostVerdict({ hostId: '999000111', targetId: T, urlPinsIdentity: true });
  const r = C.classify(el, makeArticle(), { visible: sizeVisible, closestArticle: () => ({}), classifyHost: verdict });
  assert.strictEqual(r.accepted, false, 'an unrelated permalink composer must still be rejected');
}

console.log('gate1 comment_composer regression: PASS');
