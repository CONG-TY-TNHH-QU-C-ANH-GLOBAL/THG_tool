// Execution idempotency test (incident root fix). Proves a resend of an in-flight or
// recently-completed execution_id is suppressed — which is what stops the at-least-
// once resend from double-inserting the comment.
//   Run: node content/execution_dedup.test.mjs
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const D = require('./execution_dedup.js');

let now = 1_000_000;

// Fresh id is not a duplicate.
assert.strictEqual(D.isDuplicate('exec-1', now), false);

// While ACTIVE, the same id is a duplicate (a resend mid-flight is rejected).
D.markActive('exec-1');
assert.strictEqual(D.isDuplicate('exec-1', now), true);
assert.strictEqual(D.isDuplicate('exec-2', now), false); // a different id still runs

// After completion, a resend within the window is still a duplicate.
D.markDone('exec-1', now);
assert.strictEqual(D.isDuplicate('exec-1', now + 5_000), true, 'resend right after completion suppressed');

// Past the window, the id is allowed again (a genuinely new, much-later command).
assert.strictEqual(D.isDuplicate('exec-1', now + 120_000), false);

// Empty execution_id is never treated as a duplicate (legacy rows fall through).
assert.strictEqual(D.isDuplicate('', now), false);

console.log('execution_dedup: PASS');
