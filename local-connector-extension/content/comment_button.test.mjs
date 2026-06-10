// Gate1 comment-button discovery tests (PR-B). Proves: discovery finds the button or an
// already-mounted composer; a delayed (lazily-mounted) action row is found after
// scroll+retry; a button that never appears yields comment_button_not_found; a post we
// never reached yields target_not_reached.
//   Run: node content/comment_button.test.mjs
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const CB = require('./comment_button.js');

const visible = () => true;
const labelOf = (el) => String((el && el._label) || '').toLowerCase(); // mirrors outbound.js (lowercased)
const baseDeps = { visible, labelOf, findCommentEditor: (a) => a && a._editor };

function btn(label) {
  return { _label: label, getAttribute: () => null };
}
function staticArticle(els, editor) {
  return { querySelector: () => null, querySelectorAll: () => els, _editor: editor || null };
}

// 1. A visible Comment/Bình luận button → found via button.
{
  const st = CB.commentSurfaceState(staticArticle([btn('Bình luận')]), baseDeps);
  assert.strictEqual(st.found, true);
  assert.strictEqual(st.via, 'button');
}

// 2. No comment button but an already-mounted composer (permalink layout) → found via composer.
{
  const st = CB.commentSurfaceState(staticArticle([btn('Thích'), btn('Chia sẻ')], { id: 'editor' }), baseDeps);
  assert.strictEqual(st.found, true);
  assert.strictEqual(st.via, 'composer');
}

// 3. Delayed (lazily-mounted) button: appears once we've scrolled/waited → eventually pass.
{
  let t = 0;
  const a = {
    _editor: null,
    querySelector: () => null,
    querySelectorAll: () => (t >= 600 ? [btn('Comment')] : []), // mounts after ~2 polls
  };
  const deps = { ...baseDeps, scrollIntoCenter: () => {}, wait: async () => { t += 300; }, now: () => t, timeoutMs: 4000, pollMs: 300 };
  const r = await CB.discoverCommentSurface(a, deps);
  assert.strictEqual(r.found, true, 'delayed button should be found after scroll+retry');
  assert.ok(r.scrolledAttempts >= 1, 'should have scrolled at least once');
}

// 4. Button missing forever → not found → comment_button_not_found (reached post, no button).
{
  let t = 0;
  const a = { _editor: null, querySelector: () => null, querySelectorAll: () => [btn('Thích')] };
  const deps = { ...baseDeps, scrollIntoCenter: () => {}, wait: async () => { t += 300; }, now: () => t, timeoutMs: 1200, pollMs: 300 };
  const r = await CB.discoverCommentSurface(a, deps);
  assert.strictEqual(r.found, false);
  assert.strictEqual(
    CB.classifyGate1Failure({ articleFound: true, permalinkFound: true, commentButtonFound: false }),
    'comment_button_not_found',
  );
}

// 5. Post never reached (no article) → target_not_reached.
{
  assert.strictEqual(
    CB.classifyGate1Failure({ articleFound: false, permalinkFound: false, commentButtonFound: false }),
    'target_not_reached',
  );
}

console.log('comment_button.test.mjs OK');
