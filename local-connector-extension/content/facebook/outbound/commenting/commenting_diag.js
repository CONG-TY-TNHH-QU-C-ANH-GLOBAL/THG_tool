// THGCommentingDiag — comment-only diagnostics (probeCommentGates / domCounts / navDiagFor),
// extracted verbatim from outbound.js (Workstream A · PR5): move-only, behavior-preserving.
// Provider/helper layer: depends DOWNWARD only on THGOutboundDom, comment constants,
// THGCommentingTarget (findTargetArticle), and THGNavReport (call time). It MUST NOT depend on
// the executor layer — it never imports or reads the executor global, and receives only plain
// snapshot data (booleans / primitive ctxInfo). Chrome: globalThis.THGCommentingDiag (loaded
// after commenting_target.js, before the executor module); Node: module.exports.
globalThis.THGCommentingDiag = globalThis.THGCommentingDiag || (() => {
  const THGDom = globalThis.THGOutboundDom
    || (typeof require === 'function' ? require('../dom/outbound_dom.js') : null);
  if (!THGDom) {
    throw new Error('THGOutboundDom is required before commenting_diag.js');
  }
  const { visible, labelOf, hasAny } = THGDom;
  const K = globalThis.THGCommentConstants
    || (typeof require === 'undefined' ? null : require('../../commenting/constants/comment_constants.js'));
  const THGTarget = globalThis.THGCommentingTarget
    || (typeof require === 'function' ? require('./commenting_target.js') : null);
  if (!THGTarget) {
    throw new Error('THGCommentingTarget is required before commenting_diag.js');
  }
  const { findTargetArticle } = THGTarget;

  // probeCommentGates inspects the live DOM ONCE and reports the three PR8A
  // pre-comment signals for the target post WITHOUT mutating anything:
  //   article_found       — a target [role=article] is present
  //   permalink_found     — that article carries its canonical permalink anchor
  //   comment_button_found— a Comment/Bình luận button exists in scope
  // Used to explain a target_not_reached landing ("did we even find the post?").
  function probeCommentGates(targetPostId) {
    const out = { articleFound: false, permalinkFound: false, commentButtonFound: false };
    const article = targetPostId ? findTargetArticle(targetPostId) : null;
    out.articleFound = !!article;
    const scope = article || document;
    if (article) {
      out.permalinkFound = !!article.querySelector(
        'a[href*="/posts/"], a[href*="/permalink/"], a[href*="story_fbid="], a[href*="/videos/"], a[href*="/reel/"], a[href*="/share/"]'
      );
    }
    const commentKeys = K.COMMENT_KEYS;
    const buttons = Array.from(scope.querySelectorAll('div[role="button"], button, a[role="button"], span[role="button"]')).filter(el => visible(el));
    out.commentButtonFound = buttons.some(el => {
      const label = labelOf(el);
      return hasAny(label, commentKeys) && !label.includes('share') && !label.includes('like');
    });
    return out;
  }

  // domCounts is the PR8A DOM census — raw element counts on the landed page,
  // captured at the failing gate. The ROOT_CAUSE_REPORT reads these to separate
  // a redirect (everything zero) from a gate failure (article_count>0 but
  // composer_count==0) from a composer/typing failure — WITHOUT a screenshot.
  // Pure read, no mutation.
  function domCounts() {
    const commentKeys = K.COMMENT_KEYS;
    const buttons = Array.from(document.querySelectorAll('div[role="button"], button, a[role="button"], span[role="button"]')).filter(el => visible(el));
    const commentButtons = buttons.filter(el => {
      const label = labelOf(el);
      return hasAny(label, commentKeys) && !label.includes('share') && !label.includes('like');
    });
    return {
      article_count: document.querySelectorAll('[role="article"]').length,
      comment_button_count: commentButtons.length,
      composer_count: document.querySelectorAll('[contenteditable="true"][role="textbox"]').length,
      textarea_count: document.querySelectorAll('textarea').length,
      contenteditable_count: document.querySelectorAll('[contenteditable="true"]').length,
    };
  }

  // navDiagFor assembles the structured NavDiagnostic for the current page
  // state at a given gate. Pure-ish: reads location/title + caller-supplied
  // gate booleans + the background nav trace. Returns {} when THGNavReport is
  // not loaded (defensive — re-injection paths may omit it).
  //
  // phase is the execution phase the caller was ATTEMPTING when it aborted. We
  // refine it deterministically here: a gate-1 abort whose landing is not a real
  // permalink is really a NAVIGATION failure (the tab never reached the post),
  // not a gate failure — that distinction is the Redirect-vs-Gate split the
  // ROOT_CAUSE_REPORT turns on. landed_url is the background-verified landing
  // (≈ target); final_url is where the page actually is now (post-drift).
  function navDiagFor(stage, phase, gates, ctxInfo) {
    if (!THGNavReport) return null;
    const finalUrl = location.href || '';
    const navLanded = ctxInfo.navTrace?.landed_url || '';
    const rc = THGNavReport.classifyLanding(finalUrl);
    let reachedPhase = phase || '';
    if (reachedPhase === 'gate1' && rc !== 'permalink') reachedPhase = 'navigation';
    return THGNavReport.buildNavDiagnostic({
      navFromUrl: ctxInfo.navTrace?.from_url,
      navToUrl: ctxInfo.navTrace?.to_url || ctxInfo.targetUrl,
      navDurationMs: ctxInfo.navTrace?.duration_ms,
      navAttempts: ctxInfo.navTrace?.attempts,
      landedUrl: navLanded,
      finalUrl,
      docTitle: document.title || '',
      articleFound: gates.articleFound,
      permalinkFound: gates.permalinkFound,
      commentButtonFound: gates.commentButtonFound,
      counts: domCounts(),
      phase: reachedPhase,
      targetPostId: ctxInfo.targetPostId,
      accountId: ctxInfo.accountId,
      fbUserId: ctxInfo.fbUID,
      redirectClass: rc,
      stage,
      domSnapshot: gates.articleFound ? '' : (document.body?.innerText || '').slice(0, 2048),
    });
  }

  const api = { probeCommentGates, domCounts, navDiagFor };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  return api;
})();
