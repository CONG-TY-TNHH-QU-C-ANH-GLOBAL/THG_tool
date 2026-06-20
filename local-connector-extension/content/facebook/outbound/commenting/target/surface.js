// THGCommentingTargetSurface — comment-surface deps + composer-vocabulary helpers, split
// verbatim from commenting_target.js (Workstream A · PR7): move-only, behavior-preserving.
// Consumes THGOutboundDom + comment constants + THGCommentingTargetPostId; reads THGCommentComposer
// as a bare global at call time (preserved). Chrome: globalThis.THGCommentingTargetSurface (loaded
// after target/post_id.js, before target/article.js); Node: module.exports.
globalThis.THGCommentingTargetSurface = globalThis.THGCommentingTargetSurface || (() => {
  const THGDom = globalThis.THGOutboundDom
    || (typeof require === 'function' ? require('../../dom/outbound_dom.js') : null);
  if (!THGDom) {
    throw new Error('THGOutboundDom is required before target/surface.js');
  }
  const { visible, labelOf, hasAny, norm, wait } = THGDom;
  const K = globalThis.THGCommentConstants
    || (typeof require === 'undefined' ? null : require('../../../commenting/constants/comment_constants.js'));
  const THGPostId = globalThis.THGCommentingTargetPostId
    || (typeof require === 'function' ? require('./post_id.js') : null);
  if (!THGPostId) {
    throw new Error('THGCommentingTargetPostId is required before target/surface.js');
  }
  const { extractArticleCanonicalEntityId, onTargetPermalinkPage } = THGPostId;
  // Debug-gated swallow for best-effort browser calls (silent at normal runtime).
  function ignoreErr(e, ctx) { if (globalThis.__THG_COMMENTING_DEBUG__) console.debug(`[THGCommentingTargetSurface] ${ctx}`, e); }

  // commentSurfaceDeps injects the DOM helpers the comment_button.js + comment_composer.js
  // modules need (kept here so the modules stay pure + unit-testable). closestArticle +
  // docEditables enable the page-wide, article-scoped composer fallback.
  // classifyHostFor builds the Facebook-specific host-identity verdict the generic composer
  // core injects via deps.classifyHost: a candidate's nearest [role=article] is compared by
  // CANONICAL permalink id, not DOM-node identity. 'target' = same post, 'foreign' = a
  // positively different post (wrong_post), 'unknown' = a host with no own post permalink (a
  // comment item / wrapper) where the core falls back to shape/keyword acceptance. When no
  // targetPostId is supplied (legacy profile_post/inbox callers) every host reads 'unknown',
  // preserving the pre-existing backward-compat behaviour.
  function classifyHostFor(targetPostId) {
    // urlPinsIdentity: on the target post's OWN permalink page the URL identifies a single
    // top-level post, so a host [role=article] that extracts a DIFFERENT id is a nested
    // comment/answer item — not a competing post. The channel-neutral verdict rule lives in
    // the composer core (THGCommentComposer.hostVerdict); this Facebook layer only supplies
    // the canonical ids + the permalink signal. FEED pages keep the strict 'foreign' verdict.
    const urlPinsIdentity = onTargetPermalinkPage(targetPostId);
    return (host) => {
      if (!host || !targetPostId) return 'unknown';
      return THGCommentComposer.hostVerdict({
        hostId: extractArticleCanonicalEntityId(host), targetId: String(targetPostId), urlPinsIdentity,
      });
    };
  }
  // isCreatePostComposer reuses the composer core's create-post vocabulary so the global
  // "What's on your mind / Tạo bài viết" box can never be mistaken for a post's composer —
  // the one thing the permalink-page identity relaxation must still positively exclude.
  function isCreatePostComposer(el) {
    const C = globalThis.THGCommentComposer;
    if (!el?.getAttribute || !C?.CREATE_POST_KEYS) return false;
    const raw = [el.getAttribute('aria-label') || '', el.getAttribute('placeholder') || '',
      (el.parentElement?.textContent || '').slice(0, 80)].join(' ').toLowerCase();
    return C.CREATE_POST_KEYS.some((k) => raw.includes(k));
  }
  function commentSurfaceDeps(targetPostId) {
    return {
      visible, labelOf, findCommentEditor,
      closestArticle: (el) => (el?.closest?.('[role="article"], [role="dialog"]') ?? null),
      docEditables: () => Array.from(document.querySelectorAll('[role="textbox"], [contenteditable="true"], textarea')),
      classifyHost: classifyHostFor(targetPostId),
    };
  }
  // discoverDeps adds the scroll/retry primitives for the gate1 fallback. scrollIntoCenter
  // alternates center / toward-bottom so a lazily-mounted composer below the action row gets
  // surfaced. Bounded to ~12s (FB group posts can be slow) — never waits forever.
  function discoverDeps(targetPostId) {
    return {
      visible, labelOf, findCommentEditor,
      closestArticle: (el) => (el?.closest?.('[role="article"], [role="dialog"]') ?? null),
      docEditables: () => Array.from(document.querySelectorAll('[role="textbox"], [contenteditable="true"], textarea')),
      classifyHost: classifyHostFor(targetPostId),
      scrollIntoCenter: (el, towardBottom) => {
        try { el.scrollIntoView({ block: towardBottom ? 'end' : 'center' }); } catch (e) { ignoreErr(e, 'scroll'); }
      },
      wait, now: () => Date.now(), timeoutMs: 12000, pollMs: 400,
    };
  }

  function findCommentEditor(scope) {
    const commentKeys = K.COMMENT_KEYS;
    const badKeys = ['search', 'tim kiem', 'message', 'messenger', 'nhan tin'];
    const root = scope || document;
    const editors = Array.from(root.querySelectorAll('[contenteditable="true"], textarea, input[type="text"]'))
      .filter(el => visible(el) && !hasAny(labelOf(el), badKeys));
    return editors.find(el => hasAny(labelOf(el), commentKeys))
      || editors.find(el => norm(el.getAttribute('role')) === 'textbox')
      || editors[0]
      || null;
  }

  const api = { classifyHostFor, isCreatePostComposer, commentSurfaceDeps, discoverDeps, findCommentEditor };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  return api;
})();
