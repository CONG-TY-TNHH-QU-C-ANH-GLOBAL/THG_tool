// THGCommentingTarget — Facebook comment TARGET/identity/composer-discovery helpers,
// extracted verbatim from outbound.js (Workstream A · PR5): move-only, behavior-preserving.
// Lowest comment layer: consumes generic primitives from THGOutboundDom + comment constants/
// composer/button siblings (read at call time as bare globals). Depends on NOTHING later in
// load order. Chrome: globalThis.THGCommentingTarget (manifest-loaded after inbox_outbound.js,
// before commenting_diag.js); Node: module.exports.
globalThis.THGCommentingTarget = globalThis.THGCommentingTarget || (() => {
  const THGDom = globalThis.THGOutboundDom
    || (typeof require === 'function' ? require('./outbound_dom.js') : null);
  if (!THGDom) {
    throw new Error('THGOutboundDom is required before commenting_target.js');
  }
  const { visible, labelOf, hasAny, norm, wait } = THGDom;
  const K = globalThis.THGCommentConstants
    || (typeof require === 'undefined' ? null : require('./comment_constants.js'));
  // Debug-gated swallow for best-effort browser calls (silent at normal runtime).
  function ignoreErr(e, ctx) { if (globalThis.__THG_COMMENTING_DEBUG__) console.debug(`[THGCommentingTarget] ${ctx}`, e); }

  // extractPostIdFromUrl pulls the canonical Facebook post identifier
  // out of a target URL. Returns "" when the URL is missing or shaped
  // in a way the executor cannot pin to a specific post — caller then
  // falls back to legacy global scoping. Recognised forms:
  //   /groups/<gid>/posts/<numeric>/
  //   /<user>/posts/<numeric>/
  //   /<user>/posts/pfbid<base64ish>
  //   /<page>/permalink/<numeric>/
  //   /<page>/videos/<numeric>/   /<page>/reel/<numeric>/   /watch/<numeric>/
  //   ?story_fbid=<id>
  //   /photo.php?fbid=<id>
  // postIdFromQuery resolves a FB post id from query params, in priority order: story_fbid,
  // first multi_permalinks id, photo.php fbid, watch v. Returns '' when none apply.
  function postIdFromQuery(url, path) {
    const sf = url.searchParams.get('story_fbid');
    if (sf) return sf;
    // multi_permalinks may be a comma-list; the first id is the canonical target.
    const mp = url.searchParams.get('multi_permalinks');
    if (mp) {
      const first = mp.split(',')[0].trim();
      if (first) return first;
    }
    const lower = path.toLowerCase();
    if (lower.endsWith('/photo.php')) {
      const fbid = url.searchParams.get('fbid');
      if (fbid) return fbid;
    }
    if (lower.includes('/watch')) { // /watch/?v=<id> — id lives in the query param.
      const v = url.searchParams.get('v');
      if (v) return v;
    }
    return '';
  }

  function extractPostIdFromUrl(raw) {
    try {
      // Relative hrefs (e.g. "/groups/X/permalink/123/") — prepend a base so new URL() parses.
      let s = String(raw || '');
      if (s.startsWith('/') && !s.startsWith('//')) s = 'https://www.facebook.com' + s;
      const url = new URL(s);
      // Foreign-host guard: reject non-Facebook hosts so a hostile anchor can't spoof the
      // identity gate (e.g. https://shortener.evil/posts/123 shape-matches but isn't FB).
      const host = url.hostname.toLowerCase();
      const isFB = host === 'facebook.com' || host.endsWith('.facebook.com') ||
                   host === 'fb.watch' || host.endsWith('.fb.watch');
      if (!isFB) return '';
      const path = url.pathname;
      // pfbid (alphanumeric) matched BEFORE numeric. [a-z0-9] under /i already covers A-Z.
      const pf = /\/(?:posts|permalink|videos|reel|watch|share)\/(pfbid[a-z0-9]+)/i.exec(path);
      if (pf) return pf[1];
      const num = /\/(?:posts|permalink|videos|reel|watch|share)\/(\d{6,})/i.exec(path);
      if (num) return num[1];
      return postIdFromQuery(url, path);
    } catch {
      return '';
    }
  }

  // extractArticleCanonicalEntityId returns the entity id of the post
  // that the supplied article container ACTUALLY REPRESENTS.
  //
  // The rule: the FIRST post-shape permalink anchor in DOM order is the
  // article's own timestamp link — Facebook's UI puts the timestamp at
  // the very top of the post header, and that timestamp is rendered as
  // an <a> targeting the post's permalink. Anchors that appear later
  // belong to embedded shared posts, "Related posts" carousels,
  // reaction buttons with fbclid query params, or comment-thread links.
  // Those are NOT the article's own identity.
  //
  // Returns "" when no permalink anchor exists — the caller MUST treat
  // that as "identity unverifiable" and abort rather than guess.
  function extractArticleCanonicalEntityId(article) {
    if (!article) return '';
    const anchors = article.querySelectorAll(
      'a[href*="/posts/"], a[href*="/permalink/"], a[href*="story_fbid="], a[href*="/videos/"], a[href*="/reel/"], a[href*="/share/"], a[href*="fbid="]'
    );
    for (const a of anchors) {
      const href = a.getAttribute('href') || a.href || '';
      const id = extractPostIdFromUrl(href);
      if (id) return id;
    }
    return '';
  }

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

  // onTargetPermalinkPage reports whether the BROWSER URL currently addresses the
  // target post ITSELF (its own permalink page). On such a page the focused post
  // is unambiguous — the page-level comment composer ("Write an answer…") belongs
  // to the target post even when it sits OUTSIDE the post's [role=article]
  // (comments already expanded, no in-article Comment button — the observed 204
  // case: comment_button_found=0 but composer_count=1). The in-article scoping
  // guards exist to disambiguate FEED pages (many posts); a permalink page has a
  // single focused post — its URL — so a page-level composer is safe to use there.
  function onTargetPermalinkPage(postId) {
    return !!postId && extractPostIdFromUrl(location.href || '') === String(postId);
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

  const api = {
    extractPostIdFromUrl, extractArticleCanonicalEntityId, classifyHostFor, isCreatePostComposer,
    commentSurfaceDeps, discoverDeps, articleIsReadyForComment, waitUntilTargetArticleStable,
    findTargetArticle, onTargetPermalinkPage, findCommentEditor, findComposerForTarget, acquireTargetComposer,
  };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  return api;
})();
