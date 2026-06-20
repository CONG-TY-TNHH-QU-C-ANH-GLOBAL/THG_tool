// THGCommentingTarget — Facebook comment TARGET/identity/composer-discovery FACADE. After
// Workstream A · PR7 the helpers live in four responsibility sub-modules under target/
// (post_id → surface → article → composer); this file only AGGREGATES their APIs into the
// existing globalThis.THGCommentingTarget surface so downstream consumers (commenting_diag.js,
// the execute/ layer) keep the exact same contract. Move-only, behavior-preserving. Chrome:
// globalThis.THGCommentingTarget (loaded after target/*.js, before commenting_diag.js); Node:
// module.exports.
globalThis.THGCommentingTarget = globalThis.THGCommentingTarget || (() => {
  // Each sub-module is manifest-loaded before this facade; guarded fallback for Node tests.
  const THGPostId = globalThis.THGCommentingTargetPostId
    || (typeof require === 'function' ? require('./target/post_id.js') : null);
  if (!THGPostId) {
    throw new Error('THGCommentingTargetPostId is required before commenting_target.js');
  }
  const THGSurface = globalThis.THGCommentingTargetSurface
    || (typeof require === 'function' ? require('./target/surface.js') : null);
  if (!THGSurface) {
    throw new Error('THGCommentingTargetSurface is required before commenting_target.js');
  }
  const THGArticle = globalThis.THGCommentingTargetArticle
    || (typeof require === 'function' ? require('./target/article.js') : null);
  if (!THGArticle) {
    throw new Error('THGCommentingTargetArticle is required before commenting_target.js');
  }
  const THGComposer = globalThis.THGCommentingTargetComposer
    || (typeof require === 'function' ? require('./target/composer.js') : null);
  if (!THGComposer) {
    throw new Error('THGCommentingTargetComposer is required before commenting_target.js');
  }

  const api = { ...THGPostId, ...THGSurface, ...THGArticle, ...THGComposer };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  return api;
})();
