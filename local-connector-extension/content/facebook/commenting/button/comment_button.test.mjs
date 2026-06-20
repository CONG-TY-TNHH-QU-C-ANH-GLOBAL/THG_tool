// Gate1 comment-ENTRY discovery tests. Gate1 must pass if EITHER a comment button OR a
// visible composer exists under the TARGET article; a neighbouring post's composer must not
// satisfy it; lazily-mounted button/composer are found after scroll; neither → not found.
//   Run: node content/comment_button.test.mjs
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
require('../composer/comment_composer.js'); // sets globalThis.THGCommentComposer (composer delegation)
const CB = require('./comment_button.js');

const visible = () => true;
const labelOf = (el) => String((el && el._label) || '').toLowerCase();
const deps = { visible, labelOf, findCommentEditor: (a) => a && a._editor };

function btn(label) {
  return { _label: label, getAttribute: (n) => (n === 'aria-label' ? label : null) };
}
const composerEl = () => ({ getAttribute: () => null });

// makeArticle routes selector strings to the right fake nodes (scoped to THIS article).
function makeArticle(state) {
  const get = typeof state === 'function' ? state : () => state;
  return {
    querySelector(sel) {
      if (sel.includes('/posts/') || sel.includes('permalink') || sel.includes('story_fbid')) {
        return get().permalink ? {} : null;
      }
      return null;
    },
    querySelectorAll(sel) {
      const o = get();
      if (sel.includes('role="button"')) return o.buttons || []; // BUTTON_SEL
      if (sel.includes('contenteditable') || sel.includes('role="textbox"') || sel.includes('textarea')) return o.composers || [];
      if (sel.includes('aria-label') || sel.includes('placeholder')) return o.placeholders || [];
      return [];
    },
    get _editor() { return get().editor || null; },
  };
}

// 1 + 6. No action button but a visible composer (permalink / Anonymous-participant shape) →
// gate1 passes via composer_entry.
{
  const a = makeArticle({ permalink: true, composers: [composerEl()] });
  const st = CB.commentSurfaceState(a, deps);
  assert.strictEqual(st.found, true);
  assert.strictEqual(st.via, 'composer_entry');
}
// 2. Action button delayed until after a scroll → discovery eventually passes.
{
  let t = 0;
  const a = makeArticle(() => (t >= 800 ? { buttons: [btn('Comment')] } : {}));
  const d = { ...deps, scrollIntoCenter: () => {}, wait: async () => { t += 400; }, now: () => t, timeoutMs: 12000, pollMs: 400 };
  const r = await CB.discoverCommentSurface(a, d);
  assert.strictEqual(r.found, true);
  assert.strictEqual(r.via, 'comment_button');
}

// 3. Composer delayed until after a scroll → discovery eventually passes via composer_entry.
{
  let t = 0;
  const a = makeArticle(() => (t >= 800 ? { composers: [composerEl()] } : {}));
  const d = { ...deps, scrollIntoCenter: () => {}, wait: async () => { t += 400; }, now: () => t, timeoutMs: 12000, pollMs: 400 };
  const r = await CB.discoverCommentSurface(a, d);
  assert.strictEqual(r.found, true);
  assert.strictEqual(r.via, 'composer_entry');
}

// 4. Reached the post but neither button nor composer → not found → comment_button_not_found.
{
  let t = 0;
  const a = makeArticle({ permalink: true }); // no buttons, no composers
  const d = { ...deps, scrollIntoCenter: () => {}, wait: async () => { t += 400; }, now: () => t, timeoutMs: 1600, pollMs: 400 };
  const r = await CB.discoverCommentSurface(a, d);
  assert.strictEqual(r.found, false);
  assert.strictEqual(
    CB.classifyGate1Failure({ articleFound: true, permalinkFound: true, commentButtonFound: false, composerEntryFound: false }),
    'comment_button_not_found',
  );
}

// 5. A composer on a NEIGHBOURING post (a different article) must not satisfy the gate — we
// only ever query the TARGET article, which has none.
{
  const target = makeArticle({ permalink: true }); // target has no entry
  const _neighbour = makeArticle({ composers: [composerEl()] }); // irrelevant — never queried
  void _neighbour;
  assert.strictEqual(CB.commentSurfaceState(target, deps).found, false);
}

// 7. Post never reached (no article / no permalink) → target_not_reached.
{
  assert.strictEqual(
    CB.classifyGate1Failure({ articleFound: false, permalinkFound: false, commentButtonFound: false, composerEntryFound: false }),
    'target_not_reached',
  );
}

// diagnostics surfaces the composer + textbox candidates + gate1_passed_via.
{
  const a = makeArticle({ permalink: true, composers: [composerEl()] });
  const d = CB.diagnostics(a, deps);
  assert.strictEqual(d.composer_entry_found, true);
  assert.strictEqual(d.gate1_passed_via, 'composer_entry');
  assert.ok(d.textbox_candidates_count >= 0);
}

console.log('comment_button.test.mjs OK');
