// PR3 characterization — Posting layer (executePost / postResult) result + proof shape.
//   Run: node local-connector-extension/test/posting_outbound.test.js
//   CI:  node --test (auto-discovered)
//
// Locks the EXACT posting return/proof contract BEFORE the extraction and re-runs unchanged
// AFTER it. Goes through the facade (THGContentOutbound.executeOutbound) so it is agnostic to
// whether executePost lives in outbound.js (pre-PR3) or posting_outbound.js (post-PR3).
//
// Sequential by construction (single load; no concurrency).
const assert = require('node:assert');
const { loadOutboundWithGlobals, loadPostingOutbound } = require('./outbound_test_env');

// The posting proof = buildPostProof()'s emptyProof() fields + the execution_id echo.
// 12 keys, snake_case where multi-word. This set is the backend ExtensionExecutionReport
// contract and must not drift.
const EXPECTED_PROOF_KEYS = [
  'bubble_fresh', 'comment_permalink', 'count_increased', 'dom_snippet', 'duplicate',
  'execution_id', 'failure_reason', 'message_bubble_id', 'node_matched', 'notes',
  'page_url_after', 'success',
];

(async () => {
  const { O, restore } = loadOutboundWithGlobals({ realModules: ['../content/proof'] });
  try {
    // group_post + profile_post both route to the posting path. With no DOM the composer is
    // not found, so postResult builds the failure proof — its exact key set is the contract.
    for (const type of ['group_post', 'profile_post']) {
      const r = await O.executeOutbound({ type, content: 'hello world', execution_id: 'exec-123' });
      assert.strictEqual(r.ok, false, type + ': ok=false');
      assert.strictEqual(r.error, 'post_composer_not_found', type + ': routes to executePost');
      assert.ok(r.proof && typeof r.proof === 'object', type + ': has proof');

      // Exact key set (sorted via localeCompare — no bare .sort()).
      assert.deepStrictEqual(
        Object.keys(r.proof).sort((a, b) => a.localeCompare(b)), EXPECTED_PROOF_KEYS,
        type + ': posting proof key set is exactly the historical set');

      // snake_case guard: every key stays snake_case; no camelCase variants leak in.
      for (const k of Object.keys(r.proof)) {
        assert.ok(/^[a-z0-9]+(_[a-z0-9]+)*$/.test(k), 'snake_case proof key: ' + k);
      }
      for (const cc of ['executionId', 'targetUrl', 'accountId', 'failureReason', 'pageUrlAfter']) {
        assert.ok(!(cc in r.proof), 'no camelCase key ' + cc);
      }
      // Illustrative-but-absent keys must NOT be added to the posting proof.
      for (const absent of ['type', 'content', 'target_url', 'account_id']) {
        assert.ok(!(absent in r.proof), 'posting proof must not contain ' + absent);
      }

      // Field-level checks (not just truthiness).
      assert.strictEqual(r.proof.execution_id, 'exec-123', type + ': execution_id echoed (snake_case)');
      assert.strictEqual(r.proof.success, false, type + ': failure proof success=false');
      assert.strictEqual(typeof r.proof.failure_reason, 'string', type + ': failure_reason is a string');
    }

    // Top-level failure shape: exactly { ok, error, proof } (no extra keys).
    const f = await O.executeOutbound({ type: 'group_post', content: 'x', execution_id: 'e' });
    assert.deepStrictEqual(Object.keys(f).sort((a, b) => a.localeCompare(b)), ['error', 'ok', 'proof'],
      'failure top-level shape is exactly { ok, error, proof }');

    console.log('posting proof characterization (facade): PASS');
  } finally {
    restore();
  }

  // --- Direct THGPostingOutbound module (post-PR3 extraction) ------------------------
  {
    const { POST, api, restore: restore2 } = loadPostingOutbound({ realModules: ['../content/proof'] });
    try {
      // Runtime surface: executePost present; _test (postResult) Node-only, not on the global.
      assert.strictEqual(typeof api.executePost, 'function', 'THGPostingOutbound.executePost present');
      assert.ok(!('_test' in api), 'runtime THGPostingOutbound must not expose _test');
      assert.ok(POST._test && typeof POST._test.postResult === 'function', 'module.exports._test.postResult (Node-only)');

      // Direct executePost yields the same failure + proof contract as via the facade.
      const r = await POST.executePost('hello world', 'exec-direct');
      assert.strictEqual(r.ok, false);
      assert.strictEqual(r.error, 'post_composer_not_found', 'direct executePost composer-not-found');
      assert.deepStrictEqual(
        Object.keys(r.proof).sort((a, b) => a.localeCompare(b)), EXPECTED_PROOF_KEYS,
        'direct executePost proof key set is exactly the historical set');
      assert.strictEqual(r.proof.execution_id, 'exec-direct', 'direct executePost echoes execution_id');

      // postResult with the proof builder absent → no proof, top-level shape preserved.
      delete globalThis.THGContentProof;
      const noProof = POST._test.postResult(true, '', 'sent_post_button', { content: 'x', executionId: 'e' });
      assert.deepStrictEqual(noProof, { ok: true, detail: 'sent_post_button' }, 'absent proof builder → { ok, detail } only');
      const noProofErr = POST._test.postResult(false, 'post_failed', null, { content: 'x', executionId: 'e' });
      assert.deepStrictEqual(noProofErr, { ok: false, error: 'post_failed' }, 'absent proof builder failure → { ok, error } only');

      console.log('posting direct-module characterization: PASS');
    } finally {
      restore2();
    }
  }
})().catch((e) => { console.error(e); process.exit(1); });
