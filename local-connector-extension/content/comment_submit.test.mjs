// Pure-ish tests for the extracted comment submit machinery (refactor-only move).
//   Run: node content/comment_submit.test.mjs
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const S = require('./comment_submit.js');
const hasAny = (v, keys) => keys.some(k => String(v).includes(k));

// rejectActionLabel: composer-toolbar icons are rejected; the real send labels are NOT.
assert.strictEqual(S._rejectActionLabel('avatar cua ban', { hasAny }), true);
assert.strictEqual(S._rejectActionLabel('nhan dan', { hasAny }), true);   // sticker
assert.strictEqual(S._rejectActionLabel('may anh', { hasAny }), true);    // camera
assert.strictEqual(S._rejectActionLabel('binh luan', { hasAny }), false); // the send button
assert.strictEqual(S._rejectActionLabel('gui', { hasAny }), false);

// submitCandidateSpatial: a compact button just right/below the editor is a candidate;
// a far, oversized one is not.
const editor = { getBoundingClientRect: () => ({ top: 100, bottom: 130, left: 0, right: 300, height: 30, width: 300 }) };
const near = { getBoundingClientRect: () => ({ top: 100, bottom: 128, left: 305, right: 345, height: 28, width: 40 }) };
const far = { getBoundingClientRect: () => ({ top: 400, bottom: 460, left: 305, right: 600, height: 60, width: 295 }) };
assert.strictEqual(S._submitCandidateSpatial(editor, near), true);
assert.strictEqual(S._submitCandidateSpatial(editor, far), false);

console.log('comment_submit: PASS');
