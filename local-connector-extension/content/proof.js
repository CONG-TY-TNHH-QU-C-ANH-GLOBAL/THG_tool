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

  // Detect rate-limit / blocked / checkpoint banners in raw page text.
  // Multi-language because FB localises these (VN/EN combined). Returns
  // the matched code string ('rate_limited' | 'blocked' | 'checkpoint')
  // so callers can map to FailureReason.
  //
  // The banner taxonomy maps loosely onto the three FB enforcement
  // surfaces operators have observed:
  //
  //   rate_limited — temporal throttle, account fine, retry later
  //   blocked      — action denied (specific action, post, or
  //                  account-level restriction)
  //   checkpoint   — FB wants identity verification / 2FA / CAPTCHA
  //                  before allowing further interaction; session is
  //                  effectively held until human resolves it
  //
  // Adding banners is additive (new alternations in existing regexes,
  // OR new groups for new outcome codes). When the source-of-truth
  // for an emerging FB phrase isn't certain, prefer 'blocked' over
  // 'rate_limited' — the safe-stop direction.
  function detectPlatformReject() {
    const text = (document.body && document.body.innerText) || '';

    // RATE LIMIT — temporal throttle.
    if (/you('re| are) posting too quickly|too many comments|slow down|sending too many|cham lai|qua nhanh|dang gui qua nhieu/i.test(text)) {
      return 'rate_limited';
    }

    // CHECKPOINT — session-level identity gate. FB intercepts further
    // interaction until the human resolves the prompt. Detecting this
    // distinctly from 'blocked' lets the operator dashboard surface
    // "needs human intervention" instead of "account broken".
    if (/please verify your identity|confirm your identity|verify it's you|xac minh danh tinh|xac nhan danh tinh|enter the code we sent|nhap ma|account has been locked|tai khoan bi khoa|security check|kiem tra bao mat/i.test(text)) {
      return 'checkpoint';
    }

    // ACCOUNT-LEVEL RESTRICTION — FB has soft-restricted the account
    // from this action class. The exact phrasing varies by FB locale
    // + restriction type (commenting restriction, group-engagement
    // restriction, shadow-ban-lite). All map to 'blocked' so the
    // backend's risk pipeline can apply the appropriate signal.
    if (/we('ve| have) temporarily (restricted|blocked|limited)|temporarily blocked|you('re| are) temporarily blocked|restricted from (commenting|using this feature|posting)|tam thoi bi (han che|chan|gioi han)|tai khoan (bi|dang bi) (han che|chan|gioi han)|this feature isn'?t available|tinh nang nay (hien )?khong (kha dung|co san)|action unavailable|hanh dong khong kha dung/i.test(text)) {
      return 'blocked';
    }

    // POST-LEVEL / ACTION-LEVEL block — comment specifically refused.
    if (/comment can't be posted|action blocked|you can't comment|comment was removed|khong the binh luan|binh luan khong duoc dang|hanh dong bi chan|binh luan da bi xoa/i.test(text)) {
      return 'blocked';
    }

    // INBOX / MESSAGE send failure.
    if (/message (was )?not sent|failed to send|couldn't be delivered|tin nhan khong gui duoc|gui khong thanh cong|khong gui duoc/i.test(text)) {
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

    // A platform banner (rate-limit / checkpoint / blocked) is legitimate at
    // ANY phase — FB can surface it before or after a submit — so it stays the
    // first check.
    const platformReject = detectPlatformReject();
    if (platformReject) {
      proof.failure_reason = platformReject;
      proof.success = false;
      proof.notes = 'platform banner detected: ' + platformReject;
      return proof;
    }

    // PR8A PROOF INTEGRITY FIX (deterministic boundary).
    //
    // When the executor already returned ok=false it has classified the EXACT
    // phase it failed at (target_not_reached / context_drift / composer_failed /
    // typing / submit) — that is the authoritative truth. We must NOT override
    // it with the feed-URL heuristic below, because that heuristic emits
    // "page navigated to feed/home after submit", which is a LIE for any failure
    // that never reached the submit phase (article_found=false → nothing was
    // typed, nothing was submitted). The misleading "after submit" string in a
    // gate-1 redirect was the exact contradiction this fix removes: a pre-submit
    // landing on the feed is a NAVIGATION miss (target_not_reached), surfaced
    // verbatim from the executor — not a post-submit redirect.
    if (!ok) {
      proof.failure_reason = mapCommentErrorReason(errorCode);
      proof.notes = errorCode || '';
      return proof;
    }

    // Reaching here means ok=true: the executor cleared the composer, i.e. it
    // genuinely passed the submit phase. ONLY now is a feed/home landing a real
    // post-submit redirect, so the "after submit" wording is accurate.
    if (isFeedishURL(proof.page_url_after)) {
      proof.failure_reason = 'redirected_feed';
      proof.success = false;
      proof.notes = 'page navigated to feed/home after submit';
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
      // Identity-gate aborts from the pre-typing route guard (see
      // outbound.js executeComment). Every one of these means "we
      // refused to type because the rendered post is not the queued
      // target" — semantically equivalent to context_drift, the same
      // outcome the backend identity invariant emits when it detects
      // drift after the fact. Mapping them here keeps the failure
      // taxonomy consistent regardless of which layer caught the miss.
      case 'context_drift':
      case 'target_post_not_on_page':
      case 'target_identity_mismatch':
      case 'target_identity_mismatch_post_click':
      case 'target_identity_mismatch_at_typing':
        return 'context_drift';
      // PR8A: navigation never reached the post (pre-type landing gate). A
      // distinct terminal from context_drift — see internal/models/
      // execution_outcome.go ExecutionTargetNotReached. The executor stopped
      // before typing; retryable, no risk penalty.
      case 'target_not_reached':
        return 'target_not_reached';
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

  // detectMessageRequestState — SENDER-side heuristic for "this message
  // landed in the recipient's Message Requests / pending folder, not their
  // main inbox". This is the silent-non-delivery case: when the executing
  // account and the recipient are NOT connected, Facebook renders the
  // outgoing bubble normally (so node_matched + bubble_fresh would otherwise
  // promote the attempt to dom_verified — a false "verified touch" that
  // wrongly marks the lead protected/followup_pending), yet the recipient
  // may never open the request folder.
  //
  // We key off the notice the SENDER does see ("… will get your message
  // request", "Tin nhắn chờ", "sẽ chỉ nhận được … trong phần Tin nhắn chờ",
  // etc.). Best-effort + heuristic: FB's sender-side copy varies by locale
  // and is not always shown. A MISS degrades to prior behaviour; a HIT
  // downgrades to shadow_rejected with the matched phrase recorded in notes
  // so the operator can refine this phrase list from real failures.
  function detectMessageRequestState() {
    const text = (document.body && document.body.innerText) || '';
    const m = text.match(/message requests?|will (get|receive) your (message )?request|sent a message request|isn'?t receiving messages|can'?t receive (your )?messages?|you can only send (one|1) message|tin nhan cho|tin nhắn chờ|yeu cau tin nhan|yêu cầu tin nhắn|se chi nhan duoc|sẽ chỉ nhận được|chua ket noi voi|chưa kết nối với|khong nhan duoc tin nhan|không nhận được tin nhắn/i);
    return m ? m[0] : '';
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

    // SENDER-side message-request guard. A bubble may have rendered, but if
    // FB tells us the recipient will only get this as a pending request, the
    // contact is NOT a verified touch — refuse to promote to dom_verified so
    // the LeadEngagement projection does not mark the lead protected.
    // failure_reason is unmapped on the backend → ExecutionShadowRejected
    // (the safe non-success class); the granular reason rides proof.notes.
    const requestState = detectMessageRequestState();
    if (requestState) {
      proof.failure_reason = 'message_request_folder';
      proof.success = false;
      proof.notes = 'inbox.message_request_folder: bubble rendered but recipient appears non-connected — delivered to message-requests/pending folder (matched: ' + requestState + ')';
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
        // Prefer /permalink/ anchors over /posts/ — the latter sometimes
        // carries Facebook's top_level_post_id which doesn't resolve as
        // a URL. Same root cause as the dashboard "content isn't
        // available" bug closed in the crawler path.
        const link = a.querySelector('a[href*="/permalink/"]')
          || a.querySelector('a[href*="story_fbid"]')
          || a.querySelector('a[href*="/posts/"]');
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
