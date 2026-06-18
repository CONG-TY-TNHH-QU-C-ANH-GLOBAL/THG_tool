// Gate1 comment-ENTRY discovery (spec: specs/COMMENT_ASYNC_REVERIFY.md companion; PR-B + the
// gate1-robustness follow-up). Operator evidence: humans can comment most of these posts, so
// comment_button_not_found is usually a DISCOVERY false-negative, not a non-commentable post.
//
// Gate1 must pass if ANY valid entry point exists under the TARGET article: a Comment/Bình
// luận action button (broadened role/aria/text variants) OR an already-visible composer
// (contenteditable / role=textbox / textarea / "Viết bình luận…" placeholder). If the
// composer is already open we do NOT require an action button. Scoped to the target article
// so a neighbouring post's composer can never satisfy the gate.
//
// Pure logic + injected DOM deps (visible/labelOf/findCommentEditor/scrollIntoCenter/wait/
// now) so it is unit-testable with plain fake nodes — no jsdom.
var THGCommentButton = globalThis.THGCommentButton || (() => {
  const K = globalThis.THGCommentConstants
    || (typeof require === 'undefined' ? null : require('./comment_constants.js'));
  const COMMENT_KEYS = K.COMMENT_KEYS;
  const BUTTON_SEL = 'div[role="button"], button, a[role="button"], span[role="button"], [aria-label]';

  function labelHasComment(label) {
    const l = String(label || '');
    if (!COMMENT_KEYS.some((k) => l.includes(k))) return false;
    return !l.includes('share') && !l.includes('like'); // avoid share/like rows
  }

  // findCommentButton scans the target article for a visible Comment/Bình luận control.
  function findCommentButton(article, deps) {
    if (!article || !article.querySelectorAll) return null;
    const visible = deps.visible || (() => true);
    const labelOf = deps.labelOf || (() => '');
    for (const el of Array.from(article.querySelectorAll(BUTTON_SEL)).filter(visible)) {
      if (labelHasComment(labelOf(el))) return el;
    }
    return null;
  }

  // composerEntry delegates to the scope-robust THGCommentComposer (target-article subtree
  // first, then a page-wide scoped fallback). Returns { el, reason, candidates }.
  function composerEntry(article, deps) {
    const C = globalThis.THGCommentComposer;
    if (C && C.findComposerEntry) return C.findComposerEntry(article, deps);
    // Ultra-fallback if the composer module didn't load: the legacy injected editor finder.
    const el = deps.findCommentEditor ? deps.findCommentEditor(article) : null;
    return { el, reason: el ? 'legacy_editor' : 'none', candidates: [] };
  }

  // commentSurfaceState: how the entry is reachable RIGHT NOW. { found, via, composerReason }.
  function commentSurfaceState(article, deps) {
    if (findCommentButton(article, deps)) return { found: true, via: 'comment_button' };
    const comp = composerEntry(article, deps);
    if (comp.el) return { found: true, via: 'composer_entry', composerReason: comp.reason };
    if (deps.findCommentEditor && deps.findCommentEditor(article)) return { found: true, via: 'composer_entry', composerReason: 'legacy_editor' };
    return { found: false, via: 'none' };
  }

  // diagnostics gathers the structured gate1 instrumentation.
  function diagnostics(article, deps) {
    const visible = (deps && deps.visible) || (() => true);
    const labelOf = (deps && deps.labelOf) || (() => '');
    const out = {
      article_found: !!article, permalink_found: false, action_row_found: false,
      comment_button_found: false, composer_entry_found: false,
      visible_button_texts: [], aria_labels: [],
      textbox_candidates_count: 0, contenteditable_candidates_count: 0,
      gate1_passed_via: 'none',
    };
    if (!article || !article.querySelectorAll) return out;
    out.permalink_found = !!article.querySelector(
      'a[href*="/posts/"], a[href*="/permalink/"], a[href*="story_fbid="], a[href*="/videos/"], a[href*="/reel/"], a[href*="/share/"]'
    );
    out.comment_button_found = !!findCommentButton(article, deps);
    const comp = composerEntry(article, deps);
    out.composer_entry_found = !!comp.el;
    out.composer_reason = comp.reason || (comp.el ? 'found' : 'none');
    out.composer_candidates = comp.candidates || []; // per-candidate {aria,role,parent_text,accepted,reason}
    out.textbox_candidates_count = (comp.candidates || []).length;
    out.contenteditable_candidates_count = article.querySelectorAll('[contenteditable="true"]').length;
    for (const el of Array.from(article.querySelectorAll(BUTTON_SEL)).filter(visible)) {
      const label = labelOf(el);
      if (label) {
        out.visible_button_texts.push(label.slice(0, 40));
        if (label.includes('like') || label.includes('share') || labelHasComment(label)) out.action_row_found = true;
      }
      const aria = el.getAttribute && el.getAttribute('aria-label');
      if (aria) out.aria_labels.push(String(aria).slice(0, 40));
    }
    out.gate1_passed_via = out.comment_button_found ? 'comment_button' : (out.composer_entry_found ? 'composer_entry' : 'none');
    return out;
  }

  // discoverCommentSurface is the bounded fallback: alternate scroll-into-center /
  // toward-bottom, re-querying the button THEN the composer, up to ~12s.
  async function discoverCommentSurface(article, deps) {
    const wait = deps.wait || (async () => {});
    const now = deps.now || (() => 0);
    const timeoutMs = typeof deps.timeoutMs === 'number' ? deps.timeoutMs : 12000;
    const pollMs = typeof deps.pollMs === 'number' ? deps.pollMs : 400;
    let scrolledAttempts = 0;
    let towardBottom = false;
    let st = commentSurfaceState(article, deps);
    const deadline = now() + timeoutMs;
    while (!st.found && now() < deadline) {
      if (deps.scrollIntoCenter) { deps.scrollIntoCenter(article, towardBottom); scrolledAttempts++; }
      towardBottom = !towardBottom; // alternate center / toward-bottom to surface the composer
      await wait(pollMs);
      st = commentSurfaceState(article, deps);
    }
    return { found: st.found, via: st.via, scrolledAttempts };
  }

  // classifyGate1Failure: reached the post (article+permalink) but NO entry (neither button
  // nor composer) → comment_button_not_found (pre-submit, retryable). Otherwise the post was
  // never reached → target_not_reached.
  function classifyGate1Failure(gates) {
    const g = gates || {};
    if (g.articleFound && g.permalinkFound && !g.commentButtonFound && !g.composerEntryFound) return 'comment_button_not_found';
    return 'target_not_reached';
  }

  return {
    COMMENT_KEYS, findCommentButton, commentSurfaceState,
    diagnostics, discoverCommentSurface, classifyGate1Failure,
  };
})();
globalThis.THGCommentButton = THGCommentButton;
if (typeof module !== 'undefined' && module.exports) module.exports = THGCommentButton;
