// Comment executor entrypoint test (refactor-only). Proves the single entrypoint
// dispatches each comment message type to the right outbound executor.
//   Run: node content/comment_executor.test.mjs
import assert from 'node:assert';
import { createRequire } from 'node:module';

globalThis.THGContentOutbound = {
  executeOutbound: async () => ({ ok: true, via: 'outbound' }),
  executeCommentInFeed: async () => ({ ok: true, via: 'feed' }),
  executeCommentViaRung2: async () => ({ ok: true, via: 'rung2' }),
};

const require = createRequire(import.meta.url);
const E = require('./comment_executor.js');

assert.deepStrictEqual(await E.execute('thg_execute_outbound', {}), { ok: true, via: 'outbound' });
assert.deepStrictEqual(await E.execute('thg_comment_in_group_feed', {}), { ok: true, via: 'feed' });
assert.deepStrictEqual(await E.execute('thg_comment_via_rung2', {}), { ok: true, via: 'rung2' });

const bad = await E.execute('thg_unknown', {});
assert.strictEqual(bad.ok, false);
assert.ok(String(bad.error).includes('unsupported_comment_type'));

console.log('comment_executor: PASS');
