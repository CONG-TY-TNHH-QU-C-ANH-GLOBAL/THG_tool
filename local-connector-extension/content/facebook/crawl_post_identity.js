// THG Facebook crawl — post identity helpers. Pure URL/ID/string functions
// extracted from content/crawl.js (PR-C1C) so the crawl entrypoint stays a
// bridge. No DOM / window / location access here: `location.origin` is injected
// into stripPostQueryParams. Behavior copied verbatim from crawl.js.
globalThis.THGCrawlIdentity = globalThis.THGCrawlIdentity || (() => {
  // Extracts the Facebook-side post id from a permalink. Mirror of the Go
  // helper ExtractFacebookPostID. Empty when no canonical id pattern matches.
  //
  // /permalink/ FIRST because that form always carries the URL-resolvable
  // story_fbid. /posts/ LAST because Facebook sometimes renders the
  // FB-internal top_level_post_id there which doesn't resolve as a URL
  // (the "content isn't available" production bug). When both forms exist
  // on the same article, this ordering picks the working one.
  function extractPostFBID(url) {
    if (!url) return '';
    // Photo viewer URLs (`/photo/?fbid=X` modern, `/photo.php?fbid=X` legacy)
    // carry the PHOTO's fbid, NOT the parent post's. Matching `?fbid=` on
    // these URLs poisons lead.post_fbid with a photo id that does not
    // resolve back to any post — repair pipelines synthesize invalid
    // canonical permalinks and the server rejects them at comment time.
    // Skip the `?fbid=` extraction step when the path is photo-shaped.
    const isPhotoURL = /\/photo(\/|\.|\?|$)/.test(url);
    let m = url.match(/\/permalink\/(\d+)/);
    if (m) return m[1];
    m = url.match(/story_fbid=(\d+)/);
    if (m) return m[1];
    // `set=gm.X` — Facebook's group-media link parameter. The `gm.` prefix
    // marks the value as the PARENT POST fbid attached to a photo viewer
    // URL. This is how photo-only article anchors still surface a real
    // post id — without this clause, photo URLs leave post_fbid empty and
    // the inbound source_url falls back to the group shell.
    m = url.match(/[?&]set=gm\.(\d+)/);
    if (m) return m[1];
    if (!isPhotoURL) {
      m = url.match(/[?&]fbid=(\d+)/);
      if (m) return m[1];
    }
    m = url.match(/\/posts\/(\d+)/);
    if (m) return m[1];
    return '';
  }

  function extractGroupFBID(url) {
    if (!url) return '';
    // Path form `/groups/{id}/...` — canonical when navigation is on a
    // group surface.
    let m = url.match(/\/groups\/(\d+)/);
    if (m) return m[1];
    // `idorvanity={id}` — Facebook's group id query param on photo viewer
    // URLs (paired with `set=gm.X` for the post fbid). Lets us reconstruct
    // the canonical permalink even when the crawler only saw a photo anchor.
    m = url.match(/[?&]idorvanity=(\d+)/);
    if (m) return m[1];
    return '';
  }

  // Build a canonical post permalink from the IDs we already extracted.
  // Mirror of the Go side fburl.CanonicalPostPermalink.
  //
  // Uses the /permalink/ URL form (NOT /posts/). /permalink/ is Facebook's
  // canonical group-navigation path and reliably resolves regardless of
  // which internal id (story_fbid vs top_level_post_id) the caller passed.
  function canonicalPostPermalink(groupFBID, postFBID) {
    if (!postFBID) return '';
    if (groupFBID) return `https://www.facebook.com/groups/${groupFBID}/permalink/${postFBID}/`;
    return `https://www.facebook.com/permalink.php?story_fbid=${postFBID}`;
  }

  // True when the URL carries an identifier the dashboard can open as a
  // specific post (not just the group/page feed shell). Photo viewer URLs are
  // EXCLUDED even though they have `?fbid=` (that fbid is the photo's, not the
  // post's) → would fail the comment identity gate.
  function looksLikePostURL(u) {
    if (!u) return false;
    if (/\/photo(\/|\.|\?|$)/.test(u)) return false;
    return /\/posts\/|\/permalink\/|story_fbid=|multi_permalinks=|[?&]fbid=/.test(u);
  }

  // Stable hash for content+author when Facebook hasn't rendered the permalink
  // yet. djb2 — collision-resilient enough for one crawl session.
  function hashKey(s) {
    let h = 5381;
    for (let i = 0; i < s.length; i++) h = ((h << 5) + h + s.charCodeAt(i)) >>> 0;
    return h.toString(16);
  }

  // Drop comment_id and tracking params from a candidate post URL so the
  // returned link opens at the top of the post, not on a specific comment.
  // The path (/permalink/{id}/ or /posts/{id}/) is preserved verbatim.
  // `origin` is injected (crawl.js passes location.origin) so this stays pure.
  function stripPostQueryParams(raw, origin) {
    if (!raw) return raw;
    try {
      const u = new URL(raw, origin);
      const drop = [];
      u.searchParams.forEach((_v, k) => {
        if (k === 'comment_id' || k === 'reply_comment_id' || k === 'notif_id' ||
            k === 'notif_t' || k === 'ref' || k.indexOf('__') === 0) {
          drop.push(k);
        }
      });
      drop.forEach(k => u.searchParams.delete(k));
      return u.toString();
    } catch (e) {
      return raw;
    }
  }

  return Object.freeze({
    extractPostFBID, extractGroupFBID, canonicalPostPermalink,
    looksLikePostURL, hashKey, stripPostQueryParams,
  });
})();
// CommonJS export for the node test harness. No-op in the extension.
if (typeof module !== 'undefined' && module.exports) module.exports = globalThis.THGCrawlIdentity;
