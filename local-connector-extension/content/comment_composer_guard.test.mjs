// Tests for the Comment Composer Guard.
//   Run from local-connector-extension/: node content/comment_composer_guard.test.mjs
import assert from 'node:assert';
import { createRequire } from 'node:module';

// Mock composer + DOM. execCommand('insertText') REPLACES the (selected-all) content,
// matching a Lexical editor once select-all actually covers the content.
let composerText = '';
const composer = {
  isContentEditable: true, tagName: 'DIV', className: 'composer',
  get innerText() { return composerText; },
  focus() {},
  dispatchEvent() { return true; },
};
globalThis.window = { getSelection: () => ({ removeAllRanges() {}, addRange() {} }) };
globalThis.document = {
  createRange: () => ({ selectNodeContents() {} }),
  execCommand(cmd, _ui, val) {
    if (cmd === 'selectAll') return true;
    if (cmd === 'delete') { composerText = ''; return true; }
    if (cmd === 'insertText') { composerText = (val || ''); return true; } // REPLACE (select-all covered it)
    return false;
  },
};
globalThis.KeyboardEvent = class { constructor(type) { this.type = type; } };
globalThis.InputEvent = class { constructor(type) { this.type = type; } };
globalThis.Event = class { constructor(type) { this.type = type; } };
globalThis.chrome = { runtime: { getManifest: () => ({ version: 'test' }) } };

const require = createRequire(import.meta.url);
const G = require('./comment_composer_guard.js');

const A = 'Bên THG Fulfill có hỗ trợ sourcing Tumbler 20oz. Nếu cần, inbox THG Fulfill nhé.';

// Pure logic
assert.strictEqual(G.normalizeCommentText('  a   b\n c '), 'a b c');
assert.strictEqual(G.isExactRepeatedText(A, A), false);
assert.strictEqual(G.isExactRepeatedText(A + A, A), true);

// Unicode robustness (the false-negative that made the old chain over-insert):
// an NFD-composed string and a zero-width-joined string must compare EQUAL to A.
const nfd = A.normalize('NFD');
assert.strictEqual(G.normalizeCommentText(nfd), G.normalizeCommentText(A), 'NFD must equal NFC');
const withZW = A.slice(0, 10) + '​' + A.slice(10);
assert.strictEqual(G.normalizeCommentText(withZW), G.normalizeCommentText(A), 'zero-width stripped');
assert.ok(G.assertComposerExactlyExpected({ innerText: nfd }, A).ok, 'NFD composer reads as exact match');

// Single-method insert REPLACES (no A+A+A compounding): a pre-existing value is
// replaced by expected → single, method kbd_selectall_insertText.
composerText = 'OLD LEFTOVER';
const res = await G.insertTextInto(composer, A);
assert.strictEqual(G.readComposerText(composer), G.normalizeCommentText(A), 'must end SINGLE, not compounded');
assert.strictEqual(res.method, 'kbd_selectall_insertText');

console.log('comment_composer_guard: PASS');
