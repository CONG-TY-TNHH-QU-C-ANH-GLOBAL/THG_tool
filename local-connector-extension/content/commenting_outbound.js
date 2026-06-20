// THGCommentingOutbound — comment EXECUTORS (executeComment / executeCommentInFeed /
// executeCommentViaRung2 / probeRung2Click) + commentResult, extracted verbatim from
// outbound.js (Workstream A · PR5): move-only, behavior-preserving. Largest legacy comment
// surface (executeComment alone is ~300 lines) — TEMPORARILY allowlisted for file-size; a
// follow-up behavior-PR will split the big functions with tests. Consumes THGOutboundDom,
// THGCommentingTarget, THGCommentingDiag, comment constants, and the comment siblings
// (THGContentProof / THGForensics / THGCommentButton / THGCommentSM) read at call time as
// bare globals — call-time reads preserved exactly. Chrome: globalThis.THGCommentingOutbound
// (loaded after commenting_diag.js, before outbound.js); Node: module.exports (+ _test).
globalThis.THGCommentingOutbound = globalThis.THGCommentingOutbound || (() => {
  const THGDom = globalThis.THGOutboundDom
    || (typeof require === 'function' ? require('./outbound_dom.js') : null);
  if (!THGDom) {
    throw new Error('THGOutboundDom is required before commenting_outbound.js');
  }
  const { wait, norm, hasAny, visible, labelOf, clickLikeUser, enabledButton,
    textOfEditable, waitFor, dismissBlockingOverlays } = THGDom;
  const K = globalThis.THGCommentConstants
    || (typeof require === 'undefined' ? null : require('./comment_constants.js'));
  const THGTarget = globalThis.THGCommentingTarget
    || (typeof require === 'function' ? require('./commenting_target.js') : null);
  if (!THGTarget) {
    throw new Error('THGCommentingTarget is required before commenting_outbound.js');
  }
  const THGDiag = globalThis.THGCommentingDiag
    || (typeof require === 'function' ? require('./commenting_diag.js') : null);
  if (!THGDiag) {
    throw new Error('THGCommentingDiag is required before commenting_outbound.js');
  }
  const { acquireTargetComposer, articleIsReadyForComment, commentSurfaceDeps, discoverDeps,
    extractArticleCanonicalEntityId, extractPostIdFromUrl, findCommentEditor, findComposerForTarget,
    findTargetArticle, isCreatePostComposer, onTargetPermalinkPage, waitUntilTargetArticleStable } = THGTarget;
  const { probeCommentGates, navDiagFor } = THGDiag;
  // Debug-gated swallow for best-effort browser calls (silent at normal runtime).
  function ignoreErr(e, ctx) { if (globalThis.__THG_COMMENTING_DEBUG__) console.debug(`[THGCommentingOutbound] ${ctx}`, e); }

  function editorContainsContent(editor, content) {
    if (!editor || !document.contains(editor)) return false;
    const current = norm(textOfEditable(editor)).replace(/\s+/g, ' ');
    const expected = norm(content).replace(/\s+/g, ' ');
    if (!expected) return false;
    const sample = expected.slice(0, Math.min(60, expected.length));
    return current.includes(sample);
  }

  // submitDeps threads outbound.js's shared DOM predicates into the extracted
  // THGCommentSubmit module (which owns findSubmitButtons / pressEnter / the reject
  // list) — keeps those primitives DRY without polluting globals.
  const submitDeps = { labelOf, norm, hasAny, visible, enabledButton };

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
    // permalinkPage: the executor navigated to the target post's OWN permalink, so
    // a page-level composer (outside the post's [role=article]) is unambiguously
    // the target's — see onTargetPermalinkPage. Used to accept the permalink
    // layout where comments are pre-expanded and there is no in-article composer.
    const permalinkPage = onTargetPermalinkPage(targetPostId);
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
      if (!targetScope && permalinkPage && findComposerForTarget(targetPostId)) {
        // PERMALINK LAYOUT: the post is present but its comments are already
        // expanded (no in-article Comment button) and the composer lives at page
        // level. waitUntilTargetArticleStable never sees an in-article composer
        // and times out — yet the URL confirms we are on the target post's own
        // page, so the page-level composer is the target's. Proceed: keep the
        // article (if any) as scope for the button search; the editor search and
        // Checkpoint-3 below fall back to the page-level composer for this case.
        targetScope = findTargetArticle(targetPostId) || document;
      }
      if (!targetScope) {
        // Gate1 fallback (PR-B): the stability poll never caught the comment surface, but
        // the post may be present with a lazily-mounted action row. Scroll the article into
        // view and retry the broadened discovery before giving up.
        const art = findTargetArticle(targetPostId);
        if (art) {
          const disc = await THGCommentButton.discoverCommentSurface(art, discoverDeps(targetPostId));
          if (disc.found) targetScope = art;
        }
      }
      if (!targetScope) {
        const landed = location.href || '';
        // PR8A landing gate FAILED. Probe BUTTON + COMPOSER under the target article and
        // classify: reached the post but neither a comment button NOR a visible composer →
        // comment_button_not_found (a discovery miss — retryable, no risk; NOT "post can't be
        // commented"). Otherwise the post was never reached → target_not_reached.
        const probe = probeCommentGates(targetPostId);
        const art = findTargetArticle(targetPostId);
        const ent = art ? THGCommentButton.diagnostics(art, commentSurfaceDeps(targetPostId)) : { comment_button_found: false, composer_entry_found: false, textbox_candidates_count: 0, contenteditable_candidates_count: 0, gate1_passed_via: 'none', composer_candidates: [] };
        const candReasons = (ent.composer_candidates || []).map((cand) => cand.reason + (cand.accepted ? '(ok)' : '')).join(',');
        const gates = {
          articleFound: probe.articleFound, permalinkFound: probe.permalinkFound,
          commentButtonFound: ent.comment_button_found, composerEntryFound: ent.composer_entry_found,
        };
        const reason = THGCommentButton.classifyGate1Failure(gates);
        const navDiag = navDiagFor('gate1_' + reason, 'gate1', gates, ctxInfo);
        const rc = navDiag && navDiag.redirect_class ? navDiag.redirect_class : 'unknown';
        console.warn('[THG outbound.executeComment] gate1 FAIL ' + reason,
          { target_url: targetUrl, target_id: targetPostId, landed_at_fail: landed,
            nav_at_entry: navAtEntry, redirect_class: rc, gates,
            // P0 #7: per-candidate composer reasons + shape counts at top level so a future
            // UI-drift can be diagnosed from the console object alone (no full-HTML dump).
            gate1_passed_via: ent.gate1_passed_via,
            composer_candidates: ent.composer_candidates,
            textbox_candidates_count: ent.textbox_candidates_count,
            contenteditable_candidates_count: ent.contenteditable_candidates_count,
            entry: ent });
        return commentResult(false, reason, null, ctx,
          'identity_gate_1_' + reason + ': target id=' + abbreviate(targetPostId) +
          ' redirect_class=' + rc +
          ' article_found=' + gates.articleFound +
          ' permalink_found=' + gates.permalinkFound +
          ' comment_button_found=' + gates.commentButtonFound +
          ' composer_entry_found=' + gates.composerEntryFound +
          ' textbox_candidates=' + ent.textbox_candidates_count +
          ' contenteditable_candidates=' + ent.contenteditable_candidates_count +
          ' gate1_passed_via=' + ent.gate1_passed_via +
          ' composer_candidate_reasons=[' + candReasons + ']' +
          ' landed_at=' + landed + ' nav_at_entry=' + navAtEntry +
          ' did not settle (article+permalink+comment-entry) within fallback window',
          navDiag);
      }
    }
    const searchRoot = targetScope || document;

    const commentKeys = K.COMMENT_KEYS;
    const buttons = Array.from(searchRoot.querySelectorAll('div[role="button"], button, a[role="button"], span[role="button"]')).filter(el => visible(el));
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

    // Editor acquisition uses acquireTargetComposer (the SAME classifier gate1 accepted with)
    // as the source of truth; the legacy in-article / bounded-walk finders are only extra
    // fallbacks. This unifies gate1 discovery and editor selection — the P0b handoff fix.
    let acq = acquireTargetComposer(targetPostId, scope);
    let editor = acq.el || findCommentEditor(scope) || (permalinkPage ? findComposerForTarget(targetPostId) : null);
    if (!editor) {
      globalThis.scrollBy({ top: 420, behavior: 'smooth' });
      await wait(900);
      if (targetPostId) {
        const refreshed = findTargetArticle(targetPostId);
        scope = refreshed || targetScope;
        // Re-verify after the scroll — article references may have gone
        // stale, and the React tree may have rotated. On the target's own
        // permalink page the URL already pins identity, so the in-article
        // re-check is skipped (the composer is legitimately page-level there).
        if (!permalinkPage && extractArticleCanonicalEntityId(scope) !== targetPostId) {
          const landed = location.href || '';
          console.warn('[THG outbound.executeComment] gate2b FAIL scroll swap',
            { target_id: targetPostId, scope_id: extractArticleCanonicalEntityId(scope), landed_at_fail: landed });
          return commentResult(false, 'context_drift', null, ctx,
            'identity_gate_2b_scroll_swap: scope canonical id != ' + abbreviate(targetPostId) + ' landed_at=' + landed,
            navDiagFor('gate2b_scroll_swap', 'composer', probeCommentGates(targetPostId), ctxInfo));
        }
        acq = acquireTargetComposer(targetPostId, scope);
        editor = acq.el || findCommentEditor(scope) || (permalinkPage ? findComposerForTarget(targetPostId) : null);
      } else {
        editor = findCommentEditor(scope) || findCommentEditor(document);
      }
    }
    if (!editor) {
      const landed = location.href || '';
      // P0b diagnostics: prove EXACTLY why the visible composer was not used. Re-run the gate1
      // classifier snapshot + surface every editor candidate with its accept/reject reason, so
      // a comment_box_not_found is never opaque again.
      const artD = targetPostId ? findTargetArticle(targetPostId) : null;
      const entD = artD ? THGCommentButton.diagnostics(artD, commentSurfaceDeps(targetPostId)) : null;
      const cands = (acq && acq.candidates && acq.candidates.length ? acq.candidates : (entD ? entD.composer_candidates : [])) || [];
      const candReasons = cands.map((c) => c.reason + (c.accepted ? '(ok)' : '')).join(',');
      const fcft = !!findComposerForTarget(targetPostId);
      console.warn('[THG outbound.executeComment] comment_box_not_found',
        { target_id: targetPostId, landed_at_fail: landed,
          urlPinsIdentity: onTargetPermalinkPage(targetPostId),
          gate1_passed_via: entD ? entD.gate1_passed_via : 'unknown',
          findComposerForTarget_found: fcft,
          editor_candidates_count: cands.length, editor_candidates: cands,
          acquire_reason: acq ? acq.reason : 'none' });
      return commentResult(false, 'comment_box_not_found', null, ctx,
        'comment_box_not_found: target id=' + abbreviate(targetPostId || '<none>') +
        ' urlPinsIdentity=' + onTargetPermalinkPage(targetPostId) +
        ' gate1_passed_via=' + (entD ? entD.gate1_passed_via : 'unknown') +
        ' acquire_reason=' + (acq ? acq.reason : 'none') +
        ' findComposerForTarget_found=' + fcft +
        ' editor_candidates=' + cands.length +
        ' editor_candidate_reasons=[' + candReasons + ']' +
        ' landed_at=' + landed,
        navDiagFor('comment_box_not_found', 'composer', probeCommentGates(targetPostId), ctxInfo));
    }

    // Checkpoint 3 — pre-type. The editor we are about to type into
    // must still belong to the target article. This is the LAST line
    // of defence; once setEditableText runs, the comment composer
    // carries our content and a stray submit click would commit it.
    if (targetPostId) {
      const editorArticle = editor.closest('[role="article"], [role="dialog"]');
      const editorScopeID = editorArticle ? extractArticleCanonicalEntityId(editorArticle) : '';
      const inTargetArticle = editorScopeID === targetPostId;
      // On the target post's OWN permalink page the URL pins identity: the page renders ONE
      // top-level post, so an editor whose enclosing [role=article] extracts a DIFFERENT id
      // is a nested comment/answer item (the live "Write an answer…" 204 case), NOT a wrong
      // post. Accept it — the only box we must still positively reject there is the GLOBAL
      // create-post composer. On FEED pages (multiple real posts) the strict in-target-article
      // requirement remains the route-mismatch guard (May-2026 incident).
      const ok = inTargetArticle || (permalinkPage && !isCreatePostComposer(editor));
      if (!ok) {
        const landed = location.href || '';
        console.warn('[THG outbound.executeComment] gate3 FAIL editor drift',
          { target_id: targetPostId, editor_scope_id: editorScopeID || '<no-enclosing-article>', permalink_page: permalinkPage, landed_at_fail: landed });
        return commentResult(false, 'context_drift', null, ctx,
          'identity_gate_3_editor_drift: editor closest container canonical id=' + (editorScopeID || '<none>') + ' != ' + abbreviate(targetPostId) + ' landed_at=' + landed,
          navDiagFor('gate3_editor_drift', 'typing', probeCommentGates(targetPostId), ctxInfo));
      }
    }

    // Identity locked. The composer→submit STATE MACHINE (THGCommentSM) owns the
    // whole "clear → insert → assert exactly equals → submit (re-asserting before
    // each click) → composer-cleared" path, shared with the group-feed executor, so
    // no comment can submit a doubled/mismatched composer.
    const sm = await THGCommentSM.runComposerToSubmit(editor, content, commentButton, {
      executorPath: permalinkPage ? 'permalink_page' : 'permalink_article',
      outboundId: opts.outboundId || 0,
      clickLikeUser, editorContainsContent, waitFor, wait, now: () => Date.now(), submitDeps,
    });
    if (!sm.ok) {
      return commentResult(false, sm.reason, null, ctx,
        'sm.' + sm.reason + ': ' + JSON.stringify(sm.diagnostic) + ' · target id=' + abbreviate(targetPostId || '<none>'),
        navDiagFor(sm.reason, sm.diagnostic.phase || 'submit', probeCommentGates(targetPostId), ctxInfo));
    }
    // Submit accepted (composer cleared). Settle so the proof verifier sees the new node.
    await wait(700);
    return commentResult(true, '', 'sent_comment', ctx, 'sm.sent: ' + JSON.stringify(sm.diagnostic),
      navDiagFor('post_submit', 'verify', probeCommentGates(targetPostId), ctxInfo));
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
      } catch (e) { ignoreErr(e, 'forensics'); } // forensics must never break delivery
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

  // PR4: executeInbox + inboxResult moved to content/inbox_outbound.js (THGInboxOutbound).
  // PR3: executePost + postResult moved to content/posting_outbound.js (THGPostingOutbound).

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
        globalThis.scrollBy({ top: 1800, behavior: 'instant' });
      } catch {
        globalThis.scrollTo(0, globalThis.scrollY + 1800);
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
    try { targetScope.scrollIntoView({ block: 'center', behavior: 'instant' }); } catch { /* scroll is best-effort; ignore */ }
    await wait(400);

    // Find Comment button inside the article scope (NOT document-wide
    // — feed has many articles, document-wide would click the wrong one).
    const commentKeys = K.COMMENT_KEYS;
    const buttons = Array.from(targetScope.querySelectorAll('div[role="button"], button, a[role="button"], span[role="button"]')).filter(el => visible(el));
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
      globalThis.scrollBy({ top: 420, behavior: 'smooth' });
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

    // Same composer→submit STATE MACHINE as executeComment — one shared path, so the
    // group-feed executor can never diverge or submit a doubled/mismatched composer.
    const sm = await THGCommentSM.runComposerToSubmit(editor, content, commentButton, {
      executorPath: 'group_feed',
      outboundId: Number(message?.id || message?.outbound_id || 0) || 0,
      clickLikeUser, editorContainsContent, waitFor, wait, now: () => Date.now(), submitDeps,
    });
    if (!sm.ok) {
      return commentResult(false, sm.reason, null, ctx,
        'path2.' + sm.reason + ': ' + JSON.stringify(sm.diagnostic) + ' · target id=' + abbreviate(targetPostId));
    }
    await wait(700);
    return commentResult(true, '', 'sent_comment', ctx,
      'path2.comment_success: target id=' + abbreviate(targetPostId) + ' · ' + JSON.stringify(sm.diagnostic));
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

  const api = { executeComment, executeCommentInFeed, executeCommentViaRung2, probeRung2Click };
  if (typeof module !== 'undefined' && module.exports) {
    module.exports = { ...api, _test: { commentResult, abbreviate, editorContainsContent } };
  }
  return api;
})();
