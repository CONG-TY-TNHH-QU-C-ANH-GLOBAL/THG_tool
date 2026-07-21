// Comment executor entrypoint (Browser Automation Kit — comment executor extraction).
// The SINGLE entrypoint for every comment delivery path. The heavy orchestration
// bodies (executeOutbound/executeComment/executeCommentInFeed/executeCommentViaRung2)
// still live in outbound.js for now; this gives the bridge ONE comment entrypoint to
// dispatch to, so the next bug-fix PR can move the bodies here behind a stable API
// without touching the dispatcher again.
//
// Refactor-only: pure dispatch, no algorithm change.
var THGCommentExecutor = globalThis.THGCommentExecutor || (() => {
  async function execute(type, message) {
    // Read-only re-check (no compose/submit) — handled before the outbound guard so it
    // works even if the composer module isn't loaded. See specs/domains/facebook-sales-intelligence/features/comment-automation/technical.md.
    if (type === 'thg_reverify_comment') {
      const R = globalThis.THGContentReverify;
      return R ? R.executeReverifyComment(message || {}) : { ok: false, error: 'reverify_not_ready' };
    }
    const O = globalThis.THGContentOutbound;
    if (!O) return { ok: false, error: 'outbound_not_ready' };
    switch (type) {
      case 'thg_execute_outbound':
        return O.executeOutbound(message || {});
      case 'thg_comment_in_group_feed':
        return O.executeCommentInFeed(message || {});
      case 'thg_comment_via_rung2':
        return O.executeCommentViaRung2(message || {});
      default:
        return { ok: false, error: 'unsupported_comment_type:' + type };
    }
  }
  return { execute };
})();

if (typeof module !== 'undefined' && module.exports) module.exports = THGCommentExecutor;
