// THGCommentingTargetPostId — Facebook post-IDENTITY extraction (URL/anchor → canonical id),
// split verbatim from commenting_target.js (Workstream A · PR7): move-only, behavior-preserving.
// Pure identity layer: depends on NOTHING (URL/location/DOM-read only). Chrome:
// globalThis.THGCommentingTargetPostId (loaded before target/surface.js); Node: module.exports.
globalThis.THGCommentingTargetPostId = globalThis.THGCommentingTargetPostId || (() => {
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

  const api = { extractPostIdFromUrl, extractArticleCanonicalEntityId, onTargetPermalinkPage };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  return api;
})();
