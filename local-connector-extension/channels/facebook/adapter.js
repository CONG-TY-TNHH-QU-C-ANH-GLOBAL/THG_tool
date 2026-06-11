// Facebook channel adapter — wraps the EXISTING working comment flow behind the neutral
// AutomationChannelAdapter contract. Delegates to proven globals (THGContentOutbound executor,
// THGCommentComposer classifier, FB target locator / collector / failure map). Adds ZERO new
// behavior to the hot path; it is the SEAM the orchestrator + Vision fallback call. Registers
// itself into THGAutomation.ChannelRegistry under 'facebook' (lazy — load touches only the
// registry, never FB DOM globals, so manifest order cannot break it).
var THGChannelFacebook = globalThis.THGChannelFacebook || (globalThis.THGChannelFacebook = {});
THGChannelFacebook.adapter = THGChannelFacebook.adapter || (() => {
  const loc = () => THGChannelFacebook.targetLocator;
  const coll = () => THGChannelFacebook.candidateCollector;
  const fail = () => THGChannelFacebook.failureReasons;

  function parseTarget(url, actionType) { return loc().parse(url, actionType); }

  function locateTarget(ctx, deps) {
    // The executor (THGContentOutbound) owns live target location today; the adapter exposes the
    // seam and delegates a presence check when an injected locator is available.
    deps = deps || {};
    if (typeof deps.findTargetArticle === 'function' && ctx && ctx.target) {
      const art = deps.findTargetArticle(ctx.target.id);
      return { located: !!art, scope: art || null };
    }
    return { located: false, scope: null, reason: 'delegated_to_executor' };
  }

  function collectCandidates(ctx, deps) {
    deps = deps || {};
    const article = deps.article
      || (deps.findTargetArticle && ctx && ctx.target ? deps.findTargetArticle(ctx.target.id) : null);
    return coll().collect(ctx, article, deps);
  }

  function validateCandidate(candidate) {
    // Acceptance is decided by the proven classifier at collection time; the contract re-surfaces
    // it as { ok, reason } for the orchestrator / Vision re-validation.
    if (!candidate) return { ok: false, reason: 'no_candidate' };
    return { ok: !!candidate.accepted, reason: candidate.accepted ? 'accepted' : (candidate.reject_reason || 'rejected') };
  }

  async function performAction(ctx, deps) {
    // Delegate to the EXISTING working executor unchanged — never reimplement insertion/submit.
    deps = deps || {};
    const exec = deps.executor || globalThis.THGContentOutbound;
    if (!exec || typeof exec.executeOutbound !== 'function') return { ok: false, reason: 'executor_unavailable' };
    const command = deps.command || (ctx && ctx.meta ? ctx.meta.command : null);
    return exec.executeOutbound(command, deps.options || {});
  }

  function verifyAction(ctx, deps) {
    deps = deps || {};
    const v = deps.verifier || globalThis.THGContentReverify;
    if (v && typeof v.verify === 'function') return v.verify(ctx, deps);
    return { verified: false, reason: 'verifier_delegated' };
  }

  function buildVisionPromptContext(ctx, candidates) {
    return {
      channel: 'facebook', action_type: ctx ? ctx.action_type : '', target: ctx ? ctx.target : null,
      candidate_kinds: ['comment_button', 'comment_composer', 'reply_composer'],
      candidates: (candidates || []).map((c) => ({
        kind: c.candidate_kind, role: c.role, aria: c.aria, rect: c.rect,
        accepted: c.accepted, reject_reason: c.reject_reason,
      })),
      safety: { reject_create_post_composer: true, reject_wrong_post: true },
    };
  }

  function normalizeFailureReason(raw) { return fail().normalize(raw); }

  const impl = {
    parseTarget, locateTarget, collectCandidates, validateCandidate,
    performAction, verifyAction, buildVisionPromptContext, normalizeFailureReason,
  };
  const reg = globalThis.THGAutomation && globalThis.THGAutomation.ChannelRegistry;
  if (reg) { try { reg.register('facebook', impl); } catch (e) { /* contract/registry not ready at load */ } }
  return impl;
})();
if (typeof module !== 'undefined' && module.exports) module.exports = THGChannelFacebook.adapter;
