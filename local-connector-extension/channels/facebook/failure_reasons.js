// Facebook failure-reason normalization (ADAPTER-owned). Maps raw FB executor reason codes to a
// neutral { phase, reason } the platform metrics/diagnostics consume. Platform CORE has no FB
// reason strings — they are defined ONLY here.
var THGChannelFacebook = globalThis.THGChannelFacebook || (globalThis.THGChannelFacebook = {});
THGChannelFacebook.failureReasons = THGChannelFacebook.failureReasons || (() => {
  const MAP = {
    comment_button_not_found: { phase: 'gate1_discovery', reason: 'entry_not_found' },
    comment_box_not_found: { phase: 'editor_acquisition', reason: 'composer_not_found' },
    target_not_reached: { phase: 'navigation', reason: 'target_not_reached' },
    context_drift: { phase: 'identity_gate', reason: 'wrong_target' },
    wrong_post: { phase: 'candidate_validation', reason: 'wrong_target' },
    create_post_composer: { phase: 'candidate_validation', reason: 'global_composer_rejected' },
    composer_not_cleared: { phase: 'submit', reason: 'composer_state_invalid' },
    submitted_unverified: { phase: 'verify', reason: 'unverified' },
    human_required: { phase: 'precondition', reason: 'human_required' },
  };
  function normalize(raw) {
    const key = String(raw || '').toLowerCase();
    return MAP[key] || { phase: 'unknown', reason: key || 'unknown' };
  }
  return { MAP, normalize };
})();
if (typeof module !== 'undefined' && module.exports) module.exports = THGChannelFacebook.failureReasons;
