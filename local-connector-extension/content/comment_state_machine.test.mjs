// Comment executor state machine test (stage 2 bug fix). Stubs THGCommentGuard +
// THGCommentSubmit to prove the invariants: never submit a doubled/mismatched
// composer; submit is NOT called when the pre-submit assert fails; a composer that
// never clears is submit_not_accepted (NOT hidden_by_facebook).
//   Run: node content/comment_state_machine.test.mjs
import assert from 'node:assert';
import { createRequire } from 'node:module';

let submitFinds = 0;
globalThis.THGCommentGuard = {
  normalizeCommentText: t => String(t == null ? '' : t).replace(/\s+/g, ' ').trim(),
  readComposerText: e => e.text || '',
  prepareComposerForComment: async e => e._prep,
  assertComposerExactlyExpected: e => e._assert(),
  clearComposerUntilEmpty: async () => ({ ok: true }),
};
globalThis.THGCommentSubmit = {
  findSubmitButtons: () => { submitFinds++; return [{ id: 'btn' }]; },
  pressEnter: () => true,
};

const require = createRequire(import.meta.url);
const SM = require('./comment_state_machine.js');

const baseDeps = {
  executorPath: 'test',
  clickLikeUser: () => true,
  editorContainsContent: () => false, // composer cleared after submit
  waitFor: async pred => pred(),
  wait: async () => {},
  submitDeps: {},
};

// A — clean: prep ok, assert ok, composer clears → ok, submit attempted.
submitFinds = 0;
let r = await SM.runComposerToSubmit(
  { text: 'A', _prep: { ok: true, diagnostic: {} }, _assert: () => ({ ok: true, actual_length: 1 }) },
  'A', null, baseDeps);
assert.strictEqual(r.ok, true);
assert.ok(submitFinds > 0, 'submit attempted on a clean composer');
assert.strictEqual(r.diagnostic.phase, 'verify');

// A+A — prep reports doubled → abort, submit NEVER reached.
submitFinds = 0;
r = await SM.runComposerToSubmit(
  { text: 'AA', _prep: { ok: false, reason: 'comment_text_doubled', diagnostic: {} }, _assert: () => ({ ok: true }) },
  'A', null, baseDeps);
assert.strictEqual(r.ok, false);
assert.strictEqual(r.reason, 'comment_text_doubled');
assert.strictEqual(submitFinds, 0, 'must NOT look for / click submit when doubled');

// Mismatch at pre-submit assert → abort, submit NEVER reached (invariant #3).
submitFinds = 0;
r = await SM.runComposerToSubmit(
  { text: 'B', _prep: { ok: true, diagnostic: {} }, _assert: () => ({ ok: false, mismatch: true, actual_length: 1 }) },
  'A', null, baseDeps);
assert.strictEqual(r.ok, false);
assert.strictEqual(r.reason, 'comment_text_mismatch');
assert.strictEqual(submitFinds, 0, 'must NOT submit when pre-submit assert fails');

// Submit clicked but composer never clears → submit_not_accepted, NOT hidden_by_facebook (#4).
submitFinds = 0;
r = await SM.runComposerToSubmit(
  { text: 'A', _prep: { ok: true, diagnostic: {} }, _assert: () => ({ ok: true, actual_length: 1 }) },
  'A', null, { ...baseDeps, editorContainsContent: () => true });
assert.strictEqual(r.ok, false);
assert.strictEqual(r.reason, 'submit_not_accepted');
assert.notStrictEqual(r.reason, 'hidden_by_facebook');

console.log('comment_state_machine: PASS');
