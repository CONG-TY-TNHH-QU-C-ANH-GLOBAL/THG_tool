// Comment executor state machine (Browser Automation Kit, stage 2 — the real bug
// fix). ONE place that drives every comment path from composer to submit, so the
// invariant "never click submit unless the composer EXACTLY equals the queued
// content" holds on the permalink AND group-feed paths.
//
//   clear → wait stable empty → insert once → wait stable → ASSERT exactly equals
//   → (dup/mismatch: clear + retry once, else abort) → find submit
//   → RE-ASSERT right before each click → click → check composer cleared
//   → classify exact outcome.
//
// Composer mechanics live in THGCommentGuard, button finding in THGCommentSubmit.
// Shared DOM helpers (clickLikeUser / editorContainsContent / waitFor / wait /
// submitDeps) are threaded in via `deps`. Returns { ok, reason, diagnostic }.
var THGCommentSM = globalThis.THGCommentSM || (() => {
  async function runComposerToSubmit(editor, expected, commentButton, deps) {
    const G = globalThis.THGCommentGuard;
    const Sub = globalThis.THGCommentSubmit;
    const diag = {
      executor_path: deps.executorPath || 'unknown',
      expected_length: G.normalizeCommentText(expected).length,
      composer_initial_length: G.readComposerText(editor).length,
      phase: 'prepare',
    };

    // 1. clear → insert once → stabilise → equality (one dup/mismatch retry inside).
    const prep = await G.prepareComposerForComment(editor, expected);
    Object.assign(diag, prep.diagnostic || {});
    if (!prep.ok) {
      diag.phase = prep.reason; // composer_clear_failed | comment_text_doubled | comment_text_mismatch
      return { ok: false, reason: prep.reason, diagnostic: diag };
    }

    // 2. HARD pre-submit equality (invariant #3) — never submit a doubled/mismatched composer.
    diag.phase = 'pre_submit_verify';
    let check = G.assertComposerExactlyExpected(editor, expected);
    diag.composer_before_submit_length = check.actual_length;
    diag.composer_before_submit_equals_expected = check.ok;
    diag.composer_before_submit_is_duplicate_of_expected = check.duplicate;
    if (!check.ok) {
      await G.clearComposerUntilEmpty(editor);
      return { ok: false, reason: check.duplicate ? 'comment_text_doubled' : 'comment_text_mismatch', diagnostic: diag };
    }

    // 3. find the submit control.
    diag.phase = 'submit';
    const buttons = Sub.findSubmitButtons(editor, [commentButton], deps.submitDeps);
    diag.submit_button_found = buttons.length > 0;

    // 4. click → check the composer cleared. RE-ASSERT before EACH click so a draft
    // FB re-mounted between the pre-submit assert and the click can never be sent.
    let clicked = false;
    let cleared = false;
    for (const b of buttons) {
      check = G.assertComposerExactlyExpected(editor, expected);
      if (!check.ok) {
        diag.composer_before_submit_is_duplicate_of_expected = check.duplicate;
        await G.clearComposerUntilEmpty(editor);
        return { ok: false, reason: check.duplicate ? 'comment_text_doubled' : 'comment_text_mismatch', diagnostic: diag };
      }
      if (b && deps.clickLikeUser(b)) {
        clicked = true;
        cleared = await deps.waitFor(() => !deps.editorContainsContent(editor, expected), 7000, 250);
        if (cleared) break;
      }
      await deps.wait(400);
    }
    // Enter fallback — also re-asserts first.
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
      diag.phase = diag.submit_button_found ? 'submit_click_failed' : 'submit_button_not_found';
      return { ok: false, reason: diag.phase, diagnostic: diag };
    }
    if (!cleared) {
      // Submitted but composer never cleared → NOT accepted. NOT hidden_by_facebook
      // (no evidence FB accepted then hid it — invariant #4).
      diag.phase = 'submit_not_accepted';
      return { ok: false, reason: 'submit_not_accepted', diagnostic: diag };
    }

    // Composer cleared = submit accepted. The CALLER runs the DOM proof verifier to
    // classify verified_success vs submitted_unverified.
    diag.phase = 'verify';
    return { ok: true, reason: '', diagnostic: diag };
  }

  return { runComposerToSubmit };
})();

if (typeof module !== 'undefined' && module.exports) module.exports = THGCommentSM;
