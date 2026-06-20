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

    // ---- Success path + gate-3 drift (cooperative DOM + stubbed comment siblings) ----
    // Protects the executeComment phase-helper extraction: drives the whole happy path
    // (gate-1 stable → button → gate-2 → editor acquire → gate-3 → SM submit) and the
    // gate-3 editor-drift abort. Fast because findTargetArticle resolves immediately.
    {
      const ID = '123456';
      const mk = (p) => Object.assign({
        getBoundingClientRect: () => ({ left: 10, top: 10, width: 50, height: 20 }),
        getAttribute: () => null, closest: () => null, querySelector: () => null,
        querySelectorAll: () => [], isConnected: true, disabled: false,
        scrollIntoView() {}, dispatchEvent() {}, click() {}, focus() {},
      }, p);
      function makeDom(editorArticleId) {
        const anchor = mk({ getAttribute: (n) => (n === 'href' ? '/groups/1/posts/' + ID + '/' : null) });
        const editAnchor = mk({ getAttribute: (n) => (n === 'href' ? '/groups/1/posts/' + editorArticleId + '/' : null) });
        const commentBtn = mk({ innerText: 'Comment', getAttribute: (n) => (n === 'role' ? 'button' : (n === 'aria-label' ? 'Comment' : null)) });
        const article = mk({ getAttribute: (n) => (n === 'role' ? 'article' : null) });
        const editorArticle = (editorArticleId === ID) ? article : mk({ getAttribute: (n) => (n === 'role' ? 'article' : null), querySelectorAll: (s) => (s.includes('/posts/') ? [editAnchor] : []) });
        const editor = mk({ isContentEditable: true, getAttribute: (n) => (n === 'role' ? 'textbox' : (n === 'aria-label' ? 'Write a comment' : null)) });
        const artQSA = (s) => { if (s.includes('role="button"')) return [commentBtn]; if (s.includes('/posts/')) return [anchor]; if (s.includes('contenteditable')) return [editor]; return []; };
        article.querySelectorAll = artQSA;
        article.querySelector = (s) => (s.includes('/posts/') ? anchor : null);
        editor.closest = () => editorArticle;
        commentBtn.closest = () => article;
        const doc = {
          cookie: '', title: '', documentElement: { innerHTML: '' }, body: { innerText: '' }, contains: () => true,
          createRange: () => ({ selectNodeContents() {} }), createElement: () => mk({}),
          querySelector: (s) => (s.includes('/posts/') ? anchor : null),
          querySelectorAll: (s) => { if (s.includes('role="article"')) return [article]; if (s.includes('role="button"')) return [commentBtn]; if (s.includes('/posts/')) return [anchor]; return []; },
        };
        return { doc, editor };
      }
      const stubs = {
        THGCommentComposer: { hostVerdict: () => 'target', CREATE_POST_KEYS: ['create a public post'], findComposerEntry: null },
        THGCommentButton: { commentSurfaceState: () => ({ found: true }), discoverCommentSurface: async () => ({ found: true }), diagnostics: () => ({ comment_button_found: true, composer_entry_found: true, gate1_passed_via: 'x', composer_candidates: [], textbox_candidates_count: 0, contenteditable_candidates_count: 0 }), classifyGate1Failure: () => 'target_not_reached' },
        THGCommentSM: { runComposerToSubmit: async () => ({ ok: true, diagnostic: { phase: 'submit' } }) },
      };

      // (a) success path
      {
        const dom = makeDom(ID);
        stubs.THGCommentComposer.findComposerEntry = () => ({ el: dom.editor, reason: 'ok', candidates: [] });
        const { api: a2, restore: r2 } = env.loadCommentingOutbound({ realModules: ['../content/comment_constants', '../content/proof', '../content/navreport'], singletons: stubs, document: dom.doc });
        try {
          const r = await a2.executeComment('hello world', 'https://www.facebook.com/groups/1/posts/' + ID + '/', 'exec-ok');
          assert.strictEqual(r.ok, true, 'success path: ok=true');
          assert.strictEqual(r.detail, 'sent_comment', 'success path: detail sent_comment');
          assert.ok(r.proof && r.proof.execution_id === 'exec-ok', 'success path: proof + execution_id echoed');
        } finally { r2(); }
      }

      // (b) gate-3 editor drift → context_drift (editor's article id != target, feed page)
      {
        const dom = makeDom('999999'); // editor belongs to a DIFFERENT post
        stubs.THGCommentComposer.findComposerEntry = () => ({ el: dom.editor, reason: 'ok', candidates: [] });
        const { api: a3, restore: r3 } = env.loadCommentingOutbound({ realModules: ['../content/comment_constants', '../content/proof', '../content/navreport'], singletons: stubs, document: dom.doc });
        try {
          const r = await a3.executeComment('hello world', 'https://www.facebook.com/groups/1/posts/' + ID + '/', 'exec-drift');
          assert.strictEqual(r.ok, false, 'gate-3: ok=false');
          assert.strictEqual(r.error, 'context_drift', 'gate-3 editor drift → context_drift');
        } finally { r3(); }
      }
      console.log('comment success-path + gate-3 characterization: PASS');

      // (c) executeCommentInFeed success path (findCommentEditor resolves the editor in feed)
      {
        const dom = makeDom(ID);
        stubs.THGCommentComposer.findComposerEntry = () => ({ el: dom.editor, reason: 'ok', candidates: [] });
        const { api: a4, restore: r4 } = env.loadCommentingOutbound({ realModules: ['../content/comment_constants', '../content/proof', '../content/navreport'], singletons: stubs, document: dom.doc });
        try {
          const r = await a4.executeCommentInFeed({ content: 'hello world', post_id: ID, execution_id: 'exec-feed' });
          assert.strictEqual(r.ok, true, 'feed success: ok=true');
          assert.strictEqual(r.detail, 'sent_comment', 'feed success: detail sent_comment');
        } finally { r4(); }
      }

      // (d) executeCommentInFeed with no post id → early comment_target_not_post_permalink
      {
        const { api: a5, restore: r5 } = env.loadCommentingOutbound({ realModules: ['../content/comment_constants', '../content/proof', '../content/navreport'], singletons: stubs });
        try {
          const r = await a5.executeCommentInFeed({ content: 'hi', execution_id: 'e' });
          assert.strictEqual(r.ok, false, 'feed no-post-id: ok=false');
          assert.strictEqual(r.error, 'comment_target_not_post_permalink', 'feed no-post-id error preserved');
          assert.strictEqual(r.proof.failure_reason, 'context_drift', 'feed no-post-id proof.failure_reason');
          assert.strictEqual(r.proof.execution_id, 'e', 'feed no-post-id execution_id echoed');
        } finally { r5(); }
      }
      console.log('comment-in-feed success + no-post-id characterization: PASS');
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
