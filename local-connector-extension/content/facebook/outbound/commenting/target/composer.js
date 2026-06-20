// THGCommentingTargetComposer — target-scoped composer acquisition, split verbatim from
// commenting_target.js (Workstream A · PR7): move-only, behavior-preserving. Consumes
// THGOutboundDom + comment constants + THGCommentingTargetPostId/Surface/Article; reads
// THGCommentComposer as a bare global at call time (preserved). Chrome:
// globalThis.THGCommentingTargetComposer (loaded after target/article.js); Node: module.exports.
globalThis.THGCommentingTargetComposer = globalThis.THGCommentingTargetComposer || (() => {
  const THGDom = globalThis.THGOutboundDom
    || (typeof require === 'function' ? require('../../dom/outbound_dom.js') : null);
  if (!THGDom) {
    throw new Error('THGOutboundDom is required before target/composer.js');
  }
  const { visible, labelOf, hasAny, norm } = THGDom;
  const K = globalThis.THGCommentConstants
    || (typeof require === 'undefined' ? null : require('../../../commenting/constants/comment_constants.js'));
  const THGPostId = globalThis.THGCommentingTargetPostId
    || (typeof require === 'function' ? require('./post_id.js') : null);
  if (!THGPostId) {
    throw new Error('THGCommentingTargetPostId is required before target/composer.js');
  }
  const { extractArticleCanonicalEntityId, onTargetPermalinkPage } = THGPostId;
  const THGSurface = globalThis.THGCommentingTargetSurface
    || (typeof require === 'function' ? require('./surface.js') : null);
  if (!THGSurface) {
    throw new Error('THGCommentingTargetSurface is required before target/composer.js');
  }
  const { findCommentEditor, isCreatePostComposer, commentSurfaceDeps } = THGSurface;
  const THGArticle = globalThis.THGCommentingTargetArticle
    || (typeof require === 'function' ? require('./article.js') : null);
  if (!THGArticle) {
    throw new Error('THGCommentingTargetArticle is required before target/composer.js');
  }
  const { findTargetArticle } = THGArticle;

  // findComposerForTarget locates the comment composer that BELONGS TO the target
  // post when it is not nested inside the post's [role=article] (permalink layout:
  // the "Write an answer…" box is a sibling / page-level element). It expands
  // outward from the target article and returns the first visible comment editor
  // that is EITHER inside the target post's article OR not inside ANY OTHER post's
  // article (a true sibling/page-level composer near the target). A composer that
  // sits inside a DIFFERENT post's article is SKIPPED — that is the wrong-post
  // editor Checkpoint-3 was correctly rejecting (gate3_editor_drift), the cause of
  // the observed context_drift on group permalink-feed pages. Returns null when
  // only foreign-post composers exist.
  // composerInScope returns the first acceptable comment composer within one ancestor scope,
  // or null. Skips search/message boxes + the create-post box; on FEED pages skips composers
  // belonging to a DIFFERENT post; accepts the target's own / page-level (sibling) composer.
  function composerInScope(scope, id, onPermalink) {
    const badKeys = ['search', 'tim kiem', 'message', 'messenger', 'nhan tin'];
    const commentKeys = K.COMMENT_KEYS;
    const editors = Array.from(scope.querySelectorAll('[contenteditable="true"], textarea, input[type="text"]'))
      .filter(el => visible(el) && !hasAny(labelOf(el), badKeys) && !isCreatePostComposer(el));
    for (const el of editors) {
      const art = el.closest('[role="article"], [role="dialog"]');
      const artId = art ? extractArticleCanonicalEntityId(art) : '';
      // Different-post composer skipped on FEED; on the target's own permalink page the URL
      // pins identity, so a foreign-id host is a nested comment/answer item near the target.
      if (artId && artId !== id && !onPermalink) continue;
      if (hasAny(labelOf(el), commentKeys) || norm(el.getAttribute('role')) === 'textbox' || !artId) {
        return el; // target's own, or a page/sibling-level composer near the target
      }
    }
    return null;
  }

  function findComposerForTarget(targetPostId) {
    if (!targetPostId) return null;
    const id = String(targetPostId);
    const targetArticle = findTargetArticle(id);
    if (!targetArticle) return null;
    const inArticle = findCommentEditor(targetArticle);
    if (inArticle) return inArticle;
    const onPermalink = onTargetPermalinkPage(id);
    let scope = targetArticle.parentElement;
    for (let depth = 0; scope && depth < 6; depth += 1, scope = scope.parentElement) {
      const found = composerInScope(scope, id, onPermalink);
      if (found) return found;
    }
    return null;
  }

  // acquireTargetComposer is the SINGLE SOURCE OF TRUTH for editor acquisition: it re-resolves
  // the composer with the EXACT classifier gate1 accepted with (THGCommentComposer.findComposerEntry
  // over the document-wide editable sweep in commentSurfaceDeps.docEditables), so a composer gate1
  // passed can never be lost by a divergent, narrower finder. The old failure was precisely that
  // divergence: gate1 swept document-wide while findComposerForTarget only walked 6 ancestor levels
  // of the target article — the permalink "Write an answer…" box lived outside that subtree.
  // Re-resolving here (fresh DOM query, same doctrine) also survives the async waits between gate1
  // and typing. Returns { el, reason, candidates } — candidates carry per-editor diagnostics.
  function acquireTargetComposer(targetPostId, scope) {
    const article = (targetPostId && findTargetArticle(targetPostId)) || scope || document;
    return THGCommentComposer.findComposerEntry(article, commentSurfaceDeps(targetPostId));
  }

  const api = { findComposerForTarget, acquireTargetComposer };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  return api;
})();
