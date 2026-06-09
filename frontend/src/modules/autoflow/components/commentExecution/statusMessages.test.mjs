// Pure mapper test for comment execution status/reason (#3 + #8). Transpiles the
// self-contained statusMessages.ts with the installed tsc + asserts.
//   Run from frontend/: node src/modules/autoflow/components/commentExecution/statusMessages.test.mjs
import { readFileSync } from 'node:fs';
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const ts = require('typescript');
const src = readFileSync(new URL('./statusMessages.ts', import.meta.url), 'utf8');
const js = ts.transpileModule(src, { compilerOptions: { module: 'ES2020', target: 'ES2020' } }).outputText;
const { commentStatus, commentReason } = await import('data:text/javascript;base64,' + Buffer.from(js).toString('base64'));

// Lifecycle → friendly status. Success ONLY when verified.
assert.deepStrictEqual(commentStatus('queued', ''), { label: 'Đang chờ', severity: 'waiting' });
assert.deepStrictEqual(commentStatus('executing', ''), { label: 'Đang chạy', severity: 'running' });
assert.deepStrictEqual(commentStatus('finished', 'verified_success'), { label: 'Đã đăng thành công', severity: 'success' });
// Submitted but not verified is NOT success.
assert.strictEqual(commentStatus('finished', 'optimistic_success').severity, 'unverified');
assert.strictEqual(commentStatus('finished', 'optimistic_success').label, 'Đã gửi nhưng chưa xác minh');
assert.strictEqual(commentStatus('finished', 'context_drift').severity, 'failed');

// Reasons: plain Vietnamese, no raw code; success → ''.
assert.strictEqual(commentReason('verified_success'), '');
assert.strictEqual(commentReason('target_not_reached'), 'Không mở được đúng bài viết Facebook.');
assert.strictEqual(commentReason('context_drift'), 'Facebook chuyển trang trước khi gửi comment.');
assert.strictEqual(commentReason('actor_mismatch_blocked'), 'Đăng nhập nhầm Facebook.');
assert.strictEqual(commentReason('comment_quality_invalid'), 'Comment không đạt kiểm tra chất lượng.');
assert.ok(!commentReason('some_future_code').includes('_')); // friendly fallback, no raw code

console.log('Comment execution status/reason mapper: PASS');
