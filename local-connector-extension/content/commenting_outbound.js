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
  const { acquireTargetComposer, commentSurfaceDeps, discoverDeps,
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

  // gate1Failure builds the PR8A landing-gate-1 failure diagnostic + commentResult. Pure
  // diagnostic/abort (no DOM mutation): probes the post + composer and classifies the miss.
  function gate1Failure(targetPostId, navAtEntry, targetUrl, ctx, ctxInfo) {
    const landed = location.href || '';
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
    const rc = navDiag?.redirect_class || 'unknown';
    console.warn('[THG outbound.executeComment] gate1 FAIL ' + reason,
      { target_url: targetUrl, target_id: targetPostId, landed_at_fail: landed,
        nav_at_entry: navAtEntry, redirect_class: rc, gates,
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

  // locateTargetScope runs the gate-1 stability/fallback block (Checkpoint 1). Returns
  // { scope } on success or { fail } with the gate-1 failure result. targetPostId empty
  // (legacy callers) is a no-op returning { scope: null } so the document-wide search applies.
  async function locateTargetScope(targetPostId, permalinkPage, navAtEntry, targetUrl, ctx, ctxInfo) {
    if (!targetPostId) return { scope: null };
    let targetScope = await waitUntilTargetArticleStable(targetPostId, { timeoutMs: 8000, stableMs: 500, pollMs: 200 });
    if (!targetScope && permalinkPage && findComposerForTarget(targetPostId)) {
      // PERMALINK LAYOUT: comments pre-expanded, composer at page level; URL pins identity.
      targetScope = findTargetArticle(targetPostId) || document;
    }
    if (!targetScope) {
      // Gate1 fallback (PR-B): scroll the article into view + retry broadened discovery.
      const art = findTargetArticle(targetPostId);
      if (art) {
        const disc = await THGCommentButton.discoverCommentSurface(art, discoverDeps(targetPostId));
        if (disc.found) targetScope = art;
      }
    }
    if (!targetScope) return { fail: gate1Failure(targetPostId, navAtEntry, targetUrl, ctx, ctxInfo) };
    return { scope: targetScope };
  }

  // findCommentButtonIn returns the visible Comment/Bình luận button in scope, or undefined.
  function findCommentButtonIn(searchRoot) {
    const commentKeys = K.COMMENT_KEYS;
    const buttons = Array.from(searchRoot.querySelectorAll('div[role="button"], button, a[role="button"], span[role="button"]')).filter(el => visible(el));
    return buttons.find(el => {
      const label = labelOf(el);
      return hasAny(label, commentKeys) && !label.includes('share') && !label.includes('like');
    });
  }

  // gate2ResolveScope — Checkpoint 2 post-click identity. Returns { scope } or { fail }.
  function gate2ResolveScope(targetPostId, targetScope, commentButton, ctx, ctxInfo) {
    if (!targetPostId) {
      return { scope: commentButton?.closest('[role="article"], [role="dialog"]') || document };
    }
    const refreshed = findTargetArticle(targetPostId);
    const scope = refreshed || targetScope;
    if (extractArticleCanonicalEntityId(scope) !== targetPostId) {
      const landed = location.href || '';
      console.warn('[THG outbound.executeComment] gate2 FAIL post-click swap',
        { target_id: targetPostId, scope_id: extractArticleCanonicalEntityId(scope), landed_at_fail: landed });
      return { fail: commentResult(false, 'context_drift', null, ctx,
        'identity_gate_2_post_click_swap: scope canonical id != ' + abbreviate(targetPostId) + ' landed_at=' + landed,
        navDiagFor('gate2_post_click_swap', 'composer', probeCommentGates(targetPostId), ctxInfo)) };
    }
    return { scope };
  }

  // commentBoxNotFound — P0b diagnostic + result when no editor materialised.
  function commentBoxNotFound(targetPostId, acq, ctx, ctxInfo) {
    const landed = location.href || '';
    const artD = targetPostId ? findTargetArticle(targetPostId) : null;
    const entD = artD ? THGCommentButton.diagnostics(artD, commentSurfaceDeps(targetPostId)) : null;
    let cands = [];
    if (acq?.candidates?.length) cands = acq.candidates;
    else if (entD?.composer_candidates) cands = entD.composer_candidates;
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

  // acquireCommentEditor — Checkpoint-2 scope + editor acquisition (with scroll retry / gate-2b
  // re-verify). Returns { editor, scope } or { fail }.
  async function acquireCommentEditor(targetPostId, permalinkPage, targetScope, commentButton, ctx, ctxInfo) {
    const g2 = gate2ResolveScope(targetPostId, targetScope, commentButton, ctx, ctxInfo);
    if (g2.fail) return { fail: g2.fail };
    let scope = g2.scope;
    let acq = acquireTargetComposer(targetPostId, scope);
    let editor = acq.el || findCommentEditor(scope) || (permalinkPage ? findComposerForTarget(targetPostId) : null);
    if (!editor) {
      globalThis.scrollBy({ top: 420, behavior: 'smooth' });
      await wait(900);
      if (targetPostId) {
        const refreshed = findTargetArticle(targetPostId);
        scope = refreshed || targetScope;
        if (!permalinkPage && extractArticleCanonicalEntityId(scope) !== targetPostId) {
          const landed = location.href || '';
          console.warn('[THG outbound.executeComment] gate2b FAIL scroll swap',
            { target_id: targetPostId, scope_id: extractArticleCanonicalEntityId(scope), landed_at_fail: landed });
          return { fail: commentResult(false, 'context_drift', null, ctx,
            'identity_gate_2b_scroll_swap: scope canonical id != ' + abbreviate(targetPostId) + ' landed_at=' + landed,
            navDiagFor('gate2b_scroll_swap', 'composer', probeCommentGates(targetPostId), ctxInfo)) };
        }
        acq = acquireTargetComposer(targetPostId, scope);
        editor = acq.el || findCommentEditor(scope) || (permalinkPage ? findComposerForTarget(targetPostId) : null);
      } else {
        editor = findCommentEditor(scope) || findCommentEditor(document);
      }
    }
    if (!editor) return { fail: commentBoxNotFound(targetPostId, acq, ctx, ctxInfo) };
    return { editor, scope };
  }

  // checkEditorGate3 — Checkpoint 3 pre-type editor-drift guard. Returns a fail result or null.
  function checkEditorGate3(editor, targetPostId, permalinkPage, ctx, ctxInfo) {
    if (!targetPostId) return null;
    const editorArticle = editor.closest('[role="article"], [role="dialog"]');
    const editorScopeID = editorArticle ? extractArticleCanonicalEntityId(editorArticle) : '';
    const inTargetArticle = editorScopeID === targetPostId;
    // On the target's OWN permalink page the URL pins identity, so a foreign-id enclosing
    // article is a nested comment/answer item — accept unless it is the create-post box.
    const ok = inTargetArticle || (permalinkPage && !isCreatePostComposer(editor));
    if (ok) return null;
    const landed = location.href || '';
    console.warn('[THG outbound.executeComment] gate3 FAIL editor drift',
      { target_id: targetPostId, editor_scope_id: editorScopeID || '<no-enclosing-article>', permalink_page: permalinkPage, landed_at_fail: landed });
    return commentResult(false, 'context_drift', null, ctx,
      'identity_gate_3_editor_drift: editor closest container canonical id=' + (editorScopeID || '<none>') + ' != ' + abbreviate(targetPostId) + ' landed_at=' + landed,
      navDiagFor('gate3_editor_drift', 'typing', probeCommentGates(targetPostId), ctxInfo));
  }

  // submitCommentViaSM — runs the shared composer->submit state machine, then builds the result.
  async function submitCommentViaSM(args) {
    const { editor, content, commentButton, permalinkPage, opts, ctx, targetPostId, ctxInfo } = args;
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

  // executeComment orchestrates the three identity checkpoints + submit. Each phase helper is
  // behavior-preserving (verbatim logic) and aborts via commentResult; identity gates intact.
  async function executeComment(content, targetUrl = '', executionId = '', opts = {}) {
    if (globalThis.THGForensics) THGForensics.install();
    await dismissBlockingOverlays();
    const navAtEntry = location.href || '';
    console.log('[THG outbound.executeComment] start',
      { target_url: targetUrl, landed_at_entry: navAtEntry, execution_id: executionId });
    const proof = THGContentProof || null;
    const fbUID = proof?.currentFBUserID() || '';
    const preCount = proof ? proof.snapshotCommentCount() : 0;
    const preMatched = proof ? !!proof.findCommentNode(content, fbUID) : false;
    const ctx = { content, userID: fbUID, preCount, duplicate: preMatched, executionId };
    const targetPostId = extractPostIdFromUrl(targetUrl);
    const ctxInfo = {
      targetUrl, targetPostId, fbUID,
      accountId: Number(opts.accountId || 0) || 0,
      navTrace: opts.navTrace || null,
    };
    const permalinkPage = onTargetPermalinkPage(targetPostId);

    const located = await locateTargetScope(targetPostId, permalinkPage, navAtEntry, targetUrl, ctx, ctxInfo);
    if (located.fail) return located.fail;
    const targetScope = located.scope;

    const searchRoot = targetScope || document;
    const commentButton = findCommentButtonIn(searchRoot);
    if (commentButton) {
      clickLikeUser(commentButton);
      await wait(900);
    }

    const acquired = await acquireCommentEditor(targetPostId, permalinkPage, targetScope, commentButton, ctx, ctxInfo);
    if (acquired.fail) return acquired.fail;
    const editor = acquired.editor;

    const drift = checkEditorGate3(editor, targetPostId, permalinkPage, ctx, ctxInfo);
    if (drift) return drift;

    return submitCommentViaSM({ editor, content, commentButton, permalinkPage, opts, ctx, targetPostId, ctxInfo });
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
  // PR8C-Forensics: fold the content-script interaction timeline into the diagnostic, then
  // disarm. Only when this executeComment call armed the recorder (feed/rung2 paths do not).
  function foldForensicsInto(proof) {
    if (!(proof && globalThis.THGForensics && THGForensics.isArmed())) return;
    try {
      const snap = THGForensics.snapshot();
      proof.nav_diagnostic = proof.nav_diagnostic || {};
      proof.nav_diagnostic.forensics = snap;
    } catch (e) { ignoreErr(e, 'forensics'); } // forensics must never break delivery
    THGForensics.uninstall();
  }

  // PR8A: the pre-type landing gate is AUTHORITATIVE over the proof builder's post-submit
  // feedish heuristic — force target_not_reached UNLESS a real platform banner was detected
  // (more specific; keeps precedence). redirected_feed is the heuristic we override.
  function applyTargetNotReachedOverride(proof, errorCode) {
    if (!(proof && errorCode === 'target_not_reached')) return;
    const banner = proof.failure_reason && proof.failure_reason !== 'redirected_feed';
    if (!banner) proof.failure_reason = 'target_not_reached';
  }

  function commentResult(ok, errorCode, detail, ctx, notes, navDiag) {
    const proof = THGContentProof ? THGContentProof.buildCommentProof({
      ok, errorCode, content: ctx.content, userID: ctx.userID, preCount: ctx.preCount, duplicate: ctx.duplicate
    }) : null;
    if (proof && notes) {
      proof.notes = proof.notes ? proof.notes + ' · ' + notes : notes;
    }
    if (proof && navDiag) proof.nav_diagnostic = navDiag; // PR8A landing telemetry → evidence_json
    foldForensicsInto(proof);
    applyTargetNotReachedOverride(proof, errorCode);
    // Echo the server-issued execution_id back so the backend terminal-state CAS can gate on it.
    if (proof && ctx?.executionId) proof.execution_id = ctx.executionId;
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
  // buildPreCtx snapshots the pre-submit proof context (FB uid + comment count + dup match),
  // shared by both comment executors. Reads THGContentProof at call time; '' / 0 / false when absent.
  function buildPreCtx(content, executionId) {
    const proof = THGContentProof || null;
    const fbUID = proof?.currentFBUserID() || '';
    const preCount = proof ? proof.snapshotCommentCount() : 0;
    const preMatched = proof ? !!proof.findCommentNode(content, fbUID) : false;
    return { content, userID: fbUID, preCount, duplicate: preMatched, executionId };
  }

  const FEED_MAX_SCROLLS = 8;
  const FEED_PER_ATTEMPT_TIMEOUT_MS = 2500;

  // feedScrollLocate is the feed-aware GATE-1: alternate short stability attempts with scroll
  // pulses until the target article materialises (or scrolls exhausted). Returns { targetScope, scrollsDone }.
  async function feedScrollLocate(targetPostId) {
    let targetScope = null;
    let scrollsDone = 0;
    for (let i = 0; i <= FEED_MAX_SCROLLS; i++) {
      targetScope = await waitUntilTargetArticleStable(targetPostId, {
        timeoutMs: FEED_PER_ATTEMPT_TIMEOUT_MS,
        stableMs: 500,
        pollMs: 200,
      });
      if (targetScope) break;
      if (i === FEED_MAX_SCROLLS) break;
      try {
        globalThis.scrollBy({ top: 1800, behavior: 'instant' });
      } catch {
        globalThis.scrollTo(0, globalThis.scrollY + 1800);
      }
      scrollsDone++;
      await wait(700);
    }
    return { targetScope, scrollsDone };
  }

  function feedArticleNotFound(targetPostId, scrollsDone, navAtEntry, ctx) {
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
      (FEED_MAX_SCROLLS + 1) + ' attempts of ' + FEED_PER_ATTEMPT_TIMEOUT_MS + 'ms each)');
  }

  // feedGate2Scope — feed Checkpoint 2 post-click identity. Returns { scope } or { fail }.
  function feedGate2Scope(targetPostId, targetScope, ctx) {
    const refreshed = findTargetArticle(targetPostId);
    const scope = refreshed || targetScope;
    if (extractArticleCanonicalEntityId(scope) !== targetPostId) {
      const landed = location.href || '';
      console.warn('[THG outbound.executeCommentInFeed] gate2_swap',
        { target_post_id: targetPostId, scope_id: extractArticleCanonicalEntityId(scope), landed });
      return { fail: commentResult(false, 'context_drift', null, ctx,
        'path2.identity_gate_2_post_click_swap: scope canonical id != ' + abbreviate(targetPostId) +
        ' landed_at=' + landed) };
    }
    return { scope };
  }

  // feedResolveEditor — feed editor acquisition (gate-2 + scroll retry / gate-2b). Returns
  // { editor } or { fail }.
  async function feedResolveEditor(targetPostId, targetScope, ctx) {
    const g2 = feedGate2Scope(targetPostId, targetScope, ctx);
    if (g2.fail) return { fail: g2.fail };
    let scope = g2.scope;
    let editor = findCommentEditor(scope);
    if (!editor) {
      // Scroll a touch + re-find. Same recovery executeComment uses.
      globalThis.scrollBy({ top: 420, behavior: 'smooth' });
      await wait(900);
      const refreshed2 = findTargetArticle(targetPostId);
      scope = refreshed2 || targetScope;
      if (extractArticleCanonicalEntityId(scope) !== targetPostId) {
        const landed = location.href || '';
        return { fail: commentResult(false, 'context_drift', null, ctx,
          'path2.identity_gate_2b_scroll_swap: scope canonical id != ' + abbreviate(targetPostId) +
          ' landed_at=' + landed) };
      }
      editor = findCommentEditor(scope);
    }
    if (!editor) {
      const landed = location.href || '';
      console.warn('[THG outbound.executeCommentInFeed] comment_box_not_found_post_click',
        { target_post_id: targetPostId, landed });
      return { fail: commentResult(false, 'comment_box_not_found', null, ctx,
        'path2.article_found_but_comment_button_missing: target id=' + abbreviate(targetPostId) +
        ' landed_at=' + landed +
        ' (Comment button clicked but no editable composer materialized in scope)') };
    }
    return { editor };
  }

  // feedCheckGate3 — feed Checkpoint 3 pre-type editor-drift guard. Returns a fail result or null.
  function feedCheckGate3(editor, targetPostId, ctx) {
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
    return null;
  }

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

    const ctx = buildPreCtx(content, executionId);

    // GATE-1 (feed-aware): scroll-then-wait until the article materialises and is stable.
    const loc = await feedScrollLocate(targetPostId);
    if (!loc.targetScope) return feedArticleNotFound(targetPostId, loc.scrollsDone, navAtEntry, ctx);
    const targetScope = loc.targetScope;

    // Scroll target into view so FB doesn't unmount it as we click.
    try { targetScope.scrollIntoView({ block: 'center', behavior: 'instant' }); } catch (e) { ignoreErr(e, 'feed-scroll'); }
    await wait(400);

    // Find Comment button inside the article scope (NOT document-wide — feed has many articles).
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

    const resolved = await feedResolveEditor(targetPostId, targetScope, ctx);
    if (resolved.fail) return resolved.fail;
    const editor = resolved.editor;

    const drift = feedCheckGate3(editor, targetPostId, ctx);
    if (drift) return drift;

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
        .find(el => String(el.getAttribute('href') || '').includes(targetId)) || null;
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
      () => !!targetId && (location.href || '').includes(targetId),
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
    console.log('[THG rung2] executeComment result', r && (r.ok ? 'OK' : (r.error || r.proof?.failure_reason)), r?.proof?.notes);
    return r;
  }

  const api = { executeComment, executeCommentInFeed, executeCommentViaRung2, probeRung2Click };
  if (typeof module !== 'undefined' && module.exports) {
    module.exports = { ...api, _test: { commentResult, abbreviate, editorContainsContent } };
  }
  return api;
})();
