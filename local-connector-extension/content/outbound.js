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

  async function dismissBlockingOverlays() {
    const labels = ['not now', 'ok', 'close', 'later', 'maybe later', 'remember password', 'de sau', 'luc khac', 'khong phai bay gio'];
    const candidates = Array.from(document.querySelectorAll('div[role="button"], button, [aria-label]')).filter(visible);
    for (const el of candidates) {
      const label = labelOf(el);
      if (!label) continue;
      if (labels.some(key => label.includes(key))) {
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

  async function executeComment(content, targetUrl = '', executionId = '') {
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
        console.warn('[THG outbound.executeComment] gate1 FAIL',
          { target_url: targetUrl, target_id: targetPostId, landed_at_fail: landed, nav_at_entry: navAtEntry });
        return commentResult(false, 'context_drift', null, ctx,
          'identity_gate_1_no_article_or_unstable: target id=' + abbreviate(targetPostId) +
          ' landed_at=' + landed + ' nav_at_entry=' + navAtEntry +
          ' did not settle (article+permalink+comment-button stable 500ms) within 8s');
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
          'identity_gate_2_post_click_swap: scope canonical id != ' + abbreviate(targetPostId) + ' landed_at=' + landed);
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
            'identity_gate_2b_scroll_swap: scope canonical id != ' + abbreviate(targetPostId) + ' landed_at=' + landed);
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
        'comment_box_not_found: target id=' + abbreviate(targetPostId || '<none>') + ' landed_at=' + landed);
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
          'identity_gate_3_editor_drift: editor closest container canonical id != ' + abbreviate(targetPostId) + ' landed_at=' + landed);
      }
    }

    // Identity locked. Only NOW do we type.
    if (!setEditableText(editor, content)) return commentResult(false, 'comment_text_insert_failed', null, ctx);
    const inserted = await waitFor(() => editorContainsContent(editor, content), 1800, 150);
    if (!inserted) return commentResult(false, 'comment_text_not_confirmed', null, ctx);

    const submitButtons = findSubmitButtons(editor, [commentButton]);
    for (const submit of submitButtons) {
      if (submit && clickLikeUser(submit)) {
        const cleared = await waitFor(() => !editorContainsContent(editor, content), 7000, 250);
        if (cleared) {
          // Settle delay — give the DOM a moment to render the new comment
          // node before we walk it for proof. Without this, the verifier
          // often misses the node and downgrades to optimistic_success.
          await wait(700);
          return commentResult(true, '', 'sent_comment_button', ctx);
        }
      }
      await wait(400);
    }

    if (pressEnter(editor)) {
      const cleared = await waitFor(() => !editorContainsContent(editor, content), 7000, 250);
      if (cleared) {
        await wait(700);
        return commentResult(true, '', 'sent_comment_enter', ctx);
      }
    }

    return commentResult(false, submitButtons.length ? 'comment_submit_not_confirmed' : 'comment_submit_not_found', null, ctx);
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
  function commentResult(ok, errorCode, detail, ctx, notes) {
    const proof = THGContentProof ? THGContentProof.buildCommentProof({
      ok, errorCode, content: ctx.content, userID: ctx.userID, preCount: ctx.preCount, duplicate: ctx.duplicate
    }) : null;
    if (proof && notes) {
      proof.notes = proof.notes ? proof.notes + ' · ' + notes : notes;
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
    if (type === 'comment') return executeComment(content, targetUrl, executionId);
    if (type === 'inbox') return executeInbox(content, executionId);
    if (type === 'group_post' || type === 'profile_post') return executePost(content, executionId);
    return { ok: false, error: `unsupported_outbox_type:${type}` };
  }

  return { executeOutbound };
})();
globalThis.THGContentOutbound = THGContentOutbound;
