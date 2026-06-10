// Gate1 comment-button discovery (spec: specs/COMMENT_ASYNC_REVERIFY.md companion; PR-B).
// Extracted OUT of the outbound.js composer god file. Some FB group posts render
// article + permalink but lazily mount the action row (Comment/Bình luận button) only when
// scrolled into view — gate1 then timed out as target_not_reached even though the post was
// right there. This module broadens the button search, adds a scroll-into-center + retry
// fallback, gathers diagnostics, and (critically) classifies "reached the post but no
// comment button" as comment_button_not_found, NOT target_not_reached.
//
// Pure logic + injected DOM deps (visible/labelOf/findCommentEditor/scrollIntoCenter/wait/
// now) so it is unit-testable with plain fake nodes — no jsdom.
var THGCommentButton = globalThis.THGCommentButton || (() => {
  const COMMENT_KEYS = [
    'comment', 'write a comment', 'binh luan', 'viet binh luan',
    'bình luận', 'viết bình luận', 'add a comment',
  ];

  function labelHasComment(label) {
    const l = String(label || '');
    if (!COMMENT_KEYS.some((k) => l.includes(k))) return false;
    return !l.includes('share') && !l.includes('like'); // avoid share/like rows
  }

  // findCommentButton scans an article for a visible Comment/Bình luận control using
  // broadened role + aria-label variants. deps: { visible, labelOf }. Returns el | null.
  function findCommentButton(article, deps) {
    if (!article || !article.querySelectorAll) return null;
    const visible = deps.visible || (() => true);
    const labelOf = deps.labelOf || (() => '');
    const sel = 'div[role="button"], button, a[role="button"], span[role="button"], [aria-label]';
    const els = Array.from(article.querySelectorAll(sel)).filter(visible);
    for (const el of els) {
      if (labelHasComment(labelOf(el))) return el;
    }
    return null;
  }

  // commentSurfaceState classifies how the comment surface is reachable RIGHT NOW: a Comment
  // button to expand it, or an already-mounted composer (permalink layout). { found, via }.
  function commentSurfaceState(article, deps) {
    if (findCommentButton(article, deps)) return { found: true, via: 'button' };
    if (deps.findCommentEditor && deps.findCommentEditor(article)) return { found: true, via: 'composer' };
    return { found: false, via: 'none' };
  }

  // diagnostics gathers the gate1 instrumentation fields (requirement 1).
  function diagnostics(article, deps) {
    const visible = (deps && deps.visible) || (() => true);
    const labelOf = (deps && deps.labelOf) || (() => '');
    const out = {
      article_found: !!article,
      permalink_found: false,
      comment_button_found: false,
      action_row_found: false,
      visible_buttons_text: [],
      aria_labels: [],
    };
    if (!article || !article.querySelectorAll) return out;
    out.permalink_found = !!article.querySelector(
      'a[href*="/posts/"], a[href*="/permalink/"], a[href*="story_fbid="], a[href*="/videos/"], a[href*="/reel/"], a[href*="/share/"]'
    );
    const els = Array.from(article.querySelectorAll('div[role="button"], button, a[role="button"], span[role="button"], [aria-label]')).filter(visible);
    for (const el of els) {
      const label = labelOf(el);
      if (label) {
        out.visible_buttons_text.push(label.slice(0, 40));
        if (labelHasComment(label)) out.comment_button_found = true;
        if (label.includes('like') || label.includes('share') || labelHasComment(label)) out.action_row_found = true;
      }
      const aria = el.getAttribute && el.getAttribute('aria-label');
      if (aria) out.aria_labels.push(String(aria).slice(0, 40));
    }
    return out;
  }

  // discoverCommentSurface is the fallback: scroll the article into center, then poll for
  // the comment surface (button OR composer) within a bounded time, retrying. Returns
  // { found, via, scrolledAttempts, expandedAttempts }. deps adds scrollIntoCenter/wait/now.
  async function discoverCommentSurface(article, deps) {
    const wait = deps.wait || (async () => {});
    const now = deps.now || (() => 0);
    const timeoutMs = typeof deps.timeoutMs === 'number' ? deps.timeoutMs : 4000;
    const pollMs = typeof deps.pollMs === 'number' ? deps.pollMs : 300;
    let scrolledAttempts = 0;
    let expandedAttempts = 0;

    let st = commentSurfaceState(article, deps);
    const deadline = now() + timeoutMs;
    while (!st.found && now() < deadline) {
      if (deps.scrollIntoCenter) { deps.scrollIntoCenter(article); scrolledAttempts++; }
      // A collapsed action row sometimes hides behind a "more actions" control — try to
      // reveal it before the next probe.
      if (deps.expandMoreActions && deps.expandMoreActions(article)) expandedAttempts++;
      await wait(pollMs);
      st = commentSurfaceState(article, deps);
    }
    return { found: st.found, via: st.via, scrolledAttempts, expandedAttempts };
  }

  // classifyGate1Failure: reached the post (article+permalink) but no comment button →
  // comment_button_not_found (a pre-submit, retryable, no-risk failure distinct from a true
  // navigation miss). Otherwise target_not_reached.
  function classifyGate1Failure(gates) {
    const g = gates || {};
    if (g.articleFound && g.permalinkFound && !g.commentButtonFound) return 'comment_button_not_found';
    return 'target_not_reached';
  }

  return {
    COMMENT_KEYS,
    findCommentButton,
    commentSurfaceState,
    diagnostics,
    discoverCommentSurface,
    classifyGate1Failure,
  };
})();
globalThis.THGCommentButton = THGCommentButton;
if (typeof module !== 'undefined' && module.exports) module.exports = THGCommentButton;
