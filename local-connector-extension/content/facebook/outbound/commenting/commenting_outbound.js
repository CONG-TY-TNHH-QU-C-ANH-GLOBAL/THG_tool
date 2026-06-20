// THGCommentingOutbound — comment EXECUTOR FACADE. After Workstream A · PR7 the executors live
// in responsibility sub-modules under execute/ (result → direct_gates → direct → feed_gates →
// feed → rung2); this file only AGGREGATES the four public entrypoints into the existing
// globalThis.THGCommentingOutbound surface so outbound.js keeps the exact same contract. Public
// runtime surface stays exactly the 4 comment methods; _test (commentResult / abbreviate /
// editorContainsContent) stays Node-only via module.exports. Move-only, behavior-preserving.
// Chrome: globalThis.THGCommentingOutbound (loaded after execute/*.js, before outbound.js); Node:
// module.exports (+ _test).
globalThis.THGCommentingOutbound = globalThis.THGCommentingOutbound || (() => {
  // Each execute sub-module is manifest-loaded before this facade; guarded fallback for Node tests.
  const THGResult = globalThis.THGCommentingResult
    || (typeof require === 'function' ? require('./execute/result.js') : null);
  if (!THGResult) {
    throw new Error('THGCommentingResult is required before commenting_outbound.js');
  }
  const THGDirect = globalThis.THGCommentingDirect
    || (typeof require === 'function' ? require('./execute/direct.js') : null);
  if (!THGDirect) {
    throw new Error('THGCommentingDirect is required before commenting_outbound.js');
  }
  const THGFeed = globalThis.THGCommentingFeed
    || (typeof require === 'function' ? require('./execute/feed.js') : null);
  if (!THGFeed) {
    throw new Error('THGCommentingFeed is required before commenting_outbound.js');
  }
  const THGRung2 = globalThis.THGCommentingRung2
    || (typeof require === 'function' ? require('./execute/rung2.js') : null);
  if (!THGRung2) {
    throw new Error('THGCommentingRung2 is required before commenting_outbound.js');
  }

  // Public 4-key runtime surface — identical to the pre-split contract; outbound.js / channels
  // read only these.
  const api = {
    executeComment: THGDirect.executeComment,
    executeCommentInFeed: THGFeed.executeCommentInFeed,
    executeCommentViaRung2: THGRung2.executeCommentViaRung2,
    probeRung2Click: THGRung2.probeRung2Click,
  };
  if (typeof module !== 'undefined' && module.exports) {
    module.exports = { ...api, _test: { commentResult: THGResult.commentResult, abbreviate: THGResult.abbreviate, editorContainsContent: THGResult.editorContainsContent } };
  }
  return api;
})();
