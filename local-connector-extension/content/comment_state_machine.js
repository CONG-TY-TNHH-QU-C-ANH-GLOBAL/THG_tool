// Comment executor state machine. ONE place that drives every comment path from
// composer to submit, so "never click submit unless the composer EXACTLY equals the
// queued content" holds on the permalink AND group-feed paths.
//
//   clear-or-abort → insert (verified) → assert exactly equals → (dup/mismatch: clear
//   + retry once, else abort) → find submit → RE-ASSERT before each click → click →
//   check composer cleared → classify.
//
// Composer mechanics live in THGCommentGuard, button finding in THGCommentSubmit.
// Shared DOM helpers + outboundId + executorPath are threaded in via `deps`.
var THGCommentSM = globalThis.THGCommentSM || (() => {
  const K = globalThis.THGCommentConstants
    || (typeof require === 'undefined' ? null : require('./comment_constants.js'));
  const { TIMING } = K;
  const DEBUG_COMPOSER = true; // flip false to silence telemetry after the incident
  const EXT_VERSION = (() => { try { return chrome.runtime.getManifest().version; } catch (_) { return ''; } })();

  function slog(diag, ok, reason) {
    if (!DEBUG_COMPOSER) return;
    try {
      console.log('[THG sm]', {
        extension_version: EXT_VERSION,
        outbound_id: diag.outbound_id || 0,
        executor_path: diag.executor_path || 'unknown',
        method: diag.method || '-',
        expected_length: diag.expected_length != null ? diag.expected_length : -1,
        actual_length: diag.composer_before_submit_length != null ? diag.composer_before_submit_length
          : (diag.composer_after_insert_length != null ? diag.composer_after_insert_length : -1),
        equals_expected: !!diag.composer_before_submit_equals_expected,
        is_duplicate: !!diag.composer_before_submit_is_duplicate_of_expected,
        phase: diag.phase,
        reason: reason || '',
        ok: !!ok,
        submit_clicked: !!diag.submit_clicked,
        composer_cleared_after_submit: !!diag.composer_cleared_after_submit,
        submit_button_found: !!diag.submit_button_found,
      });
    } catch (_) { /* never break delivery on a log */ }
  }

  async function runComposerToSubmit(editor, expected, commentButton, deps) {
    const G = globalThis.THGCommentGuard;
    const Sub = globalThis.THGCommentSubmit;
    const ctx = { outboundId: deps.outboundId || 0, executorPath: deps.executorPath || 'unknown' };
    const diag = {
      outbound_id: ctx.outboundId,
      executor_path: ctx.executorPath,
      expected_length: G.normalizeCommentText(expected).length,
      composer_initial_length: G.readComposerText(editor).length,
      phase: 'prepare',
    };
    const done = (ok, reason) => { slog(diag, ok, reason); return { ok, reason: reason || '', diagnostic: diag }; };

    // 1. clear-or-abort → insert (verified) → assert (guard owns it, with telemetry).
    const prep = await G.prepareComposerForComment(editor, expected, ctx);
    Object.assign(diag, prep.diagnostic || {});
    diag.executor_path = ctx.executorPath;
    diag.outbound_id = ctx.outboundId;
    if (!prep.ok) { diag.phase = prep.reason; return done(false, prep.reason); }

    // 2. HARD pre-submit equality — never submit a doubled/mismatched composer.
    diag.phase = 'pre_submit_verify';
    let check = G.assertComposerExactlyExpected(editor, expected);
    diag.composer_before_submit_length = check.actual_length;
    diag.composer_before_submit_equals_expected = check.ok;
    diag.composer_before_submit_is_duplicate_of_expected = check.duplicate;
    if (!check.ok) {
      await G.clearComposerUntilEmpty(editor);
      return done(false, check.duplicate ? 'comment_text_doubled' : 'comment_text_mismatch');
    }

    // 3+4. SUBMIT — atomic readiness against Facebook's React/Lexical send button.
    // After execCommand('insertText') activates Lexical's EditorState (done in the guard
    // above), Facebook attaches/enables/RE-MOUNTS the real send button on a React flush.
    // Clicking the pre-flush generation is a ghost-button no-op (the "1–3 attempts before
    // submit succeeds" symptom). So:
    //   (a) SETTLE GATE — poll until the top submit candidate stops being re-created
    //       (waitForStableSubmitTarget), instead of a blind fixed-ms wait, then
    //   (b) RE-QUERY the submit control FRESH on EVERY attempt — never reuse a node
    //       captured before the flush; it may be stale/detached/replaced.
    // Unchanged: findSubmitButtons stays scoped to THIS editor (no global "Comment"
    // search), keeps its visible/enabled/reject-label filtering, RE-ASSERT-before-click
    // guards against a doubled composer, and submit "success" is still proven only by the
    // composer clearing. Post/account identity gates are upstream (executeComment) and
    // untouched. Synthetic clicks are NOT trusted input — success is effect-verified.
    diag.phase = 'submit';
    const settledTarget = await Sub.waitForStableSubmitTarget(editor, [commentButton], deps.submitDeps, {
      wait: deps.wait, now: deps.now,
    });
    diag.submit_target_settled = !!settledTarget;

    let clicked = false;
    let cleared = false;
    let sawButton = false;
    let submitAttempts = 0;
    for (let attempt = 0; attempt < TIMING.maxSubmitAttempts && !cleared; attempt += 1) {
      submitAttempts = attempt + 1;
      // Double-submit guard: once a click has fired, a cleared composer means the submit
      // was ACCEPTED (FB cleared it, possibly slowly) — stop; never click a second time.
      if (clicked && !deps.editorContainsContent(editor, expected)) { cleared = true; break; }
      check = G.assertComposerExactlyExpected(editor, expected);
      if (!check.ok) {
        diag.composer_before_submit_is_duplicate_of_expected = check.duplicate;
        await G.clearComposerUntilEmpty(editor);
        return done(false, check.duplicate ? 'comment_text_doubled' : 'comment_text_mismatch');
      }
      // RE-QUERY fresh — the current generation of the send button, scoped to this editor.
      const fresh = Sub.findSubmitButtons(editor, [commentButton], deps.submitDeps);
      if (fresh.length > 0) sawButton = true;
      const b = fresh[0];
      if (b && deps.clickLikeUser(b)) {
        clicked = true;
        cleared = await deps.waitFor(() => !deps.editorContainsContent(editor, expected), TIMING.clearedTimeoutMs, TIMING.clearedPollMs);
        if (cleared) break;
      }
      await deps.wait(TIMING.submitRetryWaitMs);
    }
    diag.submit_button_found = sawButton;
    diag.submit_requeried_attempts = submitAttempts;
    if (!cleared) {
      check = G.assertComposerExactlyExpected(editor, expected);
      if (check.ok && Sub.pressEnter(editor)) {
        clicked = true;
        cleared = await deps.waitFor(() => !deps.editorContainsContent(editor, expected), TIMING.clearedTimeoutMs, TIMING.clearedPollMs);
      }
    }
    diag.submit_clicked = clicked;
    diag.composer_cleared_after_submit = cleared;

    if (!clicked) {
      await G.clearComposerUntilEmpty(editor); // no leftover text → no FB draft next time
      diag.phase = diag.submit_button_found ? 'submit_click_failed' : 'submit_button_not_found';
      return done(false, diag.phase);
    }
    if (!cleared) {
      // submit_not_accepted — NEVER hidden_by_facebook. hidden_by_facebook is only
      // valid when submit_clicked === true AND composer_cleared_after_submit === true
      // (the success path below); a composer that never cleared has no such evidence.
      await G.clearComposerUntilEmpty(editor);
      diag.phase = 'submit_not_accepted';
      return done(false, 'submit_not_accepted');
    }

    // Composer cleared = submit accepted. Caller runs the DOM proof verifier to
    // classify verified_success vs submitted_unverified.
    diag.phase = 'verify';
    return done(true, '');
  }

  return { runComposerToSubmit };
})();

if (typeof module !== 'undefined' && module.exports) module.exports = THGCommentSM;
