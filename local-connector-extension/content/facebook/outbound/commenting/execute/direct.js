// THGCommentingDirect — the permalink/direct comment EXECUTOR (executeComment orchestrator +
// locateTargetScope / acquireCommentEditor / submitCommentViaSM), split verbatim from
// commenting_outbound.js (Workstream A · PR7): move-only, behavior-preserving. executeComment
// stays a thin orchestrator: each early-stop-capable helper returns { fail } / a fail result
// and the parent immediately propagates it. The three identity checkpoints are intact. Consumes
// THGOutboundDom + THGCommentingResult + THGCommentingDiag + THGCommentingTarget +
// THGCommentingDirectGates; reads THGForensics / THGContentProof / THGCommentButton / THGCommentSM
// as bare globals at call time (preserved). Chrome: globalThis.THGCommentingDirect (loaded after
// execute/direct_gates.js, before execute/rung2.js); Node: module.exports.
globalThis.THGCommentingDirect = globalThis.THGCommentingDirect || (() => {
  const THGDom = globalThis.THGOutboundDom
    || (typeof require === 'function' ? require('../../dom/outbound_dom.js') : null);
  if (!THGDom) {
    throw new Error('THGOutboundDom is required before execute/direct.js');
  }
  const { wait, clickLikeUser, waitFor, dismissBlockingOverlays } = THGDom;
  const THGResult = globalThis.THGCommentingResult
    || (typeof require === 'function' ? require('./result.js') : null);
  if (!THGResult) {
    throw new Error('THGCommentingResult is required before execute/direct.js');
  }
  const { commentResult, abbreviate, editorContainsContent, submitDeps } = THGResult;
  const THGDiag = globalThis.THGCommentingDiag
    || (typeof require === 'function' ? require('../commenting_diag.js') : null);
  if (!THGDiag) {
    throw new Error('THGCommentingDiag is required before execute/direct.js');
  }
  const { probeCommentGates, navDiagFor } = THGDiag;
  const THGTarget = globalThis.THGCommentingTarget
    || (typeof require === 'function' ? require('../commenting_target.js') : null);
  if (!THGTarget) {
    throw new Error('THGCommentingTarget is required before execute/direct.js');
  }
  const { acquireTargetComposer, discoverDeps, extractArticleCanonicalEntityId, extractPostIdFromUrl,
    findCommentEditor, findComposerForTarget, findTargetArticle, onTargetPermalinkPage,
    waitUntilTargetArticleStable } = THGTarget;
  const THGGates = globalThis.THGCommentingDirectGates
    || (typeof require === 'function' ? require('./direct_gates.js') : null);
  if (!THGGates) {
    throw new Error('THGCommentingDirectGates is required before execute/direct.js');
  }
  const { gate1Failure, findCommentButtonIn, gate2ResolveScope, commentBoxNotFound, checkEditorGate3 } = THGGates;

  // locateTargetScope runs the gate-1 stability/fallback block (Checkpoint 1). Returns
  // { scope } on success or { fail } with the gate-1 failure result. targetPostId empty
  // (legacy callers) is a no-op returning { scope: null } so the document-wide search applies.
  async function locateTargetScope(targetPostId, permalinkPage, navAtEntry, targetUrl, ctx, ctxInfo) {
    if (!targetPostId) return { scope: null };
    let targetScope = await waitUntilTargetArticleStable(targetPostId, { timeoutMs: 8000, stableMs: 500, pollMs: 200 });
    if (!targetScope && permalinkPage && findComposerForTarget(targetPostId)) {
      // PERMALINK LAYOUT: comments pre-expanded, composer at page level; URL pins identity.
      targetScope = findTargetArticle(targetPostId) || document;
    }
    if (!targetScope) {
      // Gate1 fallback (PR-B): scroll the article into view + retry broadened discovery.
      const art = findTargetArticle(targetPostId);
      if (art) {
        const disc = await THGCommentButton.discoverCommentSurface(art, discoverDeps(targetPostId));
        if (disc.found) targetScope = art;
      }
    }
    if (!targetScope) return { fail: gate1Failure(targetPostId, navAtEntry, targetUrl, ctx, ctxInfo) };
    return { scope: targetScope };
  }

  // acquireCommentEditor — Checkpoint-2 scope + editor acquisition (with scroll retry / gate-2b
  // re-verify). Returns { editor, scope } or { fail }.
  async function acquireCommentEditor(targetPostId, permalinkPage, targetScope, commentButton, ctx, ctxInfo) {
    const g2 = gate2ResolveScope(targetPostId, targetScope, commentButton, ctx, ctxInfo);
    if (g2.fail) return { fail: g2.fail };
    let scope = g2.scope;
    let acq = acquireTargetComposer(targetPostId, scope);
    let editor = acq.el || findCommentEditor(scope) || (permalinkPage ? findComposerForTarget(targetPostId) : null);
    if (!editor) {
      globalThis.scrollBy({ top: 420, behavior: 'smooth' });
      await wait(900);
      if (targetPostId) {
        const refreshed = findTargetArticle(targetPostId);
        scope = refreshed || targetScope;
        if (!permalinkPage && extractArticleCanonicalEntityId(scope) !== targetPostId) {
          const landed = location.href || '';
          console.warn('[THG outbound.executeComment] gate2b FAIL scroll swap',
            { target_id: targetPostId, scope_id: extractArticleCanonicalEntityId(scope), landed_at_fail: landed });
          return { fail: commentResult(false, 'context_drift', null, ctx,
            'identity_gate_2b_scroll_swap: scope canonical id != ' + abbreviate(targetPostId) + ' landed_at=' + landed,
            navDiagFor('gate2b_scroll_swap', 'composer', probeCommentGates(targetPostId), ctxInfo)) };
        }
        acq = acquireTargetComposer(targetPostId, scope);
        editor = acq.el || findCommentEditor(scope) || (permalinkPage ? findComposerForTarget(targetPostId) : null);
      } else {
        editor = findCommentEditor(scope) || findCommentEditor(document);
      }
    }
    if (!editor) return { fail: commentBoxNotFound(targetPostId, acq, ctx, ctxInfo) };
    return { editor, scope };
  }

  // submitCommentViaSM — runs the shared composer->submit state machine, then builds the result.
  async function submitCommentViaSM(args) {
    const { editor, content, commentButton, permalinkPage, opts, ctx, targetPostId, ctxInfo } = args;
    const sm = await THGCommentSM.runComposerToSubmit(editor, content, commentButton, {
      executorPath: permalinkPage ? 'permalink_page' : 'permalink_article',
      outboundId: opts.outboundId || 0,
      clickLikeUser, editorContainsContent, waitFor, wait, now: () => Date.now(), submitDeps,
    });
    if (!sm.ok) {
      return commentResult(false, sm.reason, null, ctx,
        'sm.' + sm.reason + ': ' + JSON.stringify(sm.diagnostic) + ' · target id=' + abbreviate(targetPostId || '<none>'),
        navDiagFor(sm.reason, sm.diagnostic.phase || 'submit', probeCommentGates(targetPostId), ctxInfo));
    }
    // Submit accepted (composer cleared). Settle so the proof verifier sees the new node.
    await wait(700);
    return commentResult(true, '', 'sent_comment', ctx, 'sm.sent: ' + JSON.stringify(sm.diagnostic),
      navDiagFor('post_submit', 'verify', probeCommentGates(targetPostId), ctxInfo));
  }

  // executeComment orchestrates the three identity checkpoints + submit. Each phase helper is
  // behavior-preserving (verbatim logic) and aborts via commentResult; identity gates intact.
  async function executeComment(content, targetUrl = '', executionId = '', opts = {}) {
    if (globalThis.THGForensics) THGForensics.install();
    await dismissBlockingOverlays();
    const navAtEntry = location.href || '';
    console.log('[THG outbound.executeComment] start',
      { target_url: targetUrl, landed_at_entry: navAtEntry, execution_id: executionId });
    const proof = THGContentProof || null;
    const fbUID = proof?.currentFBUserID() || '';
    const preCount = proof ? proof.snapshotCommentCount() : 0;
    const preMatched = proof ? !!proof.findCommentNode(content, fbUID) : false;
    const ctx = { content, userID: fbUID, preCount, duplicate: preMatched, executionId };
    const targetPostId = extractPostIdFromUrl(targetUrl);
    const ctxInfo = {
      targetUrl, targetPostId, fbUID,
      accountId: Number(opts.accountId || 0) || 0,
      navTrace: opts.navTrace || null,
    };
    const permalinkPage = onTargetPermalinkPage(targetPostId);

    const located = await locateTargetScope(targetPostId, permalinkPage, navAtEntry, targetUrl, ctx, ctxInfo);
    if (located.fail) return located.fail;
    const targetScope = located.scope;

    const searchRoot = targetScope || document;
    const commentButton = findCommentButtonIn(searchRoot);
    if (commentButton) {
      clickLikeUser(commentButton);
      await wait(900);
    }

    const acquired = await acquireCommentEditor(targetPostId, permalinkPage, targetScope, commentButton, ctx, ctxInfo);
    if (acquired.fail) return acquired.fail;
    const editor = acquired.editor;

    const drift = checkEditorGate3(editor, targetPostId, permalinkPage, ctx, ctxInfo);
    if (drift) return drift;

    return submitCommentViaSM({ editor, content, commentButton, permalinkPage, opts, ctx, targetPostId, ctxInfo });
  }

  const api = { executeComment };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  return api;
})();
