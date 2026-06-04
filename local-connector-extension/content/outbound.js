var THGContentOutbound = globalThis.THGContentOutbound || (() => {
  const wait = (ms) => new Promise(resolve => setTimeout(resolve, ms));
  const norm = (value) => String(value || '')
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[đĐ]/g, 'd')
    .trim()
    .toLowerCase();
  const hasAny = (value, keys) => keys.some(key => value.includes(key));

  function visible(el) {
    if (!el) return false;
    const rect = el.getBoundingClientRect();
    const style = getComputedStyle(el);
    return rect.width > 8 && rect.height > 8 && style.visibility !== 'hidden' && style.display !== 'none';
  }

  function labelOf(el) {
    return norm(el?.innerText || el?.getAttribute?.('aria-label') || el?.getAttribute?.('placeholder') || el?.title);
  }

  function eventInit(x, y, extra = {}) {
    return {
      bubbles: true,
      cancelable: true,
      composed: true,
      clientX: x,
      clientY: y,
      ...extra
    };
  }

  function dispatchMouseLike(el, type, x, y, extra = {}) {
    try {
      el.dispatchEvent(new MouseEvent(type, eventInit(x, y, extra)));
    } catch (_) {}
  }

  function dispatchPointerLike(el, type, x, y, extra = {}) {
    try {
      el.dispatchEvent(new PointerEvent(type, eventInit(x, y, {
        pointerId: 1,
        pointerType: 'mouse',
        isPrimary: true,
        button: 0,
        buttons: type.endsWith('down') ? 1 : 0,
        ...extra
      })));
    } catch (_) {}
  }

  function clickLikeUser(el) {
    if (!el) return false;
    try { el.scrollIntoView({ block: 'center', inline: 'center' }); } catch (_) {}
    const rect = el.getBoundingClientRect();
    const x = Math.max(0, Math.min(window.innerWidth - 1, rect.left + rect.width / 2));
    const y = Math.max(0, Math.min(window.innerHeight - 1, rect.top + rect.height / 2));
    try {
      dispatchPointerLike(el, 'pointerover', x, y);
      dispatchPointerLike(el, 'pointermove', x, y);
      dispatchPointerLike(el, 'pointerdown', x, y);
      dispatchMouseLike(el, 'mousedown', x, y);
      dispatchPointerLike(el, 'pointerup', x, y);
      dispatchMouseLike(el, 'mouseup', x, y);
      dispatchMouseLike(el, 'click', x, y);
      el.click();
      return true;
    } catch (_) {
      try { el.click(); return true; } catch (_) { return false; }
    }
  }

  // labelMatchesDismiss tests whether a normalized label names a dismiss
  // control, using WHOLE-WORD matching — never a raw substring.
  //
  // PR8C-Forensics root cause (2026-06-04): the old `label.includes(key)` with
  // key='ok' matched "facebook" ("faceb-OO-K" contains "ok"), so the top-nav
  // Facebook LOGO (aria-label="Facebook") was treated as an "OK" dismiss button.
  // dismissBlockingOverlays then clicked the logo at t+7ms → FB's SPA router
  // pushState'd the tab to the home feed at t+11ms → every comment failed
  // target_not_reached/redirect_class=home. The forensic last_op_before_reset
  // was literally `dispatchEvent click a[role=link][al=Facebook]`. Word-boundary
  // matching makes "ok" require its own token, so "facebook" no longer matches.
  function labelMatchesDismiss(label, keys) {
    return keys.some((key) => {
      const escaped = key.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
      return new RegExp('(^|\\W)' + escaped + '($|\\W)').test(label);
    });
  }

  async function dismissBlockingOverlays() {
    const labels = ['not now', 'ok', 'close', 'later', 'maybe later', 'remember password', 'de sau', 'luc khac', 'khong phai bay gio'];
    // Candidates are real button controls only. The old selector included a bare
    // `[aria-label]`, which matched the top-nav `a[role="link"]` Facebook logo —
    // a NAVIGATION link, never an overlay-dismiss control. Restricting to
    // button-shaped elements (role="button" / <button>) excludes nav links and
    // is the structural half of the PR8C fix (word-boundary matching is the
    // other half). A real dismiss control is a button, not a link to home.
    const candidates = Array.from(document.querySelectorAll('div[role="button"], button, a[role="button"], span[role="button"]')).filter(visible);
    for (const el of candidates) {
      // Defense-in-depth: never click an element that navigates (an anchor with
      // an href, or anything still carrying role="link"). Overlay dismiss never
      // navigates; a stray match that does would re-introduce the logo bug.
      const role = norm(el.getAttribute?.('role'));
      const isNavLink = role === 'link' || (el.tagName === 'A' && !!el.getAttribute?.('href') && role !== 'button');
      if (isNavLink) continue;
      const label = labelOf(el);
      if (!label) continue;
      if (labelMatchesDismiss(label, labels)) {
        if (clickLikeUser(el)) {
          await wait(500);
          return 'clicked';
        }
      }
    }
    return 'none';
  }

  function textOfEditable(editor) {
    if (!editor) return '';
    if ('value' in editor) return String(editor.value || '');
    return String(editor.innerText || editor.textContent || '');
  }

  function setInputValue(editor, value) {
    const proto = editor instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
    if (setter) setter.call(editor, value);
    else editor.value = value;
  }

  function selectEditableContents(editor) {
    try {
      const range = document.createRange();
      range.selectNodeContents(editor);
      const selection = window.getSelection();
      selection.removeAllRanges();
      selection.addRange(range);
      return true;
    } catch (_) {
      try {
        document.execCommand('selectAll', false, null);
        return true;
      } catch (_) {
        return false;
      }
    }
  }

  function emitEditableInput(editor, text = '') {
    try {
      editor.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText', data: text }));
    } catch (_) {
      editor.dispatchEvent(new Event('input', { bubbles: true }));
    }
    try { editor.dispatchEvent(new Event('change', { bubbles: true })); } catch (_) {}
  }

  function setEditableText(editor, text) {
    try { editor.focus({ preventScroll: true }); } catch (_) { try { editor.focus(); } catch (_) {} }
    if (editor.isContentEditable) {
      selectEditableContents(editor);
      document.execCommand('insertText', false, text);
    } else if ('value' in editor) {
      setInputValue(editor, text);
    } else {
      return false;
    }
    emitEditableInput(editor, text);
    return true;
  }

  async function waitFor(predicate, timeoutMs = 2500, stepMs = 150) {
    const started = Date.now();
    while (Date.now() - started < timeoutMs) {
      if (predicate()) return true;
      await wait(stepMs);
    }
    return predicate();
  }

  function editorContainsContent(editor, content) {
    if (!editor || !document.contains(editor)) return false;
    const current = norm(textOfEditable(editor)).replace(/\s+/g, ' ');
    const expected = norm(content).replace(/\s+/g, ' ');
    if (!expected) return false;
    const sample = expected.slice(0, Math.min(60, expected.length));
    return current.includes(sample);
  }

  function pressEnter(editor) {
    if (!editor) return false;
    try { editor.focus({ preventScroll: true }); } catch (_) { try { editor.focus(); } catch (_) {} }
    const init = {
      key: 'Enter',
      code: 'Enter',
      keyCode: 13,
      which: 13,
      bubbles: true,
      cancelable: true,
      composed: true
    };
    try {
      editor.dispatchEvent(new KeyboardEvent('keydown', init));
      editor.dispatchEvent(new KeyboardEvent('keypress', init));
      editor.dispatchEvent(new KeyboardEvent('keyup', init));
      return true;
    } catch (_) {
      return false;
    }
  }

  function enabledButton(el) {
    return el && el.getAttribute?.('aria-disabled') !== 'true' && !el.disabled;
  }

  function rejectActionLabel(label) {
    return hasAny(label, ['share', 'like', 'cancel', 'photo', 'gif', 'emoji', 'sticker', 'anh', 'huy', 'thich', 'chia se']);
  }

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
  function extractPostIdFromUrl(raw) {
    try {
      // FB DOM anchors often store relative paths in their `href`
      // attribute (e.g. "/groups/X/permalink/123/"). new URL() rejects
      // those — prepend a base so the parser succeeds. The base only
      // affects pathname / searchParams parsing, which is all we need.
      let s = String(raw || '');
      if (s.startsWith('/') && !s.startsWith('//')) s = 'https://www.facebook.com' + s;
      const url = new URL(s);
      // Foreign-host guard. FB pages contain 3rd-party tracking anchors
      // and external shortlinks; their paths can SHAPE-MATCH our regexes
      // (e.g. https://shortener.evil/posts/123) but they obviously do
      // not address a Facebook entity. Reject anything that isn't on a
      // Facebook-controlled host so the identity gate cannot be tricked
      // by hostile DOM content.
      const host = url.hostname.toLowerCase();
      const isFB = host === 'facebook.com' || host.endsWith('.facebook.com') ||
                   host === 'fb.watch' || host.endsWith('.fb.watch');
      if (!isFB) return '';
      const path = url.pathname;
      // Compact identifier ("pfbid..."): match BEFORE the numeric branch
      // because pfbid tokens contain alphanumerics and are unique.
      let m = path.match(/\/(?:posts|permalink|videos|reel|watch|share)\/(pfbid[A-Za-z0-9]+)/i);
      if (m) return m[1];
      // Numeric post id (group posts, legacy permalinks).
      m = path.match(/\/(?:posts|permalink|videos|reel|watch|share)\/(\d{6,})/i);
      if (m) return m[1];
      const sf = url.searchParams.get('story_fbid');
      if (sf) return sf;
      // multi_permalinks may be a comma-list; the first id is the canonical target.
      const mp = url.searchParams.get('multi_permalinks');
      if (mp) {
        const first = mp.split(',')[0].trim();
        if (first) return first;
      }
      if (path.toLowerCase().endsWith('/photo.php')) {
        const fbid = url.searchParams.get('fbid');
        if (fbid) return fbid;
      }
      // /watch/?v=<id> — FB Watch page; id lives in the query param.
      if (path.toLowerCase().includes('/watch')) {
        const v = url.searchParams.get('v');
        if (v) return v;
      }
      return '';
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
  function articleIsReadyForComment(article) {
    if (!article) return false;
    const permalink = article.querySelector(
      'a[href*="/posts/"], a[href*="/permalink/"], a[href*="story_fbid="], a[href*="/videos/"], a[href*="/reel/"], a[href*="/share/"]'
    );
    if (!permalink || !visible(permalink)) return false;
    const commentKeys = ['comment', 'write a comment', 'binh luan', 'viet binh luan'];
    const buttons = Array.from(article.querySelectorAll('div[role="button"], button, a[role="button"], span[role="button"]')).filter(visible);
    return buttons.some(el => {
      const label = labelOf(el);
      return hasAny(label, commentKeys) && !label.includes('share') && !label.includes('like');
    });
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
  // The stability check also catches the case where the canonical
  // identity CHANGES mid-window (FB swaps article content in place
  // during virtualised scroll). When findTargetArticle returns a
  // different element across two checks, the window resets.
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
      const ready = article && articleIsReadyForComment(article);
      if (ready && article === stableArticle) {
        if (stableSince === 0) stableSince = Date.now();
        if (Date.now() - stableSince >= stableMs) {
          return article;
        }
      } else if (ready) {
        // Found a ready article but it's a different reference than
        // the previous tick (or this is the first tick). Start a
        // fresh stability window.
        stableArticle = article;
        stableSince = Date.now();
        // Don't return yet — wait at least one more poll to confirm.
      } else {
        // Either no article, or article not ready. Reset.
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
    const containers = Array.from(document.querySelectorAll('[role="article"], [role="dialog"]')).filter(visible);
    for (const container of containers) {
      if (extractArticleCanonicalEntityId(container) === id) return container;
    }
    return null;
  }

  function findCommentEditor(scope) {
    const commentKeys = ['comment', 'write a comment', 'binh luan', 'viet binh luan'];
    const badKeys = ['search', 'tim kiem', 'message', 'messenger', 'nhan tin'];
    const root = scope || document;
    const editors = Array.from(root.querySelectorAll('[contenteditable="true"], textarea, input[type="text"]'))
      .filter(el => visible(el) && !hasAny(labelOf(el), badKeys));
    return editors.find(el => hasAny(labelOf(el), commentKeys))
      || editors.find(el => norm(el.getAttribute('role')) === 'textbox')
      || editors[0]
      || null;
  }

  function submitScore(editor, button) {
    const er = editor.getBoundingClientRect();
    const br = button.getBoundingClientRect();
    const ey = er.top + er.height / 2;
    const by = br.top + br.height / 2;
    let score = Math.abs(ey - by) + Math.max(0, er.left - br.left) / 3;
    const label = labelOf(button);
    const text = norm(button.innerText || '');
    if (!text) score -= 20;
    if (text && hasAny(text, ['comment', 'binh luan'])) score += 80;
    if (!hasAny(label, ['comment', 'post', 'send', 'binh luan', 'dang', 'gui'])) score += 100;
    return score;
  }

  function submitCandidateSpatial(editor, button) {
    const er = editor.getBoundingClientRect();
    const br = button.getBoundingClientRect();
    const verticallyNear = br.bottom >= er.top - 28 && br.top <= er.bottom + 42;
    const toRight = br.left >= er.left - 10;
    const compact = br.width <= 110 && br.height <= 72;
    return verticallyNear && toRight && compact;
  }

  function findSubmitButtons(editor, excluded = []) {
    const submitKeys = ['comment', 'post', 'send', 'binh luan', 'dang', 'gui'];
    const scopes = [];
    const form = editor.closest('form');
    if (form) scopes.push(form);
    let parent = editor.parentElement;
    for (let i = 0; parent && i < 8; i += 1) {
      scopes.push(parent);
      parent = parent.parentElement;
    }
    scopes.push(editor.closest('[role="dialog"], [role="article"]') || document);
    const seen = new Set(excluded.filter(Boolean));
    const candidates = [];
    for (const scope of scopes) {
      if (!scope) continue;
      for (const el of Array.from(scope.querySelectorAll('div[role="button"], button, [aria-label]'))) {
        if (seen.has(el)) continue;
        seen.add(el);
        const label = labelOf(el);
        const hasSubmitLabel = hasAny(label, submitKeys);
        const spatial = submitCandidateSpatial(editor, el);
        if (!visible(el) || !enabledButton(el)) continue;
        if (label && rejectActionLabel(label)) continue;
        if (!hasSubmitLabel && !spatial) continue;
        if (el === editor || el.contains(editor)) continue;
        candidates.push(el);
      }
      if (candidates.length >= 3) break;
    }
    candidates.sort((a, b) => submitScore(editor, a) - submitScore(editor, b));
    return candidates.slice(0, 5);
  }

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
    const commentKeys = ['comment', 'write a comment', 'binh luan', 'viet binh luan'];
    const buttons = Array.from(scope.querySelectorAll('div[role="button"], button, a[role="button"], span[role="button"]')).filter(visible);
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
    const commentKeys = ['comment', 'write a comment', 'binh luan', 'viet binh luan'];
    const buttons = Array.from(document.querySelectorAll('div[role="button"], button, a[role="button"], span[role="button"]')).filter(visible);
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
    const navLanded = (ctxInfo.navTrace && ctxInfo.navTrace.landed_url) || '';
    const rc = THGNavReport.classifyLanding(finalUrl);
    let reachedPhase = phase || '';
    if (reachedPhase === 'gate1' && rc !== 'permalink') reachedPhase = 'navigation';
    return THGNavReport.buildNavDiagnostic({
      navFromUrl: ctxInfo.navTrace && ctxInfo.navTrace.from_url,
      navToUrl: (ctxInfo.navTrace && ctxInfo.navTrace.to_url) || ctxInfo.targetUrl,
      navDurationMs: ctxInfo.navTrace && ctxInfo.navTrace.duration_ms,
      navAttempts: ctxInfo.navTrace && ctxInfo.navTrace.attempts,
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
      domSnapshot: gates.articleFound ? '' : ((document.body && document.body.innerText) || '').slice(0, 2048),
    });
  }

  async function executeComment(content, targetUrl = '', executionId = '', opts = {}) {
    // PR8C-Forensics: arm the interaction recorder BEFORE the first DOM op
    // (dismissBlockingOverlays already scans + synthetic-clicks). Every
    // querySelectorAll / click / focus / dispatchEvent / innerHTML read /
    // MutationObserver attach our content script performs is now timestamped, so
    // commentResult can name the last op before FB's home pushState. install()
    // is a no-op-safe if the module is missing.
    if (globalThis.THGForensics) THGForensics.install();
    await dismissBlockingOverlays();

    // LIFECYCLE INSTRUMENTATION — captured at every stage so the
    // failure note + DevTools console reveal where the flow broke.
    // Without this, `redirected_feed` masked the actual stage that
    // failed (gate-1 fail on a feed page registered as "redirected"
    // even though the real issue was "FB never loaded the post URL").
    // See project_runtime_control_plane memory: this is a precursor
    // to EXP-1 typed events; structured as proof.notes prefix for now.
    const navAtEntry = location.href || '';
    console.log('[THG outbound.executeComment] start',
      { target_url: targetUrl, landed_at_entry: navAtEntry, execution_id: executionId });

    // Pre-submit DOM snapshot (count + dup check) so the proof builder
    // can compute count_increased and duplicate without relying on the
    // executor's own beliefs.
    const proof = THGContentProof || null;
    const fbUID = proof?.currentFBUserID() || '';
    const preCount = proof ? proof.snapshotCommentCount() : 0;
    const preMatched = proof ? !!proof.findCommentNode(content, fbUID) : false;
    const ctx = { content, userID: fbUID, preCount, duplicate: preMatched, executionId };
    // PR8A: immutable context the NavDiagnostic assembler reads at each gate.
    const targetPostIdEarly = extractPostIdFromUrl(targetUrl);
    const ctxInfo = {
      targetUrl, targetPostId: targetPostIdEarly, fbUID,
      accountId: Number(opts.accountId || 0) || 0,
      navTrace: opts.navTrace || null,
    };

    // ====================================================================
    // P0 INVARIANT — NO TYPING UNTIL TARGET IDENTITY VERIFIED FIRST
    // ====================================================================
    //
    // Three identity checkpoints fire BEFORE setEditableText is ever
    // reached. Each one independently aborts (returns context_drift)
    // rather than mutating any user-visible DOM. Goal: prevent wrong
    // comments, not just detect them after the fact.
    //
    //   Checkpoint 1 — PRE-LOCATE: find an article whose CANONICAL
    //   permalink (first post-shape anchor in DOM order, i.e. the
    //   timestamp link) extracts to the target entity id. No article
    //   found ⇒ abort. This is enforced by findTargetArticle's strict
    //   matching (canonical-only; the loose innerHTML fallback was
    //   removed because it was the root cause of the May-2026
    //   route-mismatch incident).
    //
    //   Checkpoint 2 — POST-CLICK: after the "Comment" / "Bình luận"
    //   button click, Facebook may open a modal dialog. RE-RUN the
    //   canonical-id check on the resolved scope. Mismatch ⇒ abort.
    //
    //   Checkpoint 3 — PRE-TYPE: just before setEditableText runs,
    //   verify the editor's closest enclosing article container STILL
    //   resolves to the target identity. Defends against the edge case
    //   where FB swaps article content between findCommentEditor and
    //   the actual text insertion call.
    //
    // Backward compatibility: when target_url is empty or has no
    // extractable post id, all three checkpoints become no-ops — the
    // legacy document-wide search is preserved for profile_post /
    // inbox / older callers. Comment flows ALWAYS provide a target_url
    // in production (server-side outbox writers require it), so this
    // backward-compat lane carries no live traffic.
    const targetPostId = extractPostIdFromUrl(targetUrl);
    let targetScope = null;
    if (targetPostId) {
      // Checkpoint 1 — pre-locate WITH stability wait.
      //
      // Previously this was a single synchronous findTargetArticle
      // call. That was correct for stable pages but flaked under FB's
      // SPA mount-unmount churn during route transitions: the
      // article would exist at the moment we checked, then unmount
      // before we tried to type. waitUntilTargetArticleStable polls
      // for the article + its permalink + comment button all
      // continuously holding for stableMs — replacing
      // waitForTabReady's "Chrome says load complete" with a
      // DOM-truth check that survives the SPA's intermediate states.
      targetScope = await waitUntilTargetArticleStable(targetPostId, {
        timeoutMs: 8000,
        stableMs: 500,
        pollMs: 200,
      });
      if (!targetScope) {
        const landed = location.href || '';
        // PR8A — DETERMINISTIC LANDING GATE. Navigate → Wait Article → Verify
        // Post ID just FAILED: the queued post never became present+stable on
        // the page. Probe the DOM once to explain why, classify the landing,
        // and STOP — do NOT type. This is target_not_reached, NOT context_drift
        // (we never reached an article to drift from) and NOT redirected_feed
        // (nothing was submitted). Retryable, no risk penalty (see Go side).
        const gates = probeCommentGates(targetPostId);
        const navDiag = navDiagFor('gate1_no_article', 'gate1', gates, ctxInfo);
        const rc = navDiag && navDiag.redirect_class ? navDiag.redirect_class : 'unknown';
        console.warn('[THG outbound.executeComment] gate1 FAIL target_not_reached',
          { target_url: targetUrl, target_id: targetPostId, landed_at_fail: landed,
            nav_at_entry: navAtEntry, redirect_class: rc, gates });
        return commentResult(false, 'target_not_reached', null, ctx,
          'identity_gate_1_target_not_reached: target id=' + abbreviate(targetPostId) +
          ' redirect_class=' + rc +
          ' article_found=' + gates.articleFound +
          ' permalink_found=' + gates.permalinkFound +
          ' comment_button_found=' + gates.commentButtonFound +
          ' landed_at=' + landed + ' nav_at_entry=' + navAtEntry +
          ' did not settle (article+permalink+comment-button stable 500ms) within 8s',
          navDiag);
      }
    }
    const searchRoot = targetScope || document;

    const commentKeys = ['comment', 'write a comment', 'binh luan', 'viet binh luan'];
    const buttons = Array.from(searchRoot.querySelectorAll('div[role="button"], button, a[role="button"], span[role="button"]')).filter(visible);
    const commentButton = buttons.find(el => {
      const label = labelOf(el);
      return hasAny(label, commentKeys) && !label.includes('share') && !label.includes('like');
    });
    if (commentButton) {
      clickLikeUser(commentButton);
      await wait(900);
    }

    // Checkpoint 2 — post-click. The "Comment" click can:
    //  (a) expand the inline comment section in-place (article unchanged),
    //  (b) open a modal dialog anchored to the target post,
    //  (c) open a modal dialog anchored to a DIFFERENT post (rare but
    //      observed when Facebook lazy-resolves the click target).
    // (a) and (b) are safe. (c) is exactly what this checkpoint catches.
    let scope;
    if (targetPostId) {
      const refreshed = findTargetArticle(targetPostId);
      scope = refreshed || targetScope;
      if (extractArticleCanonicalEntityId(scope) !== targetPostId) {
        const landed = location.href || '';
        console.warn('[THG outbound.executeComment] gate2 FAIL post-click swap',
          { target_id: targetPostId, scope_id: extractArticleCanonicalEntityId(scope), landed_at_fail: landed });
        return commentResult(false, 'context_drift', null, ctx,
          'identity_gate_2_post_click_swap: scope canonical id != ' + abbreviate(targetPostId) + ' landed_at=' + landed,
          navDiagFor('gate2_post_click_swap', 'composer', probeCommentGates(targetPostId), ctxInfo));
      }
    } else {
      scope = commentButton?.closest('[role="article"], [role="dialog"]') || document;
    }

    let editor = findCommentEditor(scope);
    if (!editor) {
      window.scrollBy({ top: 420, behavior: 'smooth' });
      await wait(900);
      if (targetPostId) {
        const refreshed = findTargetArticle(targetPostId);
        scope = refreshed || targetScope;
        // Re-verify after the scroll — article references may have gone
        // stale, and the React tree may have rotated.
        if (extractArticleCanonicalEntityId(scope) !== targetPostId) {
          const landed = location.href || '';
          console.warn('[THG outbound.executeComment] gate2b FAIL scroll swap',
            { target_id: targetPostId, scope_id: extractArticleCanonicalEntityId(scope), landed_at_fail: landed });
          return commentResult(false, 'context_drift', null, ctx,
            'identity_gate_2b_scroll_swap: scope canonical id != ' + abbreviate(targetPostId) + ' landed_at=' + landed,
            navDiagFor('gate2b_scroll_swap', 'composer', probeCommentGates(targetPostId), ctxInfo));
        }
        editor = findCommentEditor(scope);
      } else {
        editor = findCommentEditor(scope) || findCommentEditor(document);
      }
    }
    if (!editor) {
      const landed = location.href || '';
      console.warn('[THG outbound.executeComment] comment_box_not_found',
        { target_id: targetPostId, landed_at_fail: landed });
      return commentResult(false, 'comment_box_not_found', null, ctx,
        'comment_box_not_found: target id=' + abbreviate(targetPostId || '<none>') + ' landed_at=' + landed,
        navDiagFor('comment_box_not_found', 'composer', probeCommentGates(targetPostId), ctxInfo));
    }

    // Checkpoint 3 — pre-type. The editor we are about to type into
    // must still belong to the target article. This is the LAST line
    // of defence; once setEditableText runs, the comment composer
    // carries our content and a stray submit click would commit it.
    if (targetPostId) {
      const editorArticle = editor.closest('[role="article"], [role="dialog"]');
      if (!editorArticle || extractArticleCanonicalEntityId(editorArticle) !== targetPostId) {
        const landed = location.href || '';
        const editorScopeID = editorArticle ? extractArticleCanonicalEntityId(editorArticle) : '<no-enclosing-article>';
        console.warn('[THG outbound.executeComment] gate3 FAIL editor drift',
          { target_id: targetPostId, editor_scope_id: editorScopeID, landed_at_fail: landed });
        return commentResult(false, 'context_drift', null, ctx,
          'identity_gate_3_editor_drift: editor closest container canonical id != ' + abbreviate(targetPostId) + ' landed_at=' + landed,
          navDiagFor('gate3_editor_drift', 'typing', probeCommentGates(targetPostId), ctxInfo));
      }
    }

    // Identity locked. Only NOW do we type. PR8A: phase is now 'typing'.
    if (!setEditableText(editor, content)) {
      return commentResult(false, 'comment_text_insert_failed', null, ctx,
        'typing.insert_failed: target id=' + abbreviate(targetPostId || '<none>'),
        navDiagFor('typing_insert_failed', 'typing', probeCommentGates(targetPostId), ctxInfo));
    }
    const inserted = await waitFor(() => editorContainsContent(editor, content), 1800, 150);
    if (!inserted) {
      return commentResult(false, 'comment_text_not_confirmed', null, ctx,
        'typing.not_confirmed: editor did not hold our content within 1.8s · target id=' + abbreviate(targetPostId || '<none>'),
        navDiagFor('typing_not_confirmed', 'typing', probeCommentGates(targetPostId), ctxInfo));
    }

    // PR8A: text is in the composer — we are now at the SUBMIT phase. Only from
    // here on is an "after submit" diagnostic legitimate.
    const submitButtons = findSubmitButtons(editor, [commentButton]);
    for (const submit of submitButtons) {
      if (submit && clickLikeUser(submit)) {
        const cleared = await waitFor(() => !editorContainsContent(editor, content), 7000, 250);
        if (cleared) {
          // Settle delay — give the DOM a moment to render the new comment
          // node before we walk it for proof. Without this, the verifier
          // often misses the node and downgrades to optimistic_success.
          await wait(700);
          return commentResult(true, '', 'sent_comment_button', ctx, '',
            navDiagFor('post_submit', 'verify', probeCommentGates(targetPostId), ctxInfo));
        }
      }
      await wait(400);
    }

    if (pressEnter(editor)) {
      const cleared = await waitFor(() => !editorContainsContent(editor, content), 7000, 250);
      if (cleared) {
        await wait(700);
        return commentResult(true, '', 'sent_comment_enter', ctx, '',
          navDiagFor('post_submit', 'verify', probeCommentGates(targetPostId), ctxInfo));
      }
    }

    return commentResult(false,
      submitButtons.length ? 'comment_submit_not_confirmed' : 'comment_submit_not_found',
      null, ctx,
      'submit.not_cleared: composer still held our content after ' + submitButtons.length +
      ' submit-button click(s) + Enter · target id=' + abbreviate(targetPostId || '<none>'),
      navDiagFor('submit_not_cleared', 'submit', probeCommentGates(targetPostId), ctxInfo));
  }

  // abbreviate keeps identity-gate failure notes short. pfbid tokens run
  // ~60 chars; first 16 is enough to tell two entities apart in the
  // operator-replay UI without bloating evidence_json. Matches the
  // backend's abbreviateID convention so notes from the extension and
  // backend layers read uniformly.
  function abbreviate(id) {
    if (!id) return '<missing>';
    if (id.length <= 16) return id;
    return id.slice(0, 16) + '…';
  }

  // commentResult bundles the executor's verdict + the DOM proof the
  // backend's ClassifyExtensionReport consumes. ok=false routes through
  // the proof builder too so platform-reject banners (rate_limited /
  // blocked / redirected_feed) override the executor's generic error code.
  //
  // notes (optional, ok=false only): a short stage-specific reason
  // string for the operator-replay UI. Appended onto proof.notes after
  // the proof builder has set its own annotation, so platform-detected
  // failures (banner, redirect) still take precedence in the notes
  // ordering. Used by the identity-gate aborts in executeComment to
  // distinguish "no article on page" from "post-click swap" from
  // "editor drift" in the dashboard.
  function commentResult(ok, errorCode, detail, ctx, notes, navDiag) {
    const proof = THGContentProof ? THGContentProof.buildCommentProof({
      ok, errorCode, content: ctx.content, userID: ctx.userID, preCount: ctx.preCount, duplicate: ctx.duplicate
    }) : null;
    if (proof && notes) {
      proof.notes = proof.notes ? proof.notes + ' · ' + notes : notes;
    }
    // PR8A: attach the structured landing telemetry (persists to evidence_json).
    if (proof && navDiag) {
      proof.nav_diagnostic = navDiag;
    }
    // PR8C-Forensics: fold the content-script interaction timeline (+ the
    // MAIN-world pushState/stack correlation) into the diagnostic, then disarm.
    // Only when this executeComment call armed the recorder (executeCommentInFeed
    // / rung2 paths do not), so the snapshot belongs to this attempt.
    if (proof && globalThis.THGForensics && THGForensics.isArmed()) {
      try {
        const snap = THGForensics.snapshot();
        proof.nav_diagnostic = proof.nav_diagnostic || {};
        proof.nav_diagnostic.forensics = snap;
      } catch (_) { /* forensics must never break delivery */ }
      THGForensics.uninstall();
    }
    // PR8A: the pre-type landing gate is AUTHORITATIVE over the proof builder's
    // post-submit feedish heuristic. When the executor explicitly reports
    // target_not_reached, force that failure_reason — UNLESS the proof builder
    // detected a real platform banner (checkpoint / blocked / rate_limited),
    // which is more specific and keeps precedence. (redirected_feed is the
    // heuristic we override: pre-type, "bounced to feed" IS target_not_reached.)
    if (proof && errorCode === 'target_not_reached') {
      const banner = proof.failure_reason && proof.failure_reason !== 'redirected_feed';
      if (!banner) proof.failure_reason = 'target_not_reached';
    }
    // Echo the server-issued execution_id back so the backend's
    // terminal-state CAS in store.FinalizeOutboundAttempt can gate on
    // it. Without this, a callback from a re-claimed (lease-evicted)
    // execution would silently finalize a row that no longer belongs
    // to us, masking the SW-restart-then-re-execute duplicate bug.
    if (proof && ctx && ctx.executionId) {
      proof.execution_id = ctx.executionId;
    }
    const base = ok
      ? { ok: true, detail: detail || 'sent_comment' }
      : { ok: false, error: errorCode || 'comment_failed' };
    return proof ? { ...base, proof } : base;
  }

  async function executeInbox(content, executionId = '') {
    await dismissBlockingOverlays();
    const proof = THGContentProof || null;
    // Snapshot the last bubble pre-submit so the proof builder can detect
    // whether a NEW bubble appeared (vs. an existing one already matching
    // our text — the duplicate / idempotent case).
    const preBubbleHash = proof ? proof.snapshotLastBubble() : '';
    const ctx = { content, preBubbleHash, executionId };

    const messageKeys = ['message', 'messenger', 'send message', 'nhan tin'];
    const sendKeys = ['send', 'press enter to send', 'gui'];
    let editors = Array.from(document.querySelectorAll('[contenteditable="true"], textarea')).filter(visible);
    if (!editors.length) {
      const messageButton = Array.from(document.querySelectorAll('div[role="button"], button, a[role="button"]')).filter(visible)
        .find(el => hasAny(labelOf(el), messageKeys));
      if (!messageButton || !clickLikeUser(messageButton)) return inboxResult(false, 'message_button_not_found', null, ctx);
      await wait(1800);
      editors = Array.from(document.querySelectorAll('[contenteditable="true"], textarea')).filter(visible);
    }
    let editor = editors.find(el => hasAny(labelOf(el), messageKeys) || norm(el.getAttribute('role')) === 'textbox');
    if (!editor) editor = editors[editors.length - 1];
    if (!editor) return inboxResult(false, 'message_box_not_found', null, ctx);
    if (!setEditableText(editor, content)) return inboxResult(false, 'inbox_text_insert_failed', null, ctx);
    await wait(700);
    const scope = editor.closest('[role="dialog"], form, div[aria-label]') || document;
    const send = Array.from(scope.querySelectorAll('div[role="button"], button, [aria-label]')).filter(visible).find(el => {
      const label = labelOf(el);
      return hasAny(label, sendKeys) && el.getAttribute('aria-disabled') !== 'true' && !el.disabled;
    });
    if (!send || !clickLikeUser(send)) return inboxResult(false, 'inbox_submit_not_found', null, ctx);
    // Longer settle for bubble + timestamp to render — FB animates the
    // bubble in, and "Just now" copy can lag the bubble itself.
    await wait(1500);
    return inboxResult(true, '', 'sent_inbox_button', ctx);
  }

  function inboxResult(ok, errorCode, detail, ctx) {
    const proof = THGContentProof ? THGContentProof.buildInboxProof({
      ok, errorCode, content: ctx.content, preBubbleHash: ctx.preBubbleHash
    }) : null;
    if (proof && ctx && ctx.executionId) {
      proof.execution_id = ctx.executionId;
    }
    const base = ok
      ? { ok: true, detail: detail || 'sent_inbox' }
      : { ok: false, error: errorCode || 'inbox_failed' };
    return proof ? { ...base, proof } : base;
  }

  async function executePost(content, executionId = '') {
    await dismissBlockingOverlays();
    const composerKeys = ["what's on your mind", 'write something', 'create a public post', 'ban dang nghi gi', 'viet gi do'];
    const postKeys = ['post', 'dang'];
    const ctx = { content, executionId };
    const composer = Array.from(document.querySelectorAll('div[role="button"], button, textarea, [contenteditable="true"], [aria-label]'))
      .filter(visible)
      .find(el => hasAny(labelOf(el), composerKeys));
    if (!composer || !clickLikeUser(composer)) return postResult(false, 'post_composer_not_found', null, ctx);
    await wait(1500);
    const editors = Array.from(document.querySelectorAll('[contenteditable="true"], textarea')).filter(visible);
    let editor = editors.find(el => norm(el.getAttribute('role')) === 'textbox') || editors[editors.length - 1];
    if (!editor) return postResult(false, 'post_editor_not_found', null, ctx);
    if (!setEditableText(editor, content)) return postResult(false, 'post_text_insert_failed', null, ctx);
    await wait(900);
    const scope = editor.closest('[role="dialog"], form') || document;
    const postButton = Array.from(scope.querySelectorAll('div[role="button"], button, [aria-label]')).filter(visible).reverse().find(el => {
      const label = labelOf(el);
      return hasAny(label, postKeys) && !label.includes('comment') && !label.includes('cancel') &&
        el.getAttribute('aria-disabled') !== 'true' && !el.disabled;
    });
    if (!postButton || !clickLikeUser(postButton)) return postResult(false, 'post_submit_not_found', null, ctx);
    // Generous settle — posting closes the composer dialog and re-renders
    // the feed; we need both to complete before walking the DOM for proof.
    await wait(2500);
    return postResult(true, '', 'sent_post_button', ctx);
  }

  function postResult(ok, errorCode, detail, ctx) {
    const proof = THGContentProof ? THGContentProof.buildPostProof({
      ok, errorCode, content: ctx.content
    }) : null;
    if (proof && ctx && ctx.executionId) {
      proof.execution_id = ctx.executionId;
    }
    const base = ok
      ? { ok: true, detail: detail || 'sent_post' }
      : { ok: false, error: errorCode || 'post_failed' };
    return proof ? { ...base, proof } : base;
  }

  // executeCommentInFeed is Path 2's content-script entry point. The
  // background-side outbox flow (src/outbox.js::executeInGroupFeed) has
  // already navigated the tab to /groups/<g>/ (group home — a surface
  // proven non-redirected for account #49) and verified the URL. Now we
  // find the target post's article element IN THE FEED DOM by post_id,
  // then comment from feed context. We NEVER load /groups/<g>/posts/<p>/.
  //
  // Why this exists: the permalink-page path (executeComment above) keeps
  // landing on `/` because FB silently redirects the tab during the small
  // window between navigateAndVerify success and content-script handler
  // entry. User confirmed account #49 has no FB-side restriction (manual
  // commenting works) — so the redirect is triggered by our deep-link
  // automation pattern. Group home + feed-DOM article-find bypasses the
  // entire permalink surface, sidestepping the redirect entirely.
  //
  // Reuses existing utilities: findTargetArticle, articleIsReadyForComment,
  // waitUntilTargetArticleStable (with extended timeout for scroll
  // discovery), commentButton/editor/submit finders, gates 2 + 3, and
  // commentResult. The only NEW behaviour is scroll-then-search to locate
  // articles that are below the initial fold in the group's feed.
  //
  // Diagnostic taxonomy (notes-field prefix → matches Path 2 spec):
  //   path2.article_not_found_in_feed             — gate-1 (feed-aware) timed out
  //   path2.article_found_but_comment_button_missing — gate-1 passed, no Comment btn in scope
  //   path2.article_found_comment_opened_submit_failed — typed OK, submit click did not clear editor
  //   path2.comment_success                       — terminal success
  async function executeCommentInFeed(message) {
    const content = String(message?.content || '').trim();
    if (!content) return { ok: false, error: 'outbox_content_empty' };
    if (content.length > 3000) return { ok: false, error: 'outbox_content_too_long' };
    const targetUrl = String(message?.target_url || message?.targetUrl || '').trim();
    const executionId = String(message?.execution_id || message?.executionId || '').trim();
    const explicitPostId = String(message?.post_id || message?.postId || '').trim();
    const targetPostId = explicitPostId || extractPostIdFromUrl(targetUrl);
    if (!targetPostId) {
      return {
        ok: false,
        error: 'comment_target_not_post_permalink',
        proof: {
          success: false,
          failure_reason: 'context_drift',
          page_url_after: location.href || '',
          notes: 'path2.no_post_id: target_url=' + targetUrl,
          execution_id: executionId,
        },
      };
    }

    await dismissBlockingOverlays();
    const navAtEntry = location.href || '';
    console.log('[THG outbound.executeCommentInFeed] start',
      { target_url: targetUrl, target_post_id: targetPostId, landed_at_entry: navAtEntry, execution_id: executionId });

    const proof = THGContentProof || null;
    const fbUID = proof?.currentFBUserID() || '';
    const preCount = proof ? proof.snapshotCommentCount() : 0;
    const preMatched = proof ? !!proof.findCommentNode(content, fbUID) : false;
    const ctx = { content, userID: fbUID, preCount, duplicate: preMatched, executionId };

    // GATE-1 (feed-aware): scroll-then-wait until article materializes
    // and is stable. Group home renders posts incrementally via virtual
    // scroller; the lead's target post may be 5–30 posts deep. We
    // alternate short waitUntilTargetArticleStable attempts with scroll
    // pulses so FB's React mounts the article into DOM before we look.
    const MAX_SCROLLS = 8;
    const PER_ATTEMPT_TIMEOUT_MS = 2500;
    let targetScope = null;
    let scrollsDone = 0;
    for (let i = 0; i <= MAX_SCROLLS; i++) {
      targetScope = await waitUntilTargetArticleStable(targetPostId, {
        timeoutMs: PER_ATTEMPT_TIMEOUT_MS,
        stableMs: 500,
        pollMs: 200,
      });
      if (targetScope) break;
      if (i === MAX_SCROLLS) break;
      try {
        window.scrollBy({ top: 1800, behavior: 'instant' });
      } catch {
        window.scrollTo(0, window.scrollY + 1800);
      }
      scrollsDone++;
      await wait(700);
    }
    if (!targetScope) {
      const landed = location.href || '';
      const articlesSeen = document.querySelectorAll('[role="article"]').length;
      console.warn('[THG outbound.executeCommentInFeed] article_not_found_in_feed',
        { target_post_id: targetPostId, scrolls: scrollsDone, articles_in_dom: articlesSeen, landed });
      return commentResult(false, 'context_drift', null, ctx,
        'path2.article_not_found_in_feed: target id=' + abbreviate(targetPostId) +
        ' scrolls=' + scrollsDone +
        ' articles_in_dom=' + articlesSeen +
        ' nav_at_entry=' + navAtEntry +
        ' landed_at=' + landed +
        ' (article+permalink+comment-button stable 500ms never observed across ' +
        (MAX_SCROLLS + 1) + ' attempts of ' + PER_ATTEMPT_TIMEOUT_MS + 'ms each)');
    }

    // Scroll target into view so FB doesn't unmount it as we click.
    try { targetScope.scrollIntoView({ block: 'center', behavior: 'instant' }); } catch {}
    await wait(400);

    // Find Comment button inside the article scope (NOT document-wide
    // — feed has many articles, document-wide would click the wrong one).
    const commentKeys = ['comment', 'write a comment', 'binh luan', 'viet binh luan'];
    const buttons = Array.from(targetScope.querySelectorAll('div[role="button"], button, a[role="button"], span[role="button"]')).filter(visible);
    const commentButton = buttons.find(el => {
      const label = labelOf(el);
      return hasAny(label, commentKeys) && !label.includes('share') && !label.includes('like');
    });
    if (!commentButton) {
      const landed = location.href || '';
      console.warn('[THG outbound.executeCommentInFeed] article_found_but_comment_button_missing',
        { target_post_id: targetPostId, scanned_buttons: buttons.length, landed });
      return commentResult(false, 'comment_box_not_found', null, ctx,
        'path2.article_found_but_comment_button_missing: target id=' + abbreviate(targetPostId) +
        ' scanned_buttons=' + buttons.length +
        ' landed_at=' + landed +
        ' (article in feed but no Comment button matched label keys ' + JSON.stringify(commentKeys) + ')');
    }
    clickLikeUser(commentButton);
    await wait(900);

    // GATE-2 (post-click identity): click may have opened a modal (often
    // anchored to document.body) OR expanded inline. Either is fine, but
    // we must re-verify the resolved scope still belongs to targetPostId.
    let scope;
    const refreshed = findTargetArticle(targetPostId);
    scope = refreshed || targetScope;
    if (extractArticleCanonicalEntityId(scope) !== targetPostId) {
      const landed = location.href || '';
      console.warn('[THG outbound.executeCommentInFeed] gate2_swap',
        { target_post_id: targetPostId, scope_id: extractArticleCanonicalEntityId(scope), landed });
      return commentResult(false, 'context_drift', null, ctx,
        'path2.identity_gate_2_post_click_swap: scope canonical id != ' + abbreviate(targetPostId) +
        ' landed_at=' + landed);
    }

    let editor = findCommentEditor(scope);
    if (!editor) {
      // Scroll a touch + re-find. Same recovery executeComment uses.
      window.scrollBy({ top: 420, behavior: 'smooth' });
      await wait(900);
      const refreshed2 = findTargetArticle(targetPostId);
      scope = refreshed2 || targetScope;
      if (extractArticleCanonicalEntityId(scope) !== targetPostId) {
        const landed = location.href || '';
        return commentResult(false, 'context_drift', null, ctx,
          'path2.identity_gate_2b_scroll_swap: scope canonical id != ' + abbreviate(targetPostId) +
          ' landed_at=' + landed);
      }
      editor = findCommentEditor(scope);
    }
    if (!editor) {
      const landed = location.href || '';
      console.warn('[THG outbound.executeCommentInFeed] comment_box_not_found_post_click',
        { target_post_id: targetPostId, landed });
      return commentResult(false, 'comment_box_not_found', null, ctx,
        'path2.article_found_but_comment_button_missing: target id=' + abbreviate(targetPostId) +
        ' landed_at=' + landed +
        ' (Comment button clicked but no editable composer materialized in scope)');
    }

    // GATE-3 (pre-type editor scope): mirror executeComment's last-line-
    // of-defence check. Without this, a stray modal from a different
    // article could capture our text.
    const editorArticle = editor.closest('[role="article"], [role="dialog"]');
    if (!editorArticle || extractArticleCanonicalEntityId(editorArticle) !== targetPostId) {
      const landed = location.href || '';
      const editorScopeID = editorArticle ? extractArticleCanonicalEntityId(editorArticle) : '<no-enclosing-article>';
      console.warn('[THG outbound.executeCommentInFeed] gate3_editor_drift',
        { target_post_id: targetPostId, editor_scope_id: editorScopeID, landed });
      return commentResult(false, 'context_drift', null, ctx,
        'path2.identity_gate_3_editor_drift: editor closest container canonical id != ' + abbreviate(targetPostId) +
        ' landed_at=' + landed);
    }

    if (!setEditableText(editor, content)) {
      return commentResult(false, 'comment_text_insert_failed', null, ctx,
        'path2.comment_text_insert_failed: target id=' + abbreviate(targetPostId));
    }
    const inserted = await waitFor(() => editorContainsContent(editor, content), 1800, 150);
    if (!inserted) {
      return commentResult(false, 'comment_text_not_confirmed', null, ctx,
        'path2.comment_text_not_confirmed: target id=' + abbreviate(targetPostId));
    }

    const submitButtons = findSubmitButtons(editor, [commentButton]);
    for (const submit of submitButtons) {
      if (submit && clickLikeUser(submit)) {
        const cleared = await waitFor(() => !editorContainsContent(editor, content), 7000, 250);
        if (cleared) {
          await wait(700);
          return commentResult(true, '', 'sent_comment_button',
            { ...ctx, /* attach success note via 5th arg below */ },
            'path2.comment_success: target id=' + abbreviate(targetPostId) + ' via=submit_button');
        }
      }
      await wait(400);
    }

    if (pressEnter(editor)) {
      const cleared = await waitFor(() => !editorContainsContent(editor, content), 7000, 250);
      if (cleared) {
        await wait(700);
        return commentResult(true, '', 'sent_comment_enter', ctx,
          'path2.comment_success: target id=' + abbreviate(targetPostId) + ' via=enter_key');
      }
    }

    return commentResult(false,
      submitButtons.length ? 'comment_submit_not_confirmed' : 'comment_submit_not_found',
      null, ctx,
      'path2.article_found_comment_opened_submit_failed: target id=' + abbreviate(targetPostId) +
      ' submit_candidates=' + submitButtons.length +
      ' typed=' + content.length + 'chars' +
      ' (text inserted into editor but submit click + enter both did not clear editor within 7s)');
  }

  // probeRung2Click implements the content-script half of the Rung-2 probe.
  // From a stable, already-loaded FB page (home), it click-navigates toward the
  // target permalink the way a human clicking a link does — a genuine click on
  // an anchor whose href is the permalink, which FB's delegated client router
  // can intercept and turn into an in-SPA history.pushState (NOT a redirect-
  // eligible top-level load). It returns IMMEDIATELY after the click so the
  // background can measure the post-click URL trajectory (which survives a
  // top-load unload, unlike a content-script timer). No comment is typed.
  function probeRung2Click(message) {
    const targetUrl = String(message?.target_url || message?.targetUrl || '').trim();
    const targetId = extractPostIdFromUrl(targetUrl);
    const entry = location.href || '';
    let anchor = null;
    let method = '';
    if (targetId) {
      anchor = Array.from(document.querySelectorAll('a[href*="/posts/"], a[href*="/permalink/"], a[href*="story_fbid="]'))
        .find(el => String(el.getAttribute('href') || '').indexOf(targetId) !== -1) || null;
    }
    if (anchor) {
      method = 'existing_anchor';
    } else {
      anchor = document.createElement('a');
      anchor.href = targetUrl;
      anchor.setAttribute('role', 'link');
      anchor.textContent = 'thg-nav';
      anchor.style.cssText = 'position:fixed;left:8px;top:8px;width:12px;height:12px;opacity:0.01;z-index:2147483647;';
      document.body.appendChild(anchor);
      method = 'injected_anchor';
    }
    clickLikeUser(anchor);
    return { ok: true, clicked: true, method, entry_url: entry };
  }

  // executeCommentViaRung2 is the REAL delivery on the confirmed Rung-2
  // navigation. The probe proved a genuine anchor click → FB's in-SPA router
  // reaches AND holds on the permalink (no late redirect). So: from the stable
  // home page the content script started on, click-navigate to the permalink
  // (in-SPA, gesture-carrying — survives where chrome.tabs.update is bounced),
  // wait for the URL to land on the post, then hand off to the existing
  // permalink-page executor (executeComment), whose identity checkpoints +
  // proof are unchanged. The whole flow is visible in the user's tab.
  async function executeCommentViaRung2(message) {
    const content = String(message?.content || '').trim();
    if (!content) return { ok: false, error: 'outbox_content_empty' };
    if (content.length > 3000) return { ok: false, error: 'outbox_content_too_long' };
    const targetUrl = String(message?.target_url || message?.targetUrl || '').trim();
    const executionId = String(message?.execution_id || message?.executionId || '').trim();
    const targetId = extractPostIdFromUrl(targetUrl);

    // Progress logs (visible in the FB tab's DevTools Console) so we can see
    // exactly how far the flow got even if the background's response is lost.
    console.log('[THG rung2] start', { target_id: targetId, entry: location.href, execution_id: executionId });
    // Rung-2 in-SPA navigation: genuine anchor click → FB router pushState.
    const click = probeRung2Click({ target_url: targetUrl });
    console.log('[THG rung2] clicked', click);
    // Wait until the in-SPA nav lands on the permalink (URL carries the post id).
    const landed = await waitFor(
      () => !!targetId && (location.href || '').indexOf(targetId) !== -1,
      7000, 200
    );
    console.log('[THG rung2] nav landed=', landed, 'url=', location.href);
    if (!landed) {
      return {
        ok: false,
        error: 'nav_redirected',
        proof: {
          success: false,
          failure_reason: 'redirected_feed',
          page_url_after: location.href || '',
          notes: 'c.rung2.nav_did_not_land: target_id=' + (targetId || '?') +
            ' landed_at=' + (location.href || '') +
            ' (in-SPA click nav did not reach the permalink within 7s)',
          execution_id: executionId,
        },
      };
    }
    // Settle for the post + composer to render after the in-SPA route change.
    await wait(900);
    // Hand off to the permalink-page executor (gate-1 confirms the article,
    // identity checkpoints + proof unchanged).
    console.log('[THG rung2] handing off to executeComment on', location.href);
    const r = await executeComment(content, targetUrl, executionId);
    console.log('[THG rung2] executeComment result', r && (r.ok ? 'OK' : (r.error || (r.proof && r.proof.failure_reason))), r && r.proof && r.proof.notes);
    return r;
  }

  async function executeOutbound(message) {
    const content = String(message?.content || '').trim();
    if (!content) return { ok: false, error: 'outbox_content_empty' };
    if (content.length > 3000) return { ok: false, error: 'outbox_content_too_long' };
    const type = String(message?.type || '').trim().toLowerCase();
    // target_url is the SAME field outbox.js navigates the tab to via
    // chrome.tabs.update. Surfacing it to the comment executor lets us
    // pin the DOM search to the exact post the queue intended, instead
    // of the first comment button visible on the SPA-rendered page.
    const targetUrl = String(message?.target_url || message?.targetUrl || '').trim();
    // execution_id is the server-issued idempotency token. We do NOT
    // mutate it here; we just thread it through. The proof builder in
    // commentResult attaches it to proof.execution_id so the eventual
    // /sent or /failed POST body echoes it. Backend's
    // FinalizeOutboundAttempt CAS requires this to match the row's
    // current execution_id; replays and re-claim collisions are
    // rejected there.
    const executionId = String(message?.execution_id || message?.executionId || '').trim();
    // PR8A: thread the executing account id + the background navigation trace
    // (from/to/duration, attached by src/outbox.js before sendMessage) into the
    // comment executor so the NavDiagnostic it builds is complete.
    const navOpts = {
      accountId: Number(message?.account_id || message?.accountId || 0) || 0,
      navTrace: message?.nav_trace || null,
    };
    if (type === 'comment') return executeComment(content, targetUrl, executionId, navOpts);
    if (type === 'inbox') return executeInbox(content, executionId);
    if (type === 'group_post' || type === 'profile_post') return executePost(content, executionId);
    return { ok: false, error: `unsupported_outbox_type:${type}` };
  }

  return { executeOutbound, executeCommentInFeed, probeRung2Click, executeCommentViaRung2 };
})();
globalThis.THGContentOutbound = THGContentOutbound;
