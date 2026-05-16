/*
 * THG Connector Extension — Step 3b: DOM Proof Collector
 *
 * Produces the ExtensionExecutionReport shape the Go backend defines in
 * internal/runtime/verifier.go. Without this module the backend classifies
 * every successful click as optimistic_success (deliberately emits NO risk
 * signal), so risk_score never moves and the Behaviour Profile substrate
 * stays inert. With this module the backend can reach dom_verified, fire
 * RiskSignalSuccess on the executing account, and the verification loop
 * is closed.
 *
 * Proof recipes are intentionally pessimistic: when we can't confirm a
 * specific DOM state, we surface `node_matched: false` rather than a guess.
 * The backend's ClassifyExtensionReport routes weak proof to
 * optimistic_success (still success-class for UI) but only strong proof
 * to dom_verified. Lying here would re-poison the substrate the backend
 * just made trustworthy.
 *
 * Empty/null fields are fine — backend treats them as "absent", not "0".
 */
var THGContentProof = globalThis.THGContentProof || (() => {
  const SNIPPET_MAX = 2048;

  function norm(s) {
    return String(s || '')
      .normalize('NFD')
      .replace(/[̀-ͯ]/g, '')
      .replace(/[đĐ]/g, 'd')
      .trim()
      .toLowerCase();
  }

  function truncateSnippet(s, max = SNIPPET_MAX) {
    const v = String(s || '');
    if (v.length <= max) return v;
    return v.slice(0, max) + '…[truncated]';
  }

  // Build the proof shape with all fields zero/empty. Each executor fills
  // the fields it can verify; the backend reads absent fields as "no info"
  // rather than "definitely false."
  function emptyProof() {
    return {
      success: false,
      failure_reason: '',
      comment_permalink: '',
      message_bubble_id: '',
      dom_snippet: '',
      page_url_after: '',
      count_increased: false,
      node_matched: false,
      bubble_fresh: false,
      duplicate: false,
      notes: ''
    };
  }

  // currentFBUserID extracts the logged-in user's numeric FB id from the
  // page. Used to match the author of a rendered comment node — without
  // this we'd accept ANY comment that happens to contain matching text,
  // which silently approves comments by other people on the same post.
  function currentFBUserID() {
    // Pattern 1: meta tags / inline scripts often carry USER_ID
    const html = document.documentElement?.innerHTML || '';
    const m1 = html.match(/"USER_ID":"(\d+)"/);
    if (m1) return m1[1];
    const m2 = html.match(/"actorID":"(\d+)"/);
    if (m2) return m2[1];
    // Pattern 2: data-userid attribute on a profile node
    const el = document.querySelector('[data-userid]');
    if (el) {
      const id = el.getAttribute('data-userid') || '';
      if (/^\d+$/.test(id)) return id;
    }
    return '';
  }

  // Detect rate-limit / blocked banners in raw page text. Multi-language
  // because FB localises these (VN/EN combined). Returns the matched code
  // string ('rate_limited' | 'blocked') so callers can map to FailureReason.
  function detectPlatformReject() {
    const text = (document.body && document.body.innerText) || '';
    if (/you('re| are) posting too quickly|too many comments|slow down|sending too many|cham lai|qua nhanh/i.test(text)) {
      return 'rate_limited';
    }
    if (/comment can't be posted|action blocked|you can't comment|comment was removed|khong the binh luan|hanh dong bi chan/i.test(text)) {
      return 'blocked';
    }
    if (/message (was )?not sent|failed to send|couldn't be delivered|tin nhan khong gui duoc|gui khong thanh cong/i.test(text)) {
      return 'blocked';
    }
    return '';
  }

  // Detect that the browser drifted to home.php / newsfeed / a watch page
  // after submit. The backend has isTransientFacebookURL but we don't get
  // its decision until after the POST — surfacing this proactively lets
  // the extension reach redirected_feed instead of shadow_rejected.
  function isFeedishURL(u) {
    if (!u) return false;
    if (/\/home\.php/i.test(u)) return true;
    if (/\/watch(\/|$)/i.test(u)) return true;
    // Bare facebook.com with no path
    try {
      const url = new URL(u);
      if (/^(www\.|m\.)?facebook\.com$/i.test(url.hostname)) {
        if (url.pathname === '' || url.pathname === '/') return true;
      }
    } catch (_) {}
    return false;
  }

  // ---------------------------------------------------------------------
  // COMMENT PROOF
  // ---------------------------------------------------------------------

  // Snapshot the visible comment-count integer near the target post.
  // Pre-submit value drives count_increased post-submit. Best-effort:
  // FB renders count in many shapes ("12 comments", "12 bình luận", "12K
  // comments"); we read the largest plausible number on the page and rely
  // on direction-of-change rather than exact value.
  function snapshotCommentCount() {
    const text = (document.body && document.body.innerText) || '';
    const matches = text.match(/\b(\d+(?:[\.,]\d+)?)\s*(comment|bình luận|binh luan)/gi) || [];
    let best = 0;
    for (const m of matches) {
      const n = parseFloat(m.replace(/[^0-9\.]/g, ''));
      if (Number.isFinite(n) && n > best) best = n;
    }
    return best;
  }

  // Find the rendered comment node matching `content` authored by `userID`.
  // Two filters in series:
  //   1. text fuzzy-match (lowercase trimmed substring of first 60 chars)
  //   2. author anchor href contains the executing account's UID
  // Without filter #2 we'd happily accept a comment by SOMEONE ELSE that
  // mirrors our text — that's exactly the hallucination Step 3 closes.
  function findCommentNode(content, userID) {
    const expected = norm(content).slice(0, 60);
    if (!expected) return null;
    const candidates = document.querySelectorAll(
      '[role="article"], [aria-label*="omment" i] [role="article"], div[data-testid*="comment" i]'
    );
    for (const n of candidates) {
      const t = norm(n.innerText || '');
      if (!t || t.indexOf(expected) === -1) continue;
      if (userID) {
        const a = n.querySelector('a[href*="/profile.php?id="], a[href*="facebook.com/"]');
        const href = (a && a.getAttribute('href')) || '';
        if (href.indexOf(userID) === -1) continue;
      }
      return n;
    }
    return null;
  }

  // Pull the comment permalink off a matched comment node.
  function extractCommentPermalink(node) {
    if (!node) return '';
    const a = node.querySelector('a[href*="comment_id="], a[href*="/comments/"]');
    if (!a) return '';
    return a.href || '';
  }

  // Build the full comment proof. Caller passes pre-submit snapshot so we
  // can compute count_increased. duplicateIfFound=true means the matched
  // node was ALREADY there before submit — treat as idempotent duplicate.
  function buildCommentProof({ ok, errorCode, content, userID, preCount, duplicate }) {
    const proof = emptyProof();
    proof.page_url_after = window.location.href || '';
    proof.success = !!ok;

    const platformReject = detectPlatformReject();
    if (platformReject) {
      proof.failure_reason = platformReject;
      proof.success = false;
      proof.notes = 'platform banner detected: ' + platformReject;
      return proof;
    }

    if (isFeedishURL(proof.page_url_after)) {
      proof.failure_reason = 'redirected_feed';
      proof.success = false;
      proof.notes = 'page navigated to feed/home after submit';
      return proof;
    }

    if (!ok) {
      proof.failure_reason = mapCommentErrorReason(errorCode);
      proof.notes = errorCode || '';
      return proof;
    }

    const node = findCommentNode(content, userID);
    if (node) {
      proof.node_matched = true;
      proof.comment_permalink = extractCommentPermalink(node);
      proof.dom_snippet = truncateSnippet(node.innerText || '');
    }
    const postCount = snapshotCommentCount();
    proof.count_increased = postCount > preCount;
    proof.duplicate = !!duplicate;
    return proof;
  }

  // Map the executor's internal error code onto the FailureReason vocabulary
  // the backend taxonomy understands. Anything unknown stays empty → backend
  // routes to shadow_rejected, the safe default.
  function mapCommentErrorReason(errorCode) {
    switch (errorCode) {
      case 'comment_box_not_found':
      case 'comment_text_insert_failed':
      case 'comment_text_not_confirmed':
      case 'comment_submit_not_found':
        return 'composer_failed';
      case 'comment_submit_not_confirmed':
        // We clicked submit but composer never cleared — could be
        // composer issue OR silent reject; ambiguous, mark as composer_failed
        // so the backend doesn't over-poison risk_score.
        return 'composer_failed';
      default:
        return '';
    }
  }

  // ---------------------------------------------------------------------
  // INBOX PROOF
  // ---------------------------------------------------------------------

  // Snapshot the last bubble id (or a hash of its text) so we can detect
  // whether a NEW bubble appeared after submit. Without this, an
  // out-of-date bubble with matching text would be accepted as success.
  function snapshotLastBubble() {
    const bubbles = document.querySelectorAll('[role="row"], [data-testid="mwthreadlist-message-row"]');
    if (!bubbles.length) return '';
    const last = bubbles[bubbles.length - 1];
    return hashOf(last.innerText || '');
  }

  function hashOf(s) {
    let h = 0;
    const t = String(s || '');
    for (let i = 0; i < t.length; i += 1) {
      h = ((h << 5) - h) + t.charCodeAt(i);
      h |= 0;
    }
    return String(h);
  }

  // Find the most-recent bubble whose text fuzzy-matches the queued content.
  // Returns { node, idx, fresh } where fresh derives from any visible
  // "Sent just now" / "vài giây" copy near the bubble.
  function findInboxBubble(content) {
    const expected = norm(content).slice(0, 60);
    if (!expected) return null;
    const bubbles = Array.from(document.querySelectorAll('[role="row"], [data-testid="mwthreadlist-message-row"]'));
    // Walk newest → oldest, bounded to the last ~6 rows.
    const start = Math.max(0, bubbles.length - 6);
    for (let i = bubbles.length - 1; i >= start; i -= 1) {
      const b = bubbles[i];
      const t = norm(b.innerText || '');
      if (t.indexOf(expected) !== -1) {
        return { node: b, idx: i, fresh: detectBubbleFreshness(b) };
      }
    }
    return null;
  }

  function detectBubbleFreshness(node) {
    const text = node?.innerText || '';
    if (/just now|seconds ago|vài giây|vai giay/i.test(text)) return 5;
    if (/1\s*minute|1\s*phút|một phút|mot phut/i.test(text)) return 60;
    if (/\b(\d+)\s*(minute|phút|phut)\b/i.test(text)) return 120;
    if (/hour|giờ|gio|day|ngày|ngay|yesterday|hôm qua|hom qua/i.test(text)) return 9999;
    // Absence of any relative-time copy → assume freshly rendered.
    return 15;
  }

  function isThreadOpen() {
    const u = window.location.href || '';
    return /\/messages\/|\/t\//.test(u);
  }

  function buildInboxProof({ ok, errorCode, content, preBubbleHash }) {
    const proof = emptyProof();
    proof.page_url_after = window.location.href || '';
    proof.success = !!ok;

    const platformReject = detectPlatformReject();
    if (platformReject) {
      proof.failure_reason = platformReject;
      proof.success = false;
      proof.notes = 'platform banner/toast detected: ' + platformReject;
      return proof;
    }

    if (!isThreadOpen()) {
      proof.failure_reason = 'redirected_feed';
      proof.success = false;
      proof.notes = 'thread pane closed after submit';
      return proof;
    }

    if (!ok) {
      proof.failure_reason = mapInboxErrorReason(errorCode);
      proof.notes = errorCode || '';
      return proof;
    }

    const match = findInboxBubble(content);
    if (match && match.node) {
      proof.node_matched = true;
      proof.message_bubble_id = 'row_' + match.idx;
      proof.bubble_fresh = match.fresh <= 30;
      proof.dom_snippet = truncateSnippet(match.node.innerText || '');
      // If the last bubble pre-submit had identical text, this is a duplicate.
      const postHash = hashOf(match.node.innerText || '');
      proof.duplicate = preBubbleHash && postHash === preBubbleHash;
    }
    return proof;
  }

  function mapInboxErrorReason(errorCode) {
    switch (errorCode) {
      case 'message_box_not_found':
      case 'message_button_not_found':
      case 'inbox_text_insert_failed':
      case 'inbox_submit_not_found':
        return 'composer_failed';
      default:
        return '';
    }
  }

  // ---------------------------------------------------------------------
  // POST PROOF (group_post / profile_post)
  // ---------------------------------------------------------------------

  // Posts are the hardest to verify in-place because the composer dialog
  // closes after submit and the new post may not be in the current viewport.
  // v1 recipe: confirm the composer dialog closed, scan the visible feed
  // for a [role="article"] matching the queued content. If absent and no
  // platform reject banner, mark optimistic.
  function buildPostProof({ ok, errorCode, content }) {
    const proof = emptyProof();
    proof.page_url_after = window.location.href || '';
    proof.success = !!ok;

    const platformReject = detectPlatformReject();
    if (platformReject) {
      proof.failure_reason = platformReject;
      proof.success = false;
      proof.notes = 'platform banner detected: ' + platformReject;
      return proof;
    }

    if (!ok) {
      proof.failure_reason = mapPostErrorReason(errorCode);
      proof.notes = errorCode || '';
      return proof;
    }

    // Composer-still-open detection: failure shape that gets reported as
    // shadow_rejected when the dialog never closes despite our wait.
    const composer = document.querySelector('[role="dialog"] [contenteditable="true"]');
    if (composer && norm(composer.innerText || '').indexOf(norm(content).slice(0, 60)) !== -1) {
      proof.success = false;
      proof.failure_reason = 'composer_failed';
      proof.notes = 'composer dialog still open with our content after submit wait';
      return proof;
    }

    // Look for the freshly-rendered article. Permalink isn't reliably
    // available until the page refreshes, so we record what we can.
    const expected = norm(content).slice(0, 60);
    const articles = document.querySelectorAll('[role="article"]');
    for (const a of articles) {
      const t = norm(a.innerText || '').slice(0, 200);
      if (t.indexOf(expected) !== -1) {
        proof.node_matched = true;
        const link = a.querySelector('a[href*="/posts/"], a[href*="story_fbid"], a[href*="/permalink/"]');
        if (link) proof.comment_permalink = link.href || '';
        proof.dom_snippet = truncateSnippet(a.innerText || '');
        break;
      }
    }
    return proof;
  }

  function mapPostErrorReason(errorCode) {
    switch (errorCode) {
      case 'post_composer_not_found':
      case 'post_editor_not_found':
      case 'post_text_insert_failed':
      case 'post_submit_not_found':
        return 'composer_failed';
      default:
        return '';
    }
  }

  return {
    currentFBUserID,
    snapshotCommentCount,
    snapshotLastBubble,
    findCommentNode,
    findInboxBubble,
    buildCommentProof,
    buildInboxProof,
    buildPostProof,
    detectPlatformReject,
    isFeedishURL,
    truncateSnippet,
    emptyProof
  };
})();
globalThis.THGContentProof = THGContentProof;
