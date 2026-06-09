// Pure tests for the Comment Composer Guard (incident PR-1). The DOM helpers need a
// browser; here we test the pure equality/duplicate logic with mock composers.
//   Run: node content/comment_composer_guard.test.mjs
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const G = require('./comment_composer_guard.js');

const A = 'Bên THG Fulfill có hỗ trợ sourcing Tumbler 20oz. Nếu cần, inbox THG Fulfill nhé.';

// normalizeCommentText
assert.strictEqual(G.normalizeCommentText('  a   b\n c '), 'a b c');
assert.strictEqual(G.normalizeCommentText(null), '');

// isExactRepeatedText
assert.strictEqual(G.isExactRepeatedText(A, A), false);          // single is not repeated
assert.strictEqual(G.isExactRepeatedText(A + A, A), true);       // A+A concatenated
assert.strictEqual(G.isExactRepeatedText(A + ' ' + A, A), true); // A + space + A
assert.strictEqual(G.isExactRepeatedText(A + '\n' + A, A), true);// A + newline + A (normalized)
assert.strictEqual(G.isExactRepeatedText(A + ' tail', A), false);// A + different tail is not a clean repeat
assert.strictEqual(G.isExactRepeatedText('short', 'short'), false); // too short to judge

// assertComposerExactlyExpected (mock composer = { innerText })
const okCheck = G.assertComposerExactlyExpected({ innerText: A }, A);
assert.ok(okCheck.ok && !okCheck.duplicate && !okCheck.mismatch);

const dupCheck = G.assertComposerExactlyExpected({ innerText: A + A }, A);
assert.ok(!dupCheck.ok && dupCheck.duplicate && !dupCheck.mismatch);

const misCheck = G.assertComposerExactlyExpected({ innerText: A + ' khác hoàn toàn nội dung B' }, A);
assert.ok(!misCheck.ok && misCheck.mismatch && !misCheck.duplicate);

// whitespace-only difference must still be ok (normalized equality)
assert.ok(G.assertComposerExactlyExpected({ innerText: '  ' + A + '  ' }, A).ok);

console.log('comment_composer_guard: PASS');
