// Async comment reverify — content-script search (spec: specs/COMMENT_ASYNC_REVERIFY.md,
// PR-A). Runs on the post page AFTER the connector re-navigates there. Read-only: it never
// types or submits — it only re-runs the comment search a few seconds later (lazy group
// comments / "Most relevant" sort often hide a freshly-posted comment from the in-window
// proof). Reuses THGContentProof; deliberately self-contained so it does NOT grow the
// outbound.js composer god file.
var THGContentReverify = globalThis.THGContentReverify || (() => {
  // Re-check window: a few short retries so a lazily-rendered comment can appear.
  const RETRIES = 6;
  const RETRY_MS = 700;

  function extractPermalink(node) {
    if (!node) return '';
    const a = node.querySelector('a[href*="comment_id="], a[href*="/comments/"]');
    return (a && a.href) || '';
  }

  function sleep(ms) {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }

  // executeReverifyComment searches the current post DOM for the actor's comment matching
  // the expected text. Returns { ok, found, comment_permalink, notes } — never throws.
  async function executeReverifyComment(message) {
    const P = globalThis.THGContentProof;
    if (!P || typeof P.findCommentNode !== 'function') {
      return { ok: false, error: 'proof_not_ready' };
    }
    const content = String((message && message.content) || '');
    if (!content) {
      return { ok: true, found: false, comment_permalink: '', notes: 'no_expected_content' };
    }
    const fbUID = (typeof P.currentFBUserID === 'function' && P.currentFBUserID()) || '';

    let node = P.findCommentNode(content, fbUID);
    for (let i = 0; i < RETRIES && !node; i++) {
      await sleep(RETRY_MS);
      node = P.findCommentNode(content, fbUID);
    }
    const found = !!node;
    return {
      ok: true,
      found,
      comment_permalink: found ? extractPermalink(node) : '',
      notes: found ? 'reverify_found' : 'reverify_not_found_in_window',
    };
  }

  return { executeReverifyComment };
})();
globalThis.THGContentReverify = THGContentReverify;
if (typeof module !== 'undefined' && module.exports) module.exports = THGContentReverify;
