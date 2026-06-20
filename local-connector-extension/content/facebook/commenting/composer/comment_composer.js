// Gate1 composer discovery (scope-robust). Operator evidence: a post can have a visible
// editable composer (role=textbox / contenteditable, aria "Write an answer…") that lives
// OUTSIDE the [role="article"] subtree gate1 scopes to — so the article-only query missed it
// and gate1 false-failed comment_button_not_found even though a human can comment.
//
// This module finds the composer for the TARGET post: target-article subtree FIRST, then a
// page-wide fallback that ACCEPTS a target-post / comment / answer / reply composer and
// REJECTS the global create-post composer and any neighbouring post's composer — with a
// per-candidate reject reason for diagnostics. Detection only (never types/submits).
//
// Pure + injected deps (visible / closestArticle / docEditables) so it unit-tests with fake
// nodes — no jsdom.
var THGCommentComposer = globalThis.THGCommentComposer || (() => {
  const EDITABLE_SEL = '[role="textbox"], [contenteditable="true"], textarea';
  // A target-post composer reads as comment / answer / reply (localised). Text-agnostic on
  // shape: any editable box scoped to the target article is accepted regardless of text.
  const COMMENT_REPLY_KEYS = [
    'comment', 'bình luận', 'binh luan', 'viết bình luận', 'viet binh luan',
    'answer', 'reply', 'trả lời', 'tra loi', 'phản hồi', 'phan hoi',
  ];
  // The GLOBAL create-post composer at the top of the page must never satisfy a post's gate.
  const CREATE_POST_KEYS = [
    'create a post', 'create post', 'create a public', "what's on your mind", 'whats on your mind',
    'bạn viết gì', 'ban viet gi', 'write something', 'viết gì đó', 'viet gi do', 'public post',
    'tạo bài viết', 'tao bai viet',
  ];

  function isEditableShape(el) {
    if (!el || !el.getAttribute) return false;
    const role = (el.getAttribute('role') || '').toLowerCase();
    const ce = (el.getAttribute('contenteditable') || '').toLowerCase();
    const tag = (el.tagName || '').toLowerCase();
    return role === 'textbox' || ce === 'true' || tag === 'textarea';
  }

  function textOf(el) {
    const parts = [el.getAttribute('aria-label') || '', el.getAttribute('placeholder') || ''];
    if (el.parentElement && el.parentElement.textContent) parts.push(el.parentElement.textContent.slice(0, 80));
    return parts.join(' ').toLowerCase();
  }

  const hasAny = (s, keys) => keys.some((k) => s.includes(k));

  // hostVerdict: channel-neutral identity decision for a candidate's nearest container.
  // The channel adapter supplies the container's id (hostId), the target id (targetId), and
  // whether the current URL already pins identity to a single target (urlPinsIdentity, e.g. a
  // post's own permalink page). When the URL pins identity, a host id that differs is a NESTED
  // item (a comment/answer/reply container), NOT a competing target — so we report 'unknown'
  // (let shape/keyword decide) instead of 'foreign'. On listing/feed surfaces a differing id
  // is a genuinely different target → 'foreign'.
  function hostVerdict({ hostId, targetId, urlPinsIdentity }) {
    if (!hostId || !targetId) return 'unknown';
    if (String(hostId) === String(targetId)) return 'target';
    return urlPinsIdentity ? 'unknown' : 'foreign';
  }

  // classify one editable candidate against the target article. Returns { accepted, reason }.
  //
  // Post IDENTITY is channel-specific (Facebook compares canonical permalink ids), so the host
  // article's verdict is supplied by an injected deps.classifyHost(host) → 'target' | 'foreign'
  // | 'unknown'. Generic core never parses channel ids. We hard-reject wrong_post ONLY on a
  // positive 'foreign' verdict; an 'unknown' host (e.g. a comment item article that carries no
  // own post permalink, or a layout wrapper) falls through to a shape/keyword check so a real
  // answer/comment composer is not lost — this is the gate1 false-negative the previous strict
  // `host === article` identity check produced on group "Write an answer…" posts.
  function classify(el, article, deps) {
    const visible = deps.visible || (() => true);
    if (!isEditableShape(el)) return { accepted: false, reason: 'unsupported_editable_shape' };
    if (!visible(el)) return { accepted: false, reason: 'invisible' };
    const txt = textOf(el);
    if (hasAny(txt, CREATE_POST_KEYS)) return { accepted: false, reason: 'create_post_composer' };
    // Subtree containment is robust to nested inner articles/dialogs FB renders for the
    // question/answer block, embedded content, or comment items.
    if (article && article.contains && article.contains(el)) return { accepted: true, reason: 'in_target_article' };
    const host = deps.closestArticle ? deps.closestArticle(el) : null;
    if (host && host === article) return { accepted: true, reason: 'in_target_article' };
    const verdict = host && deps.classifyHost ? deps.classifyHost(host) : 'unknown';
    if (verdict === 'target') return { accepted: true, reason: 'in_target_article' };
    if (verdict === 'foreign') return { accepted: false, reason: 'wrong_post' };
    // Host identity unknown (or no host article): accept only a comment/answer/reply-shaped
    // composer (the global create-post box is already excluded above).
    if (hasAny(txt, COMMENT_REPLY_KEYS)) return { accepted: true, reason: 'target_discussion_region' };
    // A host article exists but is neither the target nor answer-shaped → ambiguous → reject.
    if (host && host !== article) return { accepted: false, reason: 'wrong_post' };
    return { accepted: false, reason: 'outside_target_scope' };
  }

  // findComposerEntry: target-article subtree FIRST, then page-wide scoped fallback. Returns
  // { el, reason, candidates } — candidates carry per-element diagnostics for the gate1 log.
  function findComposerEntry(article, deps) {
    const out = { el: null, reason: 'none', candidates: [] };
    const visible = deps.visible || (() => true);
    if (article && article.querySelectorAll) {
      for (const el of Array.from(article.querySelectorAll(EDITABLE_SEL))) {
        if (visible(el)) { out.el = el; out.reason = 'in_target_article'; return out; }
      }
    }
    const all = (deps.docEditables ? deps.docEditables() : []) || [];
    for (const el of all) {
      const c = classify(el, article, deps);
      out.candidates.push({
        aria: (el.getAttribute && el.getAttribute('aria-label')) || '',
        role: (el.getAttribute && el.getAttribute('role')) || '',
        parent_text: ((el.parentElement && el.parentElement.textContent) || '').trim().slice(0, 60),
        accepted: c.accepted,
        reason: c.reason,
      });
      if (c.accepted && !out.el) { out.el = el; out.reason = c.reason; }
    }
    return out;
  }

  return { EDITABLE_SEL, COMMENT_REPLY_KEYS, CREATE_POST_KEYS, isEditableShape, hostVerdict, classify, findComposerEntry };
})();
globalThis.THGCommentComposer = THGCommentComposer;
if (typeof module !== 'undefined' && module.exports) module.exports = THGCommentComposer;
