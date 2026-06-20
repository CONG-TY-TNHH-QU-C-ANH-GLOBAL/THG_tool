// THGCommentingFeed — the group-feed comment EXECUTOR (executeCommentInFeed orchestrator +
// buildPreCtx), split verbatim from commenting_outbound.js (Workstream A · PR7): move-only,
// behavior-preserving. Each feed gate helper returns { fail } / a fail result and the parent
// propagates it immediately; feed identity gates 1/2/3 intact. Consumes THGOutboundDom + comment
// constants + THGCommentingResult + THGCommentingTarget + THGCommentingFeedGates; reads
// THGContentProof / THGCommentSM as bare globals at call time (preserved). Chrome:
// globalThis.THGCommentingFeed (loaded after execute/feed_gates.js, before execute/rung2.js);
// Node: module.exports.
globalThis.THGCommentingFeed = globalThis.THGCommentingFeed || (() => {
  const THGDom = globalThis.THGOutboundDom
    || (typeof require === 'function' ? require('../../dom/outbound_dom.js') : null);
  if (!THGDom) {
    throw new Error('THGOutboundDom is required before execute/feed.js');
  }
  const { wait, hasAny, visible, labelOf, clickLikeUser, waitFor, dismissBlockingOverlays } = THGDom;
  const K = globalThis.THGCommentConstants
    || (typeof require === 'undefined' ? null : require('../../../commenting/constants/comment_constants.js'));
  const THGResult = globalThis.THGCommentingResult
    || (typeof require === 'function' ? require('./result.js') : null);
  if (!THGResult) {
    throw new Error('THGCommentingResult is required before execute/feed.js');
  }
  const { commentResult, abbreviate, editorContainsContent, submitDeps } = THGResult;
  const THGTarget = globalThis.THGCommentingTarget
    || (typeof require === 'function' ? require('../commenting_target.js') : null);
  if (!THGTarget) {
    throw new Error('THGCommentingTarget is required before execute/feed.js');
  }
  const { extractPostIdFromUrl } = THGTarget;
  const THGFeedGates = globalThis.THGCommentingFeedGates
    || (typeof require === 'function' ? require('./feed_gates.js') : null);
  if (!THGFeedGates) {
    throw new Error('THGCommentingFeedGates is required before execute/feed.js');
  }
  const { feedScrollLocate, feedArticleNotFound, feedResolveEditor, feedCheckGate3 } = THGFeedGates;
  // Debug-gated swallow for best-effort browser calls (silent at normal runtime).
  function ignoreErr(e, ctx) { if (globalThis.__THG_COMMENTING_DEBUG__) console.debug(`[THGCommentingFeed] ${ctx}`, e); }

  // executeCommentInFeed is Path 2's content-script entry point. The
  // background-side outbox flow (src/outbox.js::executeInGroupFeed) has
  // already navigated the tab to /groups/<g>/ (group home — a surface
  // proven non-redirected for account #49) and verified the URL. Now we
  // find the target post's article element IN THE FEED DOM by post_id,
  // then comment from feed context. We NEVER load /groups/<g>/posts/<p>/.
  //
  // Why this exists: the permalink-page path (executeComment above) keeps
  // landing on `/` because FB silently redirects the tab during the small
  // window between navigateAndVerify success and content-script handler
  // entry. User confirmed account #49 has no FB-side restriction (manual
  // commenting works) — so the redirect is triggered by our deep-link
  // automation pattern. Group home + feed-DOM article-find bypasses the
  // entire permalink surface, sidestepping the redirect entirely.
  //
  // Reuses existing utilities: findTargetArticle, articleIsReadyForComment,
  // waitUntilTargetArticleStable (with extended timeout for scroll
  // discovery), commentButton/editor/submit finders, gates 2 + 3, and
  // commentResult. The only NEW behaviour is scroll-then-search to locate
  // articles that are below the initial fold in the group's feed.
  //
  // Diagnostic taxonomy (notes-field prefix → matches Path 2 spec):
  //   path2.article_not_found_in_feed             — gate-1 (feed-aware) timed out
  //   path2.article_found_but_comment_button_missing — gate-1 passed, no Comment btn in scope
  //   path2.article_found_comment_opened_submit_failed — typed OK, submit click did not clear editor
  //   path2.comment_success                       — terminal success
  // buildPreCtx snapshots the pre-submit proof context (FB uid + comment count + dup match),
  // shared by both comment executors. Reads THGContentProof at call time; '' / 0 / false when absent.
  function buildPreCtx(content, executionId) {
    const proof = THGContentProof || null;
    const fbUID = proof?.currentFBUserID() || '';
    const preCount = proof ? proof.snapshotCommentCount() : 0;
    const preMatched = proof ? !!proof.findCommentNode(content, fbUID) : false;
    return { content, userID: fbUID, preCount, duplicate: preMatched, executionId };
  }

  async function executeCommentInFeed(message) {
    const content = String(message?.content || '').trim();
    if (!content) return { ok: false, error: 'outbox_content_empty' };
    if (content.length > 3000) return { ok: false, error: 'outbox_content_too_long' };
    const targetUrl = String(message?.target_url || message?.targetUrl || '').trim();
    const executionId = String(message?.execution_id || message?.executionId || '').trim();
    const explicitPostId = String(message?.post_id || message?.postId || '').trim();
    const targetPostId = explicitPostId || extractPostIdFromUrl(targetUrl);
    if (!targetPostId) {
      return {
        ok: false,
        error: 'comment_target_not_post_permalink',
        proof: {
          success: false,
          failure_reason: 'context_drift',
          page_url_after: location.href || '',
          notes: 'path2.no_post_id: target_url=' + targetUrl,
          execution_id: executionId,
        },
      };
    }

    await dismissBlockingOverlays();
    const navAtEntry = location.href || '';
    console.log('[THG outbound.executeCommentInFeed] start',
      { target_url: targetUrl, target_post_id: targetPostId, landed_at_entry: navAtEntry, execution_id: executionId });

    const ctx = buildPreCtx(content, executionId);

    // GATE-1 (feed-aware): scroll-then-wait until the article materialises and is stable.
    const loc = await feedScrollLocate(targetPostId);
    if (!loc.targetScope) return feedArticleNotFound(targetPostId, loc.scrollsDone, navAtEntry, ctx);
    const targetScope = loc.targetScope;

    // Scroll target into view so FB doesn't unmount it as we click.
    try { targetScope.scrollIntoView({ block: 'center', behavior: 'instant' }); } catch (e) { ignoreErr(e, 'feed-scroll'); }
    await wait(400);

    // Find Comment button inside the article scope (NOT document-wide — feed has many articles).
    const commentKeys = K.COMMENT_KEYS;
    const buttons = Array.from(targetScope.querySelectorAll('div[role="button"], button, a[role="button"], span[role="button"]')).filter(el => visible(el));
    const commentButton = buttons.find(el => {
      const label = labelOf(el);
      return hasAny(label, commentKeys) && !label.includes('share') && !label.includes('like');
    });
    if (!commentButton) {
      const landed = location.href || '';
      console.warn('[THG outbound.executeCommentInFeed] article_found_but_comment_button_missing',
        { target_post_id: targetPostId, scanned_buttons: buttons.length, landed });
      return commentResult(false, 'comment_box_not_found', null, ctx,
        'path2.article_found_but_comment_button_missing: target id=' + abbreviate(targetPostId) +
        ' scanned_buttons=' + buttons.length +
        ' landed_at=' + landed +
        ' (article in feed but no Comment button matched label keys ' + JSON.stringify(commentKeys) + ')');
    }
    clickLikeUser(commentButton);
    await wait(900);

    const resolved = await feedResolveEditor(targetPostId, targetScope, ctx);
    if (resolved.fail) return resolved.fail;
    const editor = resolved.editor;

    const drift = feedCheckGate3(editor, targetPostId, ctx);
    if (drift) return drift;

    // Same composer→submit STATE MACHINE as executeComment — one shared path, so the
    // group-feed executor can never diverge or submit a doubled/mismatched composer.
    const sm = await THGCommentSM.runComposerToSubmit(editor, content, commentButton, {
      executorPath: 'group_feed',
      outboundId: Number(message?.id || message?.outbound_id || 0) || 0,
      clickLikeUser, editorContainsContent, waitFor, wait, now: () => Date.now(), submitDeps,
    });
    if (!sm.ok) {
      return commentResult(false, sm.reason, null, ctx,
        'path2.' + sm.reason + ': ' + JSON.stringify(sm.diagnostic) + ' · target id=' + abbreviate(targetPostId));
    }
    await wait(700);
    return commentResult(true, '', 'sent_comment', ctx,
      'path2.comment_success: target id=' + abbreviate(targetPostId) + ' · ' + JSON.stringify(sm.diagnostic));
  }

  const api = { executeCommentInFeed };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  return api;
})();
