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
  // Settle gate (Fix B). Stubbed to a no-op resolver: the loop below still RE-QUERIES
  // findSubmitButtons each attempt, which is the behaviour these tests exercise.
  waitForStableSubmitTarget: async () => null,
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
  now: () => 0,
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
assert.strictEqual(r.diagnostic.submit_target_settled, false, 'settle-gate result recorded in diagnostic');

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

// Ghost-button → real-button: the FIRST submit-button generation is a no-op (click
// does not clear the composer); a LATER generation works. Success here is only possible
// because the state machine RE-QUERIES findSubmitButtons each attempt instead of reusing
// the stale pre-flush node. Proves the atomic-readiness fix (ghost/stale-button gap).
{
  let gen = 0;
  let composerFull = true;
  globalThis.THGCommentSubmit = {
    findSubmitButtons: () => { gen += 1; return [{ id: 'btn', gen }]; },
    // Settle gate stubbed to no-op so it does not consume a generation — the loop's
    // RE-QUERY across attempts is what must surface the working button generation.
    waitForStableSubmitTarget: async () => null,
    pressEnter: () => true,
  };
  const deps = {
    executorPath: 'test',
    // Only the 2nd+ generation button is wired — clicking gen 1 is a ghost no-op.
    clickLikeUser: (b) => { if (b && b.gen >= 2) composerFull = false; return true; },
    editorContainsContent: () => composerFull,
    waitFor: async (pred) => pred(),
    wait: async () => {},
    now: () => 0,
    submitDeps: {},
  };
  const rr = await SM.runComposerToSubmit(
    { text: 'A', _prep: { ok: true, diagnostic: {} }, _assert: () => ({ ok: true, actual_length: 1 }) },
    'A', null, deps);
  assert.strictEqual(rr.ok, true, 'submit succeeds once a fresh generation is re-queried');
  assert.ok(gen >= 2, 'submit button must be RE-QUERIED across attempts (not reused)');
  assert.ok(rr.diagnostic.submit_requeried_attempts >= 2, 'diagnostic records re-query attempts');
}

// Restore the default submit stub for any later cases.
globalThis.THGCommentSubmit = {
  findSubmitButtons: () => { submitFinds++; return [{ id: 'btn' }]; },
  waitForStableSubmitTarget: async () => null,
  pressEnter: () => true,
};

console.log('comment_state_machine: PASS');

