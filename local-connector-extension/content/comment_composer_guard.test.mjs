// Tests for the Comment Composer Guard (incident invariants).
//   Run from local-connector-extension/: node content/comment_composer_guard.test.mjs
import assert from 'node:assert';
import { createRequire } from 'node:module';

// Fake clock so the guard's settle/stability waits run instantly.
let now = 1_000_000;
Date.now = () => now;
globalThis.setTimeout = (fn, ms) => { now += (ms || 0); queueMicrotask(fn); return 0; };

// Mock composer + DOM. `mode` controls how the (mocked) Lexical reacts:
//   replace  → insertText REPLACES (select-all covered it), delete clears.
//   doubled  → a SINGLE insertText yields A+A (the over-insert we must abort on).
//   clearfail→ delete is a no-op (Lexical content cannot be cleared).
let composerText = '';
let insertCount = 0;
let mode = 'replace';
const composer = {
  isContentEditable: true, tagName: 'DIV', className: 'composer',
  get innerText() { return composerText; }, focus() {}, dispatchEvent() { return true; },
};
globalThis.window = { getSelection: () => ({ removeAllRanges() {}, addRange() {} }) };
globalThis.document = {
  createRange: () => ({ selectNodeContents() {} }),
  execCommand(cmd, _ui, val) {
    if (cmd === 'selectAll') return true;
    if (cmd === 'delete') { if (mode !== 'clearfail') composerText = ''; return true; }
    if (cmd === 'insertText') { insertCount += 1; composerText = (mode === 'doubled') ? (val + val) : (val || ''); return true; }
    return false;
  },
};
globalThis.KeyboardEvent = class { constructor(t) { this.type = t; } };
globalThis.InputEvent = class { constructor(t) { this.type = t; } };
globalThis.Event = class { constructor(t) { this.type = t; } };
globalThis.chrome = { runtime: { getManifest: () => ({ version: 'test' }) } };

const require = createRequire(import.meta.url);
const G = require('./comment_composer_guard.js');
const A = 'Bên THG Fulfill có hỗ trợ sourcing Tumbler 20oz. Nếu cần, inbox THG Fulfill nhé.';

// 1. Pure + Unicode robustness (the false-negative that caused over-insertion).
assert.strictEqual(G.normalizeCommentText('  a   b\n c '), 'a b c');
assert.strictEqual(G.normalizeCommentText(A.normalize('NFD')), G.normalizeCommentText(A), 'NFD == NFC');
assert.strictEqual(G.normalizeCommentText(A.slice(0, 10) + '​' + A.slice(10)), G.normalizeCommentText(A), 'zero-width stripped');
assert.ok(G.assertComposerExactlyExpected({ innerText: A.normalize('NFD') }, A).ok, 'NFD composer is an exact match');
assert.strictEqual(G.isExactRepeatedText(A + A, A), true);

// 2. A correct single insert → ok, EXACTLY one insert, no A+A.
mode = 'replace'; composerText = 'OLD'; insertCount = 0;
let r = await G.prepareComposerForComment(composer, A, { outboundId: 1, executorPath: 'test' });
assert.strictEqual(r.ok, true, 'replace insert → ok');
assert.strictEqual(insertCount, 1, 'exactly one insert (no cumulative fallback)');
assert.strictEqual(G.readComposerText(composer), G.normalizeCommentText(A));

// 3. A doubled insert → abort comment_text_doubled, NO second insert, no submit.
mode = 'doubled'; composerText = ''; insertCount = 0;
r = await G.prepareComposerForComment(composer, A, { outboundId: 2, executorPath: 'test' });
assert.strictEqual(r.ok, false);
assert.strictEqual(r.reason, 'comment_text_doubled', 'doubled insert → comment_text_doubled');
assert.strictEqual(insertCount, 1, 'aborts after ONE insert — never a second');

// 4. Unclearable composer → composer_clear_failed, NEVER inserts.
mode = 'clearfail'; composerText = 'STUCK DRAFT THAT WONT CLEAR'; insertCount = 0;
r = await G.prepareComposerForComment(composer, A, { outboundId: 3, executorPath: 'test' });
assert.strictEqual(r.ok, false);
assert.strictEqual(r.reason, 'composer_clear_failed', 'unclearable → composer_clear_failed');
assert.strictEqual(insertCount, 0, 'never insert into a non-clearable composer');

console.log('comment_composer_guard: PASS');
