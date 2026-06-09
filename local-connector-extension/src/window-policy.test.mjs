// Window Respect policy defaults (PR-2). Run: node src/window-policy.test.mjs
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const policy = require('./window-policy.js');

// SAFE defaults — automation must not touch the user's window by default.
assert.strictEqual(policy.shouldCloseTabAfterExecution(), false, 'must NOT close tab by default');
assert.strictEqual(policy.shouldMinimizeAfterExecution(), false, 'must NOT minimize by default');
assert.strictEqual(policy.shouldManageWindowSize(), false, 'must NOT manage window size by default');

// focusUpdate focuses WITHOUT resizing by default (no state:'normal' → a maximized
// window is not snapped to half-screen).
assert.deepStrictEqual(policy.focusUpdate(), { focused: true });

// When window management is explicitly enabled (debug), focusUpdate may force normal.
policy._policy.manageWindowSize = true;
assert.deepStrictEqual(policy.focusUpdate(), { state: 'normal', focused: true });
policy._policy.manageWindowSize = false;

console.log('window-policy: PASS');
