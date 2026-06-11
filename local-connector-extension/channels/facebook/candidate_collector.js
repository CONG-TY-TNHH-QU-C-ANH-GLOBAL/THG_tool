// Facebook comment/composer candidate collection (ADAPTER-owned). Wraps the EXISTING working
// gate1 classifier (THGCommentComposer.findComposerEntry) and re-expresses its per-candidate
// output as the platform-generic THGAutomation.Candidate[], with FB specifics in
// channel_metadata. NO new detection logic — it adapts the proven path so platform/Vision code
// never touches Facebook DOM.
var THGChannelFacebook = globalThis.THGChannelFacebook || (globalThis.THGChannelFacebook = {});
THGChannelFacebook.candidateCollector = THGChannelFacebook.candidateCollector || (() => {
  function collect(ctx, article, deps) {
    const Candidate = globalThis.THGAutomation && globalThis.THGAutomation.Candidate;
    const Composer = globalThis.THGCommentComposer;
    if (!Candidate || !Composer) return [];
    const entry = Composer.findComposerEntry(article, deps || {});
    const targetId = ctx && ctx.target ? ctx.target.id : '';
    return (entry.candidates || []).map((c, i) => Candidate.create({
      channel: 'facebook', candidate_id: 'fb_composer_' + i, candidate_kind: 'comment_composer',
      role: c.role, aria: c.aria, parent_text: c.parent_text, editable: true, visible: true,
      accepted: c.accepted, reject_reason: c.accepted ? '' : c.reason,
      channel_metadata: { target_post_id: targetId, composer_reason: c.reason },
    }));
  }
  return { collect };
})();
if (typeof module !== 'undefined' && module.exports) module.exports = THGChannelFacebook.candidateCollector;
