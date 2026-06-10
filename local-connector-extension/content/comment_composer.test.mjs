// Gate1 composer discovery tests — driven by the operator's real DOM evidence: a visible
// role=textbox contenteditable composer (aria "Write an answer…") that lives OUTSIDE the
// [role="article"] subtree must be found (page-wide, scoped), while the global create-post
// composer and a neighbouring post's composer must be rejected with a clear reason.
//   Run: node content/comment_composer.test.mjs
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const CC = require('./comment_composer.js');

// Fake editable element matching the operator's diagnostic shape.
function editable(opts) {
  const o = opts || {};
  return {
    tagName: o.tag || 'DIV',
    _closest: o.closest || null,
    parentElement: { textContent: o.parentText || o.aria || '' },
    getAttribute(n) {
      if (n === 'role') return o.role || null;
      if (n === 'contenteditable') return o.contenteditable || null;
      if (n === 'aria-label') return o.aria || null;
      if (n === 'placeholder') return o.placeholder || null;
      return null;
    },
  };
}
const articleWith = (editables) => ({
  querySelectorAll: (sel) =>
    (sel.includes('textbox') || sel.includes('contenteditable') || sel.includes('textarea')) ? editables : [],
});
const depsFor = (docEditables, closest) => ({
  visible: () => true,
  closestArticle: (el) => (closest ? closest(el) : el._closest),
  docEditables: () => docEditables,
});

// 1. The EXACT operator case: visible DIV role=textbox contenteditable aria="Write an answer…"
//    present in the TARGET article subtree → PASS via in_target_article.
{
  const ed = editable({ role: 'textbox', contenteditable: 'true', aria: 'Write an answer…' });
  const r = CC.findComposerEntry(articleWith([ed]), { visible: () => true });
  assert.strictEqual(r.el, ed);
  assert.strictEqual(r.reason, 'in_target_article');
}

// 2. Composer OUTSIDE the article subtree but its closest [role=article] IS the target → PASS.
{
  const art = articleWith([]); // empty subtree
  const ed = editable({ role: 'textbox', contenteditable: 'true', aria: 'Write an answer…', closest: art });
  const r = CC.findComposerEntry(art, depsFor([ed]));
  assert.strictEqual(r.el, ed);
  assert.strictEqual(r.reason, 'in_target_article');
}

// 3. Composer not nested in any article, but reads as answer/comment/reply → target_discussion_region.
{
  const art = articleWith([]);
  const ed = editable({ role: 'textbox', contenteditable: 'true', aria: 'Write an answer…', closest: null });
  const r = CC.findComposerEntry(art, depsFor([ed]));
  assert.strictEqual(r.el, ed);
  assert.strictEqual(r.reason, 'target_discussion_region');
}

// 4. The GLOBAL create-post composer must be rejected.
{
  const art = articleWith([]);
  const ed = editable({ role: 'textbox', contenteditable: 'true', aria: "What's on your mind?", closest: null });
  const r = CC.findComposerEntry(art, depsFor([ed]));
  assert.strictEqual(r.el, null);
  assert.strictEqual(r.candidates[0].reason, 'create_post_composer');
}

// 5. A neighbouring post's composer (closest is a DIFFERENT article) must be rejected.
{
  const art = articleWith([]);
  const neighbour = {};
  const ed = editable({ role: 'textbox', contenteditable: 'true', aria: 'Write a comment…', closest: neighbour });
  const r = CC.findComposerEntry(art, depsFor([ed]));
  assert.strictEqual(r.el, null);
  assert.strictEqual(r.candidates[0].reason, 'wrong_post');
}

// 6. No button and no composer → none.
{
  const r = CC.findComposerEntry(articleWith([]), depsFor([]));
  assert.strictEqual(r.el, null);
  assert.strictEqual(r.reason, 'none');
}

// 7. Invisible candidate → rejected with reason invisible.
{
  const ed = editable({ role: 'textbox', contenteditable: 'true', aria: 'Write a comment…' });
  const r = CC.findComposerEntry(articleWith([]), { visible: () => false, docEditables: () => [ed] });
  assert.strictEqual(r.el, null);
  assert.strictEqual(r.candidates[0].reason, 'invisible');
}

// textarea + bare contenteditable shapes are recognised.
{
  assert.strictEqual(CC.isEditableShape(editable({ tag: 'TEXTAREA' })), true);
  assert.strictEqual(CC.isEditableShape(editable({ contenteditable: 'true' })), true);
  assert.strictEqual(CC.isEditableShape(editable({ role: 'button' })), false);
}

console.log('comment_composer.test.mjs OK');
