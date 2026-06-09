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

    // 3. find the submit control.
    diag.phase = 'submit';
    const buttons = Sub.findSubmitButtons(editor, [commentButton], deps.submitDeps);
    diag.submit_button_found = buttons.length > 0;

    // 4. click → check cleared. RE-ASSERT before EACH click (catch a late draft re-mount).
    let clicked = false;
    let cleared = false;
    for (const b of buttons) {
      check = G.assertComposerExactlyExpected(editor, expected);
      if (!check.ok) {
        diag.composer_before_submit_is_duplicate_of_expected = check.duplicate;
        await G.clearComposerUntilEmpty(editor);
        return done(false, check.duplicate ? 'comment_text_doubled' : 'comment_text_mismatch');
      }
      if (b && deps.clickLikeUser(b)) {
        clicked = true;
        cleared = await deps.waitFor(() => !deps.editorContainsContent(editor, expected), 7000, 250);
        if (cleared) break;
      }
      await deps.wait(400);
    }
    if (!cleared) {
      check = G.assertComposerExactlyExpected(editor, expected);
      if (check.ok && Sub.pressEnter(editor)) {
        clicked = true;
        cleared = await deps.waitFor(() => !deps.editorContainsContent(editor, expected), 7000, 250);
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
