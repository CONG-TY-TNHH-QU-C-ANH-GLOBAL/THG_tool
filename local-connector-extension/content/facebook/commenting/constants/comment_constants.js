// Shared comment-automation constants — ONE source of truth for the keyword sets
// and timing budgets the comment composer / button / submit / state-machine
// modules use. Previously COMMENT_KEYS was re-declared 6× in outbound.js and again
// in comment_button.js (S4144 duplication), and the submit timings were scattered
// magic numbers (150 / 400 / 900 / 4 / 7000). Centralising them here means the same
// vocabulary and the same settle/retry budgets can never drift across modules.
var THGCommentConstants = globalThis.THGCommentConstants || (() => {
  // Comment editor / action-button discovery vocabulary. Diacritic-stripped forms
  // first (callers whose labelOf normalises), raw-diacritic + English variants
  // after for callers that match on the raw label.
  const COMMENT_KEYS = [
    'comment', 'write a comment', 'add a comment', 'binh luan', 'viet binh luan',
    'bình luận', 'viết bình luận',
  ];
  // Submit / send control vocabulary.
  const SUBMIT_KEYS = ['comment', 'post', 'send', 'binh luan', 'dang', 'gui'];
  // Composer-toolbar controls that must NEVER be treated as the send button
  // (sticker / camera / avatar / attach / emoji / like / share / cancel).
  const REJECT_KEYS = [
    'share', 'like', 'cancel', 'photo', 'gif', 'emoji', 'sticker', 'anh', 'huy', 'thich', 'chia se',
    'nhan dan', 'bieu tuong cam xuc', 'cam xuc', 'avatar', 'may anh', 'hinh anh', 'dinh kem', 'tep', 'tap tin',
  ];

  // Submit-button spatial geometry (px). A real send control is a COMPACT button
  // vertically near the editor and not far to its left. Named so the heuristic is
  // auditable rather than a wall of magic offsets in submitCandidateSpatial.
  const SPATIAL = {
    aboveEditorPx: 28,   // candidate bottom may sit this far above the editor top
    belowEditorPx: 42,   // candidate top may sit this far below the editor bottom
    leftSlackPx: 10,     // candidate may start this far left of the editor edge
    maxWidthPx: 110,     // send controls are compact, not full-width toolbars
    maxHeightPx: 72,
    leftPenaltyDivisor: 3, // softens the horizontal term in spatialDistance
  };

  // Submit timing budget (ms / counts). settle* drives waitForStableSubmitTarget,
  // the React/Lexical mount settle gate that replaced the fixed 150ms flush guess.
  const TIMING = {
    settleTimeoutMs: 1200,    // hard cap on waiting for the send button to settle
    settlePollMs: 50,         // poll cadence while watching for a stable target
    settleStableMs: 120,      // target must be unchanged this long to count as settled
    submitRetryWaitMs: 400,   // pause between bounded submit attempts
    clearedTimeoutMs: 7000,   // wait for the composer to clear after a click (= success proof)
    clearedPollMs: 250,       // poll cadence while waiting for the composer to clear
    maxSubmitAttempts: 4,     // bounded re-query/click attempts before giving up
    maxSubmitCandidates: 5,   // cap on the ranked submit candidates returned
  };

  return { COMMENT_KEYS, SUBMIT_KEYS, REJECT_KEYS, SPATIAL, TIMING };
})();
globalThis.THGCommentConstants = THGCommentConstants;
if (typeof module !== 'undefined' && module.exports) module.exports = THGCommentConstants;
