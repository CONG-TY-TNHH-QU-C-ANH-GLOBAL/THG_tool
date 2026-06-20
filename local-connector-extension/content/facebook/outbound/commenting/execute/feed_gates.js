// THGCommentingFeedGates — feed-path GATE helpers (feedScrollLocate / feedArticleNotFound /
// feedGate2Scope / feedResolveEditor / feedCheckGate3), split verbatim from commenting_outbound.js
// (Workstream A · PR7): move-only, behavior-preserving. Each early-stop helper returns { fail } /
// a fail result so executeCommentInFeed propagates it immediately; feed identity gates 1/2/3 intact.
// Consumes THGOutboundDom + THGCommentingResult + THGCommentingTarget. Chrome:
// globalThis.THGCommentingFeedGates (loaded after execute/direct.js, before execute/feed.js);
// Node: module.exports.
globalThis.THGCommentingFeedGates = globalThis.THGCommentingFeedGates || (() => {
  const THGDom = globalThis.THGOutboundDom
    || (typeof require === 'function' ? require('../../dom/outbound_dom.js') : null);
  if (!THGDom) {
    throw new Error('THGOutboundDom is required before execute/feed_gates.js');
  }
  const { wait } = THGDom;
  const THGResult = globalThis.THGCommentingResult
    || (typeof require === 'function' ? require('./result.js') : null);
  if (!THGResult) {
    throw new Error('THGCommentingResult is required before execute/feed_gates.js');
  }
  const { commentResult, abbreviate } = THGResult;
  const THGTarget = globalThis.THGCommentingTarget
    || (typeof require === 'function' ? require('../commenting_target.js') : null);
  if (!THGTarget) {
    throw new Error('THGCommentingTarget is required before execute/feed_gates.js');
  }
  const { extractArticleCanonicalEntityId, findCommentEditor, findTargetArticle, waitUntilTargetArticleStable } = THGTarget;

  const FEED_MAX_SCROLLS = 8;
  const FEED_PER_ATTEMPT_TIMEOUT_MS = 2500;

  // feedScrollLocate is the feed-aware GATE-1: alternate short stability attempts with scroll
  // pulses until the target article materialises (or scrolls exhausted). Returns { targetScope, scrollsDone }.
  async function feedScrollLocate(targetPostId) {
    let targetScope = null;
    let scrollsDone = 0;
    for (let i = 0; i <= FEED_MAX_SCROLLS; i++) {
      targetScope = await waitUntilTargetArticleStable(targetPostId, {
        timeoutMs: FEED_PER_ATTEMPT_TIMEOUT_MS,
        stableMs: 500,
        pollMs: 200,
      });
      if (targetScope) break;
      if (i === FEED_MAX_SCROLLS) break;
      try {
        globalThis.scrollBy({ top: 1800, behavior: 'instant' });
      } catch {
        globalThis.scrollTo(0, globalThis.scrollY + 1800);
      }
      scrollsDone++;
      await wait(700);
    }
    return { targetScope, scrollsDone };
  }

  function feedArticleNotFound(targetPostId, scrollsDone, navAtEntry, ctx) {
    const landed = location.href || '';
    const articlesSeen = document.querySelectorAll('[role="article"]').length;
    console.warn('[THG outbound.executeCommentInFeed] article_not_found_in_feed',
      { target_post_id: targetPostId, scrolls: scrollsDone, articles_in_dom: articlesSeen, landed });
    return commentResult(false, 'context_drift', null, ctx,
      'path2.article_not_found_in_feed: target id=' + abbreviate(targetPostId) +
      ' scrolls=' + scrollsDone +
      ' articles_in_dom=' + articlesSeen +
      ' nav_at_entry=' + navAtEntry +
      ' landed_at=' + landed +
      ' (article+permalink+comment-button stable 500ms never observed across ' +
      (FEED_MAX_SCROLLS + 1) + ' attempts of ' + FEED_PER_ATTEMPT_TIMEOUT_MS + 'ms each)');
  }

  // feedGate2Scope — feed Checkpoint 2 post-click identity. Returns { scope } or { fail }.
  function feedGate2Scope(targetPostId, targetScope, ctx) {
    const refreshed = findTargetArticle(targetPostId);
    const scope = refreshed || targetScope;
    if (extractArticleCanonicalEntityId(scope) !== targetPostId) {
      const landed = location.href || '';
      console.warn('[THG outbound.executeCommentInFeed] gate2_swap',
        { target_post_id: targetPostId, scope_id: extractArticleCanonicalEntityId(scope), landed });
      return { fail: commentResult(false, 'context_drift', null, ctx,
        'path2.identity_gate_2_post_click_swap: scope canonical id != ' + abbreviate(targetPostId) +
        ' landed_at=' + landed) };
    }
    return { scope };
  }

  // feedResolveEditor — feed editor acquisition (gate-2 + scroll retry / gate-2b). Returns
  // { editor } or { fail }.
  async function feedResolveEditor(targetPostId, targetScope, ctx) {
    const g2 = feedGate2Scope(targetPostId, targetScope, ctx);
    if (g2.fail) return { fail: g2.fail };
    let scope = g2.scope;
    let editor = findCommentEditor(scope);
    if (!editor) {
      // Scroll a touch + re-find. Same recovery executeComment uses.
      globalThis.scrollBy({ top: 420, behavior: 'smooth' });
      await wait(900);
      const refreshed2 = findTargetArticle(targetPostId);
      scope = refreshed2 || targetScope;
      if (extractArticleCanonicalEntityId(scope) !== targetPostId) {
        const landed = location.href || '';
        return { fail: commentResult(false, 'context_drift', null, ctx,
          'path2.identity_gate_2b_scroll_swap: scope canonical id != ' + abbreviate(targetPostId) +
          ' landed_at=' + landed) };
      }
      editor = findCommentEditor(scope);
    }
    if (!editor) {
      const landed = location.href || '';
      console.warn('[THG outbound.executeCommentInFeed] comment_box_not_found_post_click',
        { target_post_id: targetPostId, landed });
      return { fail: commentResult(false, 'comment_box_not_found', null, ctx,
        'path2.article_found_but_comment_button_missing: target id=' + abbreviate(targetPostId) +
        ' landed_at=' + landed +
        ' (Comment button clicked but no editable composer materialized in scope)') };
    }
    return { editor };
  }

  // feedCheckGate3 — feed Checkpoint 3 pre-type editor-drift guard. Returns a fail result or null.
  function feedCheckGate3(editor, targetPostId, ctx) {
    const editorArticle = editor.closest('[role="article"], [role="dialog"]');
    if (!editorArticle || extractArticleCanonicalEntityId(editorArticle) !== targetPostId) {
      const landed = location.href || '';
      const editorScopeID = editorArticle ? extractArticleCanonicalEntityId(editorArticle) : '<no-enclosing-article>';
      console.warn('[THG outbound.executeCommentInFeed] gate3_editor_drift',
        { target_post_id: targetPostId, editor_scope_id: editorScopeID, landed });
      return commentResult(false, 'context_drift', null, ctx,
        'path2.identity_gate_3_editor_drift: editor closest container canonical id != ' + abbreviate(targetPostId) +
        ' landed_at=' + landed);
    }
    return null;
  }

  const api = { feedScrollLocate, feedArticleNotFound, feedResolveEditor, feedCheckGate3 };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  return api;
})();
