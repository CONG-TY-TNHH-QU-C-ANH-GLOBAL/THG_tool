/*
 * THG Connector Extension — PR8A: Navigation Hardening telemetry.
 *
 * Pure, side-effect-free helpers that turn the vague `context_drift` /
 * `redirected_feed` terminal into a precise, reproducible root cause.
 *
 *   classifyLanding(url)      → deterministic landing class (one of the
 *                               REDIRECT_CLASS_* tokens, mirrored on the Go
 *                               side as models.RedirectClass*).
 *   buildNavDiagnostic(fields)→ assembles the structured object the backend
 *                               persists into execution_attempts.evidence_json
 *                               (snake_case keys match models.NavDiagnostic).
 *
 * This module does NO DOM mutation and reads only location/URL shape + the
 * fields the caller supplies — the DOM-observation half (article_found,
 * permalink_found, comment_button_found) lives in outbound.js, which owns the
 * article-finding helpers. navreport only classifies and assembles.
 */
var THGNavReport = globalThis.THGNavReport || (() => {
  // Mirror of internal/models/nav_diagnostic.go RedirectClass* constants.
  const REDIRECT_CLASS = {
    PERMALINK: 'permalink',
    FEED: 'feed',
    HOME: 'home',
    LOGIN: 'login',
    CHECKPOINT: 'checkpoint',
    UNSUPPORTED_TARGET: 'unsupported_target',
    UNKNOWN: 'unknown',
  };

  const SNAPSHOT_MAX = 2048;

  function truncateSnapshot(s, max = SNAPSHOT_MAX) {
    const v = String(s || '');
    if (v.length <= max) return v;
    return v.slice(0, max) + '…[truncated]';
  }

  // isCommentablePostShape — does the path/query look like a post permalink
  // we can comment on? Mirrors src/outbox.js isCommentableFacebookPostUrl so
  // a landing on a real post classifies as PERMALINK (not unknown). Kept
  // self-contained because navreport loads before outbox is in scope.
  function isCommentablePostShape(url) {
    const host = url.hostname.toLowerCase();
    if (host === 'fb.watch' || host.endsWith('.fb.watch')) {
      return Boolean(url.pathname.replace(/^\/+|\/+$/g, ''));
    }
    const path = url.pathname.toLowerCase();
    const q = url.searchParams;
    if (q.get('story_fbid') || q.get('multi_permalinks')) return true;
    if (path.includes('/posts/') || path.includes('/permalink/') ||
        path.includes('/videos/') || path.includes('/reel/') ||
        path.includes('/watch/') || path.includes('/share/')) {
      return true;
    }
    return path.endsWith('/photo.php') && Boolean(q.get('fbid'));
  }

  // classifyLanding maps a landed URL onto the closed RedirectClass vocabulary.
  // Order is significant: login / checkpoint walls win over everything (they
  // are human-required, not nav bugs); then home vs feed; then a genuine post
  // permalink; then a recognised non-post FB surface (unsupported_target);
  // else unknown. Deterministic — same URL always yields the same class.
  function classifyLanding(rawUrl) {
    const s = String(rawUrl || '');
    if (!s) return REDIRECT_CLASS.UNKNOWN;

    const low = s.toLowerCase();
    // Login wall — distinct from checkpoint (login = not authenticated).
    if (/\/login(\/|\.php|$|\?)/.test(low) || /(^|\/\/)login\.facebook\.com/.test(low)) {
      return REDIRECT_CLASS.LOGIN;
    }
    // Identity checkpoint / 2FA / security gate.
    if (low.includes('/checkpoint') || low.includes('two_step') ||
        low.includes('/recover') || low.includes('confirm_identity')) {
      return REDIRECT_CLASS.CHECKPOINT;
    }

    let url;
    try { url = new URL(s); } catch { return REDIRECT_CLASS.UNKNOWN; }
    const host = url.hostname.toLowerCase();
    const isFB = host === 'facebook.com' || host.endsWith('.facebook.com') ||
                 host === 'fb.watch' || host.endsWith('.fb.watch');
    if (!isFB) return REDIRECT_CLASS.UNKNOWN;

    const path = url.pathname.replace(/\/+$/, '');
    const q = url.searchParams;

    // Bare root → home. The signature "bounced to the home feed" landing.
    if (path === '' || path === '/') {
      // /?sk=h_chr or /?sk=welcome etc. are still the home feed surface.
      return REDIRECT_CLASS.HOME;
    }
    // Explicit feed surfaces.
    if (path === '/home.php' || path === '/feed' || path === '/feed.php' ||
        (path === '/watch' && !q.get('v'))) {
      return REDIRECT_CLASS.FEED;
    }

    // A real, commentable post permalink → we DID reach the target surface.
    if (isCommentablePostShape(url)) return REDIRECT_CLASS.PERMALINK;

    // Recognised non-post FB surfaces: the nav "succeeded" but there is no
    // post to comment on (photo viewer, marketplace, events, profile, groups
    // home without a post, story viewer).
    if (path.startsWith('/photo') || path.startsWith('/marketplace') ||
        path.startsWith('/events') || path.startsWith('/profile.php') ||
        path.startsWith('/story.php') || /^\/groups\/[^/]+$/.test(path)) {
      return REDIRECT_CLASS.UNSUPPORTED_TARGET;
    }

    return REDIRECT_CLASS.UNKNOWN;
  }

  // buildNavDiagnostic assembles the structured payload. Only keys with a
  // meaningful value are emitted (the backend reads absent as "unobserved").
  // f is a flat object; we normalise types so the JSON matches the Go struct.
  function buildNavDiagnostic(f = {}) {
    const out = {};
    const putStr = (k, v) => { const s = String(v || '').trim(); if (s) out[k] = s; };
    const putInt = (k, v) => { const n = parseInt(v, 10); if (Number.isFinite(n) && n !== 0) out[k] = n; };

    putStr('nav_from_url', f.navFromUrl);
    putStr('nav_to_url', f.navToUrl);
    putInt('nav_duration_ms', f.navDurationMs);
    putInt('nav_attempts', f.navAttempts);
    putStr('landed_url', f.landedUrl);
    putStr('doc_title', f.docTitle);
    // Gate booleans are ALWAYS emitted (false is meaningful here — it says
    // "we looked and it was not there", the whole point of the gate).
    out.article_found = Boolean(f.articleFound);
    out.permalink_found = Boolean(f.permalinkFound);
    out.comment_button_found = Boolean(f.commentButtonFound);
    putStr('target_post_id', f.targetPostId);
    if (Number.isFinite(parseInt(f.accountId, 10)) && parseInt(f.accountId, 10) !== 0) {
      out.account_id = parseInt(f.accountId, 10);
    }
    putStr('fb_user_id', f.fbUserId);
    putStr('redirect_class', f.redirectClass);
    putStr('stage', f.stage);
    putStr('dom_snapshot', f.domSnapshot ? truncateSnapshot(f.domSnapshot) : '');
    return out;
  }

  return { REDIRECT_CLASS, classifyLanding, buildNavDiagnostic, truncateSnapshot };
})();
globalThis.THGNavReport = THGNavReport;
