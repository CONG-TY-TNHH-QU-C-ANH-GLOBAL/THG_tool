// PR4 characterization — Inbox layer (executeInbox / inboxResult) result + proof shape.
//   Run: node local-connector-extension/test/inbox_outbound.test.js
//   CI:  node --test (auto-discovered)
//
// Locks the EXACT inbox return/proof contract BEFORE the extraction and re-runs unchanged
// AFTER it. Goes through the facade (THGContentOutbound.executeOutbound) so it is agnostic to
// whether executeInbox lives in outbound.js (pre-PR4) or inbox_outbound.js (post-PR4).
//
// Sequential by construction (single load; no concurrency).
const assert = require('node:assert');
const { loadOutboundWithGlobals, loadInboxOutbound } = require('./outbound_test_env');

// The inbox proof = buildInboxProof()'s emptyProof() fields + the execution_id echo — the
// same canonical 12-key set the backend ExtensionExecutionReport consumes. No thread_id /
// recipient keys exist in the inbox path; they must not be added.
const EXPECTED_PROOF_KEYS = [
  'bubble_fresh', 'comment_permalink', 'count_increased', 'dom_snippet', 'duplicate',
  'execution_id', 'failure_reason', 'message_bubble_id', 'node_matched', 'notes',
  'page_url_after', 'success',
];

(async () => {
  const { O, restore } = loadOutboundWithGlobals({ realModules: ['../content/proof'] });
  try {
    // type 'inbox' routes to the inbox path. With no DOM no composer/message-button is found,
    // so inboxResult builds the failure proof — its exact key set is the contract.
    const r = await O.executeOutbound({ type: 'inbox', content: 'hello there', execution_id: 'exec-inbox-1' });
    assert.strictEqual(r.ok, false, 'inbox: ok=false');
    assert.strictEqual(r.error, 'message_button_not_found', 'inbox: first failure code preserved');
    assert.ok(r.proof && typeof r.proof === 'object', 'inbox: has proof');

    assert.deepStrictEqual(
      Object.keys(r.proof).sort((a, b) => a.localeCompare(b)), EXPECTED_PROOF_KEYS,
      'inbox proof key set is exactly the historical canonical set');

    for (const k of Object.keys(r.proof)) {
      assert.ok(/^[a-z0-9]+(_[a-z0-9]+)*$/.test(k), 'snake_case proof key: ' + k);
    }
    for (const cc of ['executionId', 'pageUrlAfter', 'failureReason', 'messageBubbleId']) {
      assert.ok(!(cc in r.proof), 'no camelCase key ' + cc);
    }
    // Keys that have NEVER been in the inbox proof must not be introduced.
    for (const absent of ['thread_id', 'recipient', 'message_request', 'cannot_receive_message', 'type', 'content', 'target_url']) {
      assert.ok(!(absent in r.proof), 'inbox proof must not contain ' + absent);
    }

    assert.strictEqual(r.proof.execution_id, 'exec-inbox-1', 'execution_id echoed (snake_case)');
    assert.strictEqual(r.proof.success, false, 'failure proof success=false');
    assert.strictEqual(typeof r.proof.failure_reason, 'string', 'failure_reason is a string');

    // Top-level failure shape: exactly { ok, error, proof }.
    assert.deepStrictEqual(Object.keys(r).sort((a, b) => a.localeCompare(b)), ['error', 'ok', 'proof'],
      'inbox failure top-level shape is exactly { ok, error, proof }');

    console.log('inbox proof characterization (facade): PASS');
  } finally {
    restore();
  }

  // --- Direct THGInboxOutbound module (post-PR4 extraction) --------------------------
  if (typeof loadInboxOutbound === 'function') {
    const { INBOX, api, restore: restore2 } = loadInboxOutbound({ realModules: ['../content/proof'] });
    try {
      assert.strictEqual(typeof api.executeInbox, 'function', 'THGInboxOutbound.executeInbox present');
      assert.ok(!('_test' in api), 'runtime THGInboxOutbound must not expose _test');
      assert.ok(INBOX._test && typeof INBOX._test.inboxResult === 'function', 'module.exports._test.inboxResult (Node-only)');

      const r = await api.executeInbox('hello there', 'exec-direct');
      assert.strictEqual(r.ok, false);
      assert.strictEqual(r.error, 'message_button_not_found', 'direct executeInbox first failure code');
      assert.deepStrictEqual(
        Object.keys(r.proof).sort((a, b) => a.localeCompare(b)), EXPECTED_PROOF_KEYS,
        'direct executeInbox proof key set exact');
      assert.strictEqual(r.proof.execution_id, 'exec-direct', 'direct executeInbox echoes execution_id');

      // inboxResult with the proof builder absent → no proof, top-level shape preserved.
      delete globalThis.THGContentProof;
      const noProof = INBOX._test.inboxResult(true, '', 'sent_inbox_button', { content: 'x', preBubbleHash: '', executionId: 'e' });
      assert.deepStrictEqual(noProof, { ok: true, detail: 'sent_inbox_button' }, 'absent builder success → { ok, detail }');
      const noProofErr = INBOX._test.inboxResult(false, 'inbox_failed', null, { content: 'x', preBubbleHash: '', executionId: 'e' });
      assert.deepStrictEqual(noProofErr, { ok: false, error: 'inbox_failed' }, 'absent builder failure → { ok, error }');

      console.log('inbox direct-module characterization: PASS');
    } finally {
      restore2();
    }
  }
})().catch((e) => { console.error(e); process.exit(1); });
