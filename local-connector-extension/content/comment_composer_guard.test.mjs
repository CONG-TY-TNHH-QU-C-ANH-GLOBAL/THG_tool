// Pure + fallback-chain tests for the Comment Composer Guard.
//   Run from local-connector-extension/: node content/comment_composer_guard.test.mjs
import assert from 'node:assert';
import { createRequire } from 'node:module';

// --- Mock a Lexical-like composer: execCommand('insertText') APPENDS (the bug),
// execCommand('delete') is a no-op (Lexical ignores it), and paste REPLACES the
// selection (Lexical's onPaste). The verified fallback chain must still end single.
let composerText = '';
const composer = {
  isContentEditable: true,
  tagName: 'DIV',
  className: 'composer',
  get innerText() { return composerText; },
  focus() {},
  dispatchEvent(ev) {
    if (ev && ev.type === 'paste') composerText = ev.clipboardData.getData('text/plain');
    return true;
  },
};
globalThis.window = { getSelection: () => ({ removeAllRanges() {}, addRange() {} }) };
globalThis.document = {
  createRange: () => ({ selectNodeContents() {} }),
  execCommand(cmd, _ui, val) {
    if (cmd === 'selectAll') return true;
    if (cmd === 'delete') return true;            // no-op on text (Lexical-broken)
    if (cmd === 'insertText') { composerText += (val || ''); return true; } // APPENDS
    return false;
  },
};
globalThis.ClipboardEvent = class { constructor(type, init) { this.type = type; this.clipboardData = init.clipboardData; } };
globalThis.DataTransfer = class { constructor() { this._d = {}; } setData(k, v) { this._d[k] = v; } getData(k) { return this._d[k] || ''; } };
globalThis.InputEvent = class { constructor(type) { this.type = type; } };
globalThis.Event = class { constructor(type) { this.type = type; } };
globalThis.chrome = { runtime: { getManifest: () => ({ version: 'test' }) } };

const require = createRequire(import.meta.url);
const G = require('./comment_composer_guard.js');

const A = 'Bên THG Fulfill có hỗ trợ sourcing Tumbler 20oz. Nếu cần, inbox THG Fulfill nhé.';

// --- Pure logic ---
assert.strictEqual(G.normalizeCommentText('  a   b\n c '), 'a b c');
assert.strictEqual(G.isExactRepeatedText(A, A), false);
assert.strictEqual(G.isExactRepeatedText(A + A, A), true);
assert.ok(G.assertComposerExactlyExpected({ innerText: A }, A).ok);
assert.ok(G.assertComposerExactlyExpected({ innerText: A + A }, A).duplicate);

// --- Verified fallback chain: a pre-existing draft equal to A means execCommand
// insertText would APPEND → A+A; the chain must fall through to paste → single A.
composerText = A; // leftover draft
const res = await G.insertTextInto(composer, A);
assert.strictEqual(G.readComposerText(composer), G.normalizeCommentText(A), 'composer must end up SINGLE, not A+A');
assert.strictEqual(res.method, 'paste', 'execCommand appended (A+A) → paste replacement fixed it');

console.log('comment_composer_guard: PASS');
