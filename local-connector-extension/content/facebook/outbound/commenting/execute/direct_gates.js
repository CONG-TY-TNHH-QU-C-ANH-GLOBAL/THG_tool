// THGCommentingDirectGates — direct-path identity CHECKPOINT + failure-diagnostic builders
// (gate1Failure / findCommentButtonIn / gate2ResolveScope / commentBoxNotFound / checkEditorGate3),
// split verbatim from commenting_outbound.js (Workstream A · PR7): move-only, behavior-preserving.
// Each builder returns a typed outcome / commentResult so executeComment propagates the early stop;
// identity gates 1/2/3 intact. Consumes THGOutboundDom + comment constants + THGCommentingResult +
// THGCommentingDiag + THGCommentingTarget; reads THGCommentButton as a bare global at call time
// (preserved). Chrome: globalThis.THGCommentingDirectGates (loaded after execute/result.js, before
// execute/direct.js); Node: module.exports.
globalThis.THGCommentingDirectGates = globalThis.THGCommentingDirectGates || (() => {
  const THGDom = globalThis.THGOutboundDom
    || (typeof require === 'function' ? require('../../dom/outbound_dom.js') : null);
  if (!THGDom) {
    throw new Error('THGOutboundDom is required before execute/direct_gates.js');
  }
  const { visible, labelOf, hasAny } = THGDom;
  const K = globalThis.THGCommentConstants
    || (typeof require === 'undefined' ? null : require('../../../commenting/constants/comment_constants.js'));
  const THGResult = globalThis.THGCommentingResult
    || (typeof require === 'function' ? require('./result.js') : null);
  if (!THGResult) {
    throw new Error('THGCommentingResult is required before execute/direct_gates.js');
  }
  const { commentResult, abbreviate } = THGResult;
  const THGDiag = globalThis.THGCommentingDiag
    || (typeof require === 'function' ? require('../commenting_diag.js') : null);
  if (!THGDiag) {
    throw new Error('THGCommentingDiag is required before execute/direct_gates.js');
  }
  const { probeCommentGates, navDiagFor } = THGDiag;
  const THGTarget = globalThis.THGCommentingTarget
    || (typeof require === 'function' ? require('../commenting_target.js') : null);
  if (!THGTarget) {
    throw new Error('THGCommentingTarget is required before execute/direct_gates.js');
  }
  const { commentSurfaceDeps, extractArticleCanonicalEntityId, findComposerForTarget,
    findTargetArticle, isCreatePostComposer, onTargetPermalinkPage } = THGTarget;

  // gate1Failure builds the PR8A landing-gate-1 failure diagnostic + commentResult. Pure
  // diagnostic/abort (no DOM mutation): probes the post + composer and classifies the miss.
  function gate1Failure(targetPostId, navAtEntry, targetUrl, ctx, ctxInfo) {
    const landed = location.href || '';
    const probe = probeCommentGates(targetPostId);
    const art = findTargetArticle(targetPostId);
    const ent = art ? THGCommentButton.diagnostics(art, commentSurfaceDeps(targetPostId)) : { comment_button_found: false, composer_entry_found: false, textbox_candidates_count: 0, contenteditable_candidates_count: 0, gate1_passed_via: 'none', composer_candidates: [] };
    const candReasons = (ent.composer_candidates || []).map((cand) => cand.reason + (cand.accepted ? '(ok)' : '')).join(',');
    const gates = {
      articleFound: probe.articleFound, permalinkFound: probe.permalinkFound,
      commentButtonFound: ent.comment_button_found, composerEntryFound: ent.composer_entry_found,
    };
    const reason = THGCommentButton.classifyGate1Failure(gates);
    const navDiag = navDiagFor('gate1_' + reason, 'gate1', gates, ctxInfo);
    const rc = navDiag?.redirect_class || 'unknown';
    console.warn('[THG outbound.executeComment] gate1 FAIL ' + reason,
      { target_url: targetUrl, target_id: targetPostId, landed_at_fail: landed,
        nav_at_entry: navAtEntry, redirect_class: rc, gates,
        gate1_passed_via: ent.gate1_passed_via,
        composer_candidates: ent.composer_candidates,
        textbox_candidates_count: ent.textbox_candidates_count,
        contenteditable_candidates_count: ent.contenteditable_candidates_count,
        entry: ent });
    return commentResult(false, reason, null, ctx,
      'identity_gate_1_' + reason + ': target id=' + abbreviate(targetPostId) +
      ' redirect_class=' + rc +
      ' article_found=' + gates.articleFound +
      ' permalink_found=' + gates.permalinkFound +
      ' comment_button_found=' + gates.commentButtonFound +
      ' composer_entry_found=' + gates.composerEntryFound +
      ' textbox_candidates=' + ent.textbox_candidates_count +
      ' contenteditable_candidates=' + ent.contenteditable_candidates_count +
      ' gate1_passed_via=' + ent.gate1_passed_via +
      ' composer_candidate_reasons=[' + candReasons + ']' +
      ' landed_at=' + landed + ' nav_at_entry=' + navAtEntry +
      ' did not settle (article+permalink+comment-entry) within fallback window',
      navDiag);
  }

  // findCommentButtonIn returns the visible Comment/Bình luận button in scope, or undefined.
  function findCommentButtonIn(searchRoot) {
    const commentKeys = K.COMMENT_KEYS;
    const buttons = Array.from(searchRoot.querySelectorAll('div[role="button"], button, a[role="button"], span[role="button"]')).filter(el => visible(el));
    return buttons.find(el => {
      const label = labelOf(el);
      return hasAny(label, commentKeys) && !label.includes('share') && !label.includes('like');
    });
  }

  // gate2ResolveScope — Checkpoint 2 post-click identity. Returns { scope } or { fail }.
  function gate2ResolveScope(targetPostId, targetScope, commentButton, ctx, ctxInfo) {
    if (!targetPostId) {
      return { scope: commentButton?.closest('[role="article"], [role="dialog"]') || document };
    }
    const refreshed = findTargetArticle(targetPostId);
    const scope = refreshed || targetScope;
    if (extractArticleCanonicalEntityId(scope) !== targetPostId) {
      const landed = location.href || '';
      console.warn('[THG outbound.executeComment] gate2 FAIL post-click swap',
        { target_id: targetPostId, scope_id: extractArticleCanonicalEntityId(scope), landed_at_fail: landed });
      return { fail: commentResult(false, 'context_drift', null, ctx,
        'identity_gate_2_post_click_swap: scope canonical id != ' + abbreviate(targetPostId) + ' landed_at=' + landed,
        navDiagFor('gate2_post_click_swap', 'composer', probeCommentGates(targetPostId), ctxInfo)) };
    }
    return { scope };
  }

  // commentBoxNotFound — P0b diagnostic + result when no editor materialised.
  function commentBoxNotFound(targetPostId, acq, ctx, ctxInfo) {
    const landed = location.href || '';
    const artD = targetPostId ? findTargetArticle(targetPostId) : null;
    const entD = artD ? THGCommentButton.diagnostics(artD, commentSurfaceDeps(targetPostId)) : null;
    let cands = [];
    if (acq?.candidates?.length) cands = acq.candidates;
    else if (entD?.composer_candidates) cands = entD.composer_candidates;
    const candReasons = cands.map((c) => c.reason + (c.accepted ? '(ok)' : '')).join(',');
    const fcft = !!findComposerForTarget(targetPostId);
    console.warn('[THG outbound.executeComment] comment_box_not_found',
      { target_id: targetPostId, landed_at_fail: landed,
        urlPinsIdentity: onTargetPermalinkPage(targetPostId),
        gate1_passed_via: entD ? entD.gate1_passed_via : 'unknown',
        findComposerForTarget_found: fcft,
        editor_candidates_count: cands.length, editor_candidates: cands,
        acquire_reason: acq ? acq.reason : 'none' });
    return commentResult(false, 'comment_box_not_found', null, ctx,
      'comment_box_not_found: target id=' + abbreviate(targetPostId || '<none>') +
      ' urlPinsIdentity=' + onTargetPermalinkPage(targetPostId) +
      ' gate1_passed_via=' + (entD ? entD.gate1_passed_via : 'unknown') +
      ' acquire_reason=' + (acq ? acq.reason : 'none') +
      ' findComposerForTarget_found=' + fcft +
      ' editor_candidates=' + cands.length +
      ' editor_candidate_reasons=[' + candReasons + ']' +
      ' landed_at=' + landed,
      navDiagFor('comment_box_not_found', 'composer', probeCommentGates(targetPostId), ctxInfo));
  }

  // checkEditorGate3 — Checkpoint 3 pre-type editor-drift guard. Returns a fail result or null.
  function checkEditorGate3(editor, targetPostId, permalinkPage, ctx, ctxInfo) {
    if (!targetPostId) return null;
    const editorArticle = editor.closest('[role="article"], [role="dialog"]');
    const editorScopeID = editorArticle ? extractArticleCanonicalEntityId(editorArticle) : '';
    const inTargetArticle = editorScopeID === targetPostId;
    // On the target's OWN permalink page the URL pins identity, so a foreign-id enclosing
    // article is a nested comment/answer item — accept unless it is the create-post box.
    const ok = inTargetArticle || (permalinkPage && !isCreatePostComposer(editor));
    if (ok) return null;
    const landed = location.href || '';
    console.warn('[THG outbound.executeComment] gate3 FAIL editor drift',
      { target_id: targetPostId, editor_scope_id: editorScopeID || '<no-enclosing-article>', permalink_page: permalinkPage, landed_at_fail: landed });
    return commentResult(false, 'context_drift', null, ctx,
      'identity_gate_3_editor_drift: editor closest container canonical id=' + (editorScopeID || '<none>') + ' != ' + abbreviate(targetPostId) + ' landed_at=' + landed,
      navDiagFor('gate3_editor_drift', 'typing', probeCommentGates(targetPostId), ctxInfo));
  }

  const api = { gate1Failure, findCommentButtonIn, gate2ResolveScope, commentBoxNotFound, checkEditorGate3 };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  return api;
})();
