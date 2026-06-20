// Gate1 comment-surface discovery integration regression.
//   Run: node local-connector-extension/test/comment_button.test.js
//
// Proves the gate1 ENTRY layer (comment_button.js) delegates to the composer module and PASSES
// via composer_entry on the live failing shape: target article + permalink + action row, NO
// comment button, and a sibling discussion composer role=textbox contenteditable=true
// aria="Write an answer…".
const assert = require('assert');
const { makeEl, makeArticle, sizeVisible } = require('./fake_dom');
require('../content/facebook/commenting/comment_composer'); // sets globalThis.THGCommentComposer (button.js depends on it)
const B = require('../content/facebook/commenting/comment_button');
const K = require('../content/facebook/commenting/comment_constants');

// S6 lock-in: the comment vocabulary is sourced from the ONE shared constants module
// (no per-file re-declaration that can drift). comment_button.js must expose exactly it.
assert.strictEqual(B.COMMENT_KEYS, K.COMMENT_KEYS, 'COMMENT_KEYS must come from THGCommentConstants');

const labelOf = (el) => String(el.getAttribute('aria-label') || '').toLowerCase();
const answerBox = () => makeEl({ role: 'textbox', ce: 'true', aria: 'Write an answer…', parentText: 'Write an answer…', w: 450, h: 20 });
const likeBtn = makeEl({ tag: 'DIV', role: 'button', aria: 'Like' });
const shareBtn = makeEl({ tag: 'DIV', role: 'button', aria: 'Share' });

// Build the live fixture: action row present (Like/Share), no comment button, composer is a
// page-wide sibling (docEditables), host identity unknown.
function liveDeps(box) {
  return {
    visible: sizeVisible, labelOf,
    closestArticle: () => ({}),
    classifyHost: () => 'unknown',
    docEditables: () => [box],
    wait: async () => {}, now: () => 0,
    scrollIntoCenter: () => {},
    timeoutMs: 1000, pollMs: 10,
  };
}

// 1) diagnostics: composer_entry_found=true, gate1_passed_via='composer_entry', action_row_found.
{
  const art = makeArticle({ buttons: [likeBtn, shareBtn], editables: [], permalink: true });
  const d = B.diagnostics(art, liveDeps(answerBox()));
  assert.strictEqual(d.article_found, true);
  assert.strictEqual(d.permalink_found, true);
  assert.strictEqual(d.action_row_found, true, 'Like/Share row should register action_row_found');
  assert.strictEqual(d.comment_button_found, false, 'no Comment button on this fixture');
  assert.strictEqual(d.composer_entry_found, true, 'sibling answer composer must be discovered');
  assert.strictEqual(d.gate1_passed_via, 'composer_entry');
  assert.strictEqual(d.composer_candidates.length, 1);
  assert.strictEqual(d.composer_candidates[0].accepted, true);
}

// 2) discoverCommentSurface returns found via composer_entry on the same fixture.
(async () => {
  const art = makeArticle({ buttons: [likeBtn, shareBtn], editables: [], permalink: true });
  const r = await B.discoverCommentSurface(art, liveDeps(answerBox()));
  assert.strictEqual(r.found, true, 'discoverCommentSurface must find the surface');
  assert.strictEqual(r.via, 'composer_entry');

  // 3) classifyGate1Failure: reached post but no entry → comment_button_not_found.
  assert.strictEqual(
    B.classifyGate1Failure({ articleFound: true, permalinkFound: true, commentButtonFound: false, composerEntryFound: false }),
    'comment_button_not_found');

  // 4) DELEGATION PROOF: comment_button.js routes composer discovery THROUGH the composer module.
  const realFind = globalThis.THGCommentComposer.findComposerEntry;
  let called = false;
  globalThis.THGCommentComposer.findComposerEntry = (article, deps) => { called = true; return realFind(article, deps); };
  try {
    B.commentSurfaceState(makeArticle({ buttons: [], editables: [], permalink: true }), liveDeps(answerBox()));
  } finally {
    globalThis.THGCommentComposer.findComposerEntry = realFind;
  }
  assert.strictEqual(called, true, 'commentSurfaceState must call THGCommentComposer.findComposerEntry');

  // 5) UNIFIED HANDOFF (P0b): the element gate1 ACCEPTS must be the SAME element editor
  //    acquisition returns. Both go through THGCommentComposer.findComposerEntry with the SAME
  //    deps (commentSurfaceDeps), so a composer gate1 passed can never be lost by the editor
  //    finder — the divergence that produced comment_box_not_found after gate1 succeeded.
  {
    const box = answerBox();
    const art = makeArticle({ buttons: [likeBtn, shareBtn], editables: [], permalink: true });
    const deps = liveDeps(box);
    const d = B.diagnostics(art, deps);                 // gate1 view
    assert.strictEqual(d.composer_entry_found, true);
    assert.strictEqual(d.gate1_passed_via, 'composer_entry');
    const acquired = globalThis.THGCommentComposer.findComposerEntry(art, deps).el; // acquisition view
    assert.strictEqual(acquired, box, 'editor acquisition must return the same composer gate1 accepted');
  }

  console.log('gate1 comment_button integration regression: PASS');
})();
