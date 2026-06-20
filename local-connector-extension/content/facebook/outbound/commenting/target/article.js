// THGCommentingTargetArticle — target-article discovery + stability polling, split verbatim
// from commenting_target.js (Workstream A · PR7): move-only, behavior-preserving. Consumes
// THGOutboundDom + THGCommentingTargetPostId + THGCommentingTargetSurface; reads THGCommentButton
// as a bare global at call time (preserved). Chrome: globalThis.THGCommentingTargetArticle (loaded
// after target/surface.js, before target/composer.js); Node: module.exports.
globalThis.THGCommentingTargetArticle = globalThis.THGCommentingTargetArticle || (() => {
  const THGDom = globalThis.THGOutboundDom
    || (typeof require === 'function' ? require('../../dom/outbound_dom.js') : null);
  if (!THGDom) {
    throw new Error('THGOutboundDom is required before target/article.js');
  }
  const { visible, wait } = THGDom;
  const THGPostId = globalThis.THGCommentingTargetPostId
    || (typeof require === 'function' ? require('./post_id.js') : null);
  if (!THGPostId) {
    throw new Error('THGCommentingTargetPostId is required before target/article.js');
  }
  const { extractArticleCanonicalEntityId } = THGPostId;
  const THGSurface = globalThis.THGCommentingTargetSurface
    || (typeof require === 'function' ? require('./surface.js') : null);
  if (!THGSurface) {
    throw new Error('THGCommentingTargetSurface is required before target/article.js');
  }
  const { commentSurfaceDeps } = THGSurface;

  // articleIsReadyForComment returns true iff the supplied article
  // container is fully mounted enough that we can interact with its
  // composer. Three conditions, all must hold:
  //
  //   1. The article's canonical permalink anchor exists AND is
  //      visible. If FB hasn't rendered the timestamp link yet the
  //      article is in a transient state — we cannot trust scope
  //      lookups against a half-mounted React subtree.
  //
  //   2. A visible "Comment" / "Bình luận" interaction button is
  //      present inside the article. Without it we cannot expand the
  //      composer; waiting for the article to exist but not for its
  //      interactive surface is the source of intermittent flakes.
  //
  //   3. (Implicit, enforced by the caller stability window.) These
  //      conditions hold continuously for stableMs milliseconds, so
  //      we are not catching a transient mount that will unmount.
  function articleIsReadyForComment(article, targetPostId) {
    if (!article) return false;
    const permalink = article.querySelector(
      'a[href*="/posts/"], a[href*="/permalink/"], a[href*="story_fbid="], a[href*="/videos/"], a[href*="/reel/"], a[href*="/share/"]'
    );
    if (!permalink || !visible(permalink)) return false;
    // The comment surface is reachable via EITHER a Comment/Bình luận button (FEED layout)
    // OR an already-mounted in-article composer (PERMALINK layout). Discovery lives in the
    // extracted THGCommentButton module. Checkpoint-3 still re-verifies the editor belongs
    // to the target article before typing, so this never types into the wrong post.
    return THGCommentButton.commentSurfaceState(article, commentSurfaceDeps(targetPostId)).found;
  }

  // waitUntilTargetArticleStable polls the live DOM until the target
  // article container is BOTH present AND ready-for-comment AND
  // stable for stableMs continuous milliseconds, OR until timeoutMs
  // elapses. Returns the stable article reference on success, null
  // on timeout.
  //
  // Why "stable for 500ms" matters: Facebook's SPA frequently mounts
  // an article into the DOM and then unmounts it within 100–300 ms
  // as React reconciles route transitions. waitForTabReady (Chrome's
  // load-complete signal) fires far too early to see that; the
  // article we found at first-paint can be gone before we type. The
  // stability window absorbs that churn — any flicker resets the
  // window and the call keeps polling.
  //
  // Stability is tracked by post IDENTITY (the canonical id matched by
  // findTargetArticle) + readiness, NOT by element reference: FB
  // legitimately remounts the SAME post's article element during
  // virtualised scroll, and resetting on every remount never converges
  // on a busy group-feed permalink page. A genuine content swap to a
  // DIFFERENT post makes findTargetArticle return null (ready=false),
  // which still resets the window — so the anti-route-mismatch guard
  // holds while benign remounts no longer starve the gate.
  async function waitUntilTargetArticleStable(targetPostId, opts) {
    const options = opts || {};
    const timeoutMs = typeof options.timeoutMs === 'number' ? options.timeoutMs : 8000;
    const stableMs = typeof options.stableMs === 'number' ? options.stableMs : 500;
    const pollMs = typeof options.pollMs === 'number' ? options.pollMs : 200;
    if (!targetPostId) return null;
    const deadline = Date.now() + timeoutMs;
    let stableSince = 0;
    let stableArticle = null;
    while (Date.now() < deadline) {
      const article = findTargetArticle(targetPostId);
      const ready = article && articleIsReadyForComment(article, targetPostId);
      if (ready) {
        // Track stability by the target post's IDENTITY (canonical id, matched
        // by findTargetArticle) + readiness — NOT by element reference.
        // Facebook remounts the article element repeatedly while a virtualised
        // group-feed permalink page reconciles, so the old
        // `article === stableArticle` reference check never converged there: the
        // post + composer were present but the window kept resetting on each
        // remount → 8 s timeout → target_not_reached with the composer right
        // there. The anti-route-mismatch guard is fully preserved — every tick
        // still id-matches via findTargetArticle AND requires the comment
        // surface via articleIsReadyForComment, so a content swap to a different
        // post (ready=false) still resets the window below.
        if (stableSince === 0) stableSince = Date.now();
        stableArticle = article; // hand back the freshest reference
        if (Date.now() - stableSince >= stableMs) {
          return stableArticle;
        }
      } else {
        // Target post gone, or its comment surface not yet mounted (or content
        // swapped to a DIFFERENT post — findTargetArticle now returns null) →
        // reset the stability window.
        stableArticle = null;
        stableSince = 0;
      }
      await wait(pollMs);
    }
    return null;
  }

  // findTargetArticle locates the [role="article"] / [role="dialog"]
  // container on the live DOM whose CANONICAL identity (its first
  // post-shape permalink anchor in DOM order — i.e. the post header's
  // timestamp link) matches the target entity id.
  //
  // This is intentionally strict. The previous two-stage fallback
  // (innerHTML.includes) accepted articles that merely *referenced*
  // the target id somewhere — in a shared embedded post, in a reaction
  // button query param, or in a sidebar — and that was the load-bearing
  // bug behind the May-2026 route-mismatch incident
  // (comment id 1293405342441584). The whole point of this guard is
  // "is this article the target post, or just an article that mentions
  // the target post?" — and only canonical-permalink matching answers
  // that correctly.
  //
  // Returns null when no container matches. The caller MUST refuse to
  // type rather than fall back to "first visible comment button".
  function findTargetArticle(postId) {
    if (!postId) return null;
    const id = String(postId);
    const containers = Array.from(document.querySelectorAll('[role="article"], [role="dialog"]')).filter(el => visible(el));
    for (const container of containers) {
      if (extractArticleCanonicalEntityId(container) === id) return container;
    }
    return null;
  }

  const api = { articleIsReadyForComment, waitUntilTargetArticleStable, findTargetArticle };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  return api;
})();
