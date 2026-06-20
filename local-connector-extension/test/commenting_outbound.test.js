// PR5 characterization — Commenting layer (executeComment + result + nav_diagnostic).
//   Run: node local-connector-extension/test/commenting_outbound.test.js
//   CI:  node --test (auto-discovered)
//
// Locks the EXACT comment return/proof/nav_diagnostic contract BEFORE the extraction and
// re-runs unchanged AFTER it. The facade section goes through THGContentOutbound.executeOutbound
// so it is agnostic to whether executeComment lives in outbound.js (pre-PR5) or
// commenting_outbound.js (post-PR5). The direct + diag-independence sections only run once the
// PR5 modules/loaders exist.
//
// Sequential by construction (single load; no concurrency).
const assert = require('node:assert');
const fs = require('node:fs');
const path = require('node:path');
const env = require('./outbound_test_env');

const REAL = ['../content/comment_composer', '../content/comment_constants', '../content/proof', '../content/navreport'];

// Comment proof = buildCommentProof()'s emptyProof() fields + execution_id echo + the
// comment-only nav_diagnostic. 13 keys, snake_case where multi-word.
const EXPECTED_COMMENT_PROOF_KEYS = [
  'bubble_fresh', 'comment_permalink', 'count_increased', 'dom_snippet', 'duplicate',
  'execution_id', 'failure_reason', 'message_bubble_id', 'nav_diagnostic', 'node_matched',
  'notes', 'page_url_after', 'success',
];

(async () => {
  // ---- Facade characterization (works pre- and post-extraction) ---------------------
  {
    const { O, restore } = env.loadOutboundWithGlobals({ realModules: REAL });
    try {
      // No DOM + no post id in target_url → executeComment skips the identity block and
      // reaches comment_box_not_found fast (no 8s stability wait). Locks the proof shape.
      const r = await O.executeOutbound({ type: 'comment', content: 'hello world', execution_id: 'exec-c1' });
      assert.strictEqual(r.ok, false, 'comment: ok=false');
      assert.strictEqual(r.error, 'comment_box_not_found', 'comment routes to executeComment');
      assert.ok(r.proof && typeof r.proof === 'object', 'comment: has proof');

      assert.deepStrictEqual(
        Object.keys(r.proof).sort((a, b) => a.localeCompare(b)), EXPECTED_COMMENT_PROOF_KEYS,
        'comment proof key set is exactly the historical set (incl. nav_diagnostic)');

      for (const k of Object.keys(r.proof)) {
        assert.ok(/^[a-z0-9]+(_[a-z0-9]+)*$/.test(k), 'snake_case proof key: ' + k);
      }
      for (const cc of ['executionId', 'pageUrlAfter', 'failureReason', 'navDiagnostic']) {
        assert.ok(!(cc in r.proof), 'no camelCase key ' + cc);
      }
      assert.strictEqual(r.proof.execution_id, 'exec-c1', 'execution_id echoed (snake_case)');
      assert.strictEqual(r.proof.success, false, 'failure proof success=false');

      // nav_diagnostic is a plain snake_case object (shape owned by THGNavReport, unchanged).
      const nd = r.proof.nav_diagnostic;
      assert.ok(nd && typeof nd === 'object' && !Array.isArray(nd), 'nav_diagnostic is a plain object');
      for (const k of Object.keys(nd)) {
        assert.ok(/^[a-z0-9]+(_[a-z0-9]+)*$/.test(k), 'nav_diagnostic snake_case key: ' + k);
      }
      assert.ok('redirect_class' in nd && 'stage' in nd, 'nav_diagnostic carries redirect_class + stage');

      // Top-level failure shape: exactly { ok, error, proof }.
      assert.deepStrictEqual(Object.keys(r).sort((a, b) => a.localeCompare(b)), ['error', 'ok', 'proof'],
        'comment failure top-level shape is exactly { ok, error, proof }');

      // Content guards still apply on the comment route.
      assert.deepStrictEqual(await O.executeOutbound({ type: 'comment', content: '' }),
        { ok: false, error: 'outbox_content_empty' }, 'empty content rejected pre-dispatch');

      console.log('comment proof characterization (facade): PASS');
    } finally {
      restore();
    }
  }

  // ---- Direct module + diagnostics independence (post-PR5 only) ---------------------
  if (typeof env.loadCommentingOutbound === 'function') {
    const { CMT, api, restore } = env.loadCommentingOutbound({ realModules: REAL });
    try {
      assert.deepStrictEqual(
        Object.keys(api).sort((a, b) => a.localeCompare(b)),
        ['executeComment', 'executeCommentInFeed', 'executeCommentViaRung2', 'probeRung2Click'],
        'THGCommentingOutbound runtime exposes the 4 comment entrypoints');
      assert.ok(!('_test' in api), 'runtime THGCommentingOutbound must not expose _test');
      assert.ok(CMT._test && typeof CMT._test.commentResult === 'function', 'module.exports._test.commentResult (Node-only)');
      assert.strictEqual(typeof CMT._test.abbreviate, 'function', '_test.abbreviate present');
      assert.strictEqual(typeof CMT._test.editorContainsContent, 'function', '_test.editorContainsContent present');

      const r = await api.executeComment('hello world', '', 'exec-direct');
      assert.strictEqual(r.error, 'comment_box_not_found', 'direct executeComment comment_box_not_found');
      assert.deepStrictEqual(
        Object.keys(r.proof).sort((a, b) => a.localeCompare(b)), EXPECTED_COMMENT_PROOF_KEYS,
        'direct executeComment proof key set exact');
      assert.strictEqual(r.proof.execution_id, 'exec-direct', 'direct executeComment echoes execution_id');

      console.log('comment direct-module characterization: PASS');
    } finally {
      restore();
    }

    // ---- Diagnostics independence scans (trap 6/7) ----------------------------------
    const diagSrc = fs.readFileSync(path.join(__dirname, '..', 'content', 'commenting_diag.js'), 'utf8');
    assert.ok(!diagSrc.includes('THGCommentingOutbound'), 'commenting_diag.js must not reference THGCommentingOutbound');
    assert.ok(!diagSrc.includes("require('./commenting_outbound.js')"), 'commenting_diag.js must not require commenting_outbound.js');
    assert.ok(!diagSrc.includes('executeComment'), 'commenting_diag.js must not call the executor');
    // navDiagFor receives only plain snapshot context (no DOM nodes / ctx / executorState).
    assert.ok(!/navDiagFor\([^)]*\b(executorState|executorContext|article|editor|button)\b/.test(diagSrc),
      'navDiagFor must not receive executor state / DOM nodes');
    console.log('comment diagnostics-independence scan: PASS');
  }
})().catch((e) => { console.error(e); process.exit(1); });
