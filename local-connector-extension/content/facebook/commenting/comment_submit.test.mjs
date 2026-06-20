// Tests for the comment submit machinery: the deterministic submit-button TIERS
// (Fix A) and the settle gate that replaced the 150ms blind wait (Fix B).
//   Run: node content/comment_submit.test.mjs
import assert from 'node:assert';
import { createRequire } from 'node:module';

// findSubmitButtons falls back to `document` as a final scope (a browser global). Stub
// an empty document so the module runs under node — our candidates come from the editor.
globalThis.document = globalThis.document || { querySelectorAll: () => [] };

const require = createRequire(import.meta.url);
const S = require('./comment_submit.js');
const K = require('./comment_constants.js');
// Mirrors outbound.js norm: lowercase + strip diacritics (so 'Bình luận' → 'binh luan').
const norm = (v) => String(v == null ? '' : v)
  .normalize('NFD').replace(/[̀-ͯ]/g, '').replace(/[đĐ]/g, 'd').toLowerCase().trim();
const hasAny = (v, keys) => keys.some(k => norm(v).includes(k));

// Shared injected DOM predicates (the same shape outbound.js threads in via submitDeps).
const d = {
  norm, hasAny,
  labelOf: (el) => norm(el._label),
  visible: (el) => el._vis !== false,
  enabledButton: (el) => el._enabled !== false,
};

// mkBtn builds a fake button. Default geometry sits compact + just right of the editor
// (which spans top 100→130, left 0→300), so it qualifies as a spatial candidate.
function mkBtn({ label = '', x = 305, y = 101, w = 40, h = 28, enabled = true, vis = true } = {}) {
  return {
    _label: label, _enabled: enabled, _vis: vis,
    getAttribute: (n) => (n === 'aria-label' ? label : null),
    getBoundingClientRect: () => ({ top: y, bottom: y + h, left: x, right: x + w, height: h, width: w }),
    contains: () => false,
  };
}

// mkEditor wires a parent whose querySelectorAll answers the candidate set.
function mkEditor(candidatesFn) {
  return {
    getBoundingClientRect: () => ({ top: 100, bottom: 130, left: 0, right: 300, height: 30, width: 300 }),
    closest: () => null,
    parentElement: { querySelectorAll: () => candidatesFn(), parentElement: null },
    contains: () => false,
  };
}

// --- existing invariants (kept) -------------------------------------------------
assert.strictEqual(S._rejectActionLabel('avatar cua ban', { hasAny }), true);
assert.strictEqual(S._rejectActionLabel('nhan dan', { hasAny }), true);   // sticker
assert.strictEqual(S._rejectActionLabel('may anh', { hasAny }), true);    // camera
assert.strictEqual(S._rejectActionLabel('binh luan', { hasAny }), false); // the send button
assert.strictEqual(S._rejectActionLabel('gui', { hasAny }), false);

const editorGeo = { getBoundingClientRect: () => ({ top: 100, bottom: 130, left: 0, right: 300, height: 30, width: 300 }) };
const near = { getBoundingClientRect: () => ({ top: 100, bottom: 128, left: 305, right: 345, height: 28, width: 40 }) };
const far = { getBoundingClientRect: () => ({ top: 400, bottom: 460, left: 305, right: 600, height: 60, width: 295 }) };
assert.strictEqual(S._submitCandidateSpatial(editorGeo, near), true);
assert.strictEqual(S._submitCandidateSpatial(editorGeo, far), false);

// --- Fix A: STRICT deterministic tiers (no inverted heuristics) -----------------
// submitRank: labelled+enabled = 0 (best), labelled+disabled = 1, unlabelled = 2.
assert.strictEqual(S._submitRank(mkBtn({ label: 'Bình luận' }), d), 0, 'labelled+enabled is tier 0');
assert.strictEqual(S._submitRank(mkBtn({ label: 'Gửi', enabled: false }), d), 1, 'labelled+disabled is tier 1');
assert.strictEqual(S._submitRank(mkBtn({ label: '' }), d), 2, 'unlabelled (spatial-only) is tier 2');

// THE BUG THIS LOCKS OUT: a text-less icon sitting CLOSER to the editor must NOT
// outrank the labelled send button. The old submitScore rewarded the text-less icon
// (-20) and penalised the labelled button (+80), so fresh[0] was the wrong target on
// the first attempts. Tiers make the labelled button win regardless of proximity.
{
  const icon = mkBtn({ label: '', x: 301, y: 100, w: 24, h: 24 });        // closer, text-less toolbar icon
  const send = mkBtn({ label: 'Bình luận', x: 360, y: 101, w: 90, h: 28 }); // labelled send, slightly farther
  const ranked = S.findSubmitButtons(mkEditor(() => [icon, send]), [], d);
  assert.strictEqual(ranked[0], send, 'labelled send button must rank before a closer text-less icon');
  // And reversing the DOM order must not change the result — ordering is total.
  const ranked2 = S.findSubmitButtons(mkEditor(() => [send, icon]), [], d);
  assert.strictEqual(ranked2[0], send, 'ranking is independent of DOM order');
}

// A composer-toolbar icon (sticker) is rejected outright and never returned.
{
  const sticker = mkBtn({ label: 'Nhãn dán', x: 305, y: 101 });
  const send = mkBtn({ label: 'Gửi', x: 360, y: 101 });
  const ranked = S.findSubmitButtons(mkEditor(() => [sticker, send]), [], d);
  assert.ok(!ranked.includes(sticker), 'sticker/reject-label control is excluded');
  assert.strictEqual(ranked[0], send);
}

// --- Fix B: settle gate replaces the 150ms blind wait ---------------------------
// waitForStableSubmitTarget polls findSubmitButtons and returns only once the top
// candidate is the SAME element for stableMs continuous ms — past the pre-mount ghost.
{
  const ghost = mkBtn({ label: 'Bình luận', x: 305, y: 101 });
  const real = mkBtn({ label: 'Bình luận', x: 360, y: 101 });
  let clock = 0;
  let polls = 0;
  const now = () => clock;
  const wait = async (ms) => { clock += ms; polls += 1; };
  // First two polls expose the ghost generation; from the 3rd poll the real button mounts.
  const editor = mkEditor(() => (polls < 2 ? [ghost] : [real]));
  const settled = await S.waitForStableSubmitTarget(editor, [], d, {
    wait, now, timeoutMs: 5000, pollMs: 50, stableMs: 60,
  });
  assert.strictEqual(settled, real, 'settle gate returns the stable (post-mount) target, not the ghost');
}

// Timeout path: a target that never appears yields null (caller still degrades safely).
{
  let clock = 0;
  const settled = await S.waitForStableSubmitTarget(mkEditor(() => []), [], d, {
    wait: async (ms) => { clock += ms; }, now: () => clock, timeoutMs: 300, pollMs: 50, stableMs: 60,
  });
  assert.strictEqual(settled, null, 'no candidate ever → null, never a fabricated target');
}

// --- S6 lock-in: constants come from the single shared module -------------------
assert.strictEqual(typeof K.TIMING.maxSubmitAttempts, 'number');
assert.ok(K.SUBMIT_KEYS.includes('binh luan') && K.SUBMIT_KEYS.includes('gui'));

console.log('comment_submit: PASS');
