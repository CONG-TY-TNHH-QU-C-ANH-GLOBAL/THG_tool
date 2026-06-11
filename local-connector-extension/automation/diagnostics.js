// Channel-aware structured diagnostics (P6). ONE object per gate/phase failure so any channel's
// UI drift is explainable WITHOUT a full HTML dump. Platform CORE — neutral fields only; channel
// specifics ride in candidates[].channel_metadata and the adapter's normalized failure reason.
var THGAutomation = globalThis.THGAutomation || (globalThis.THGAutomation = {});
THGAutomation.Diagnostics = THGAutomation.Diagnostics || (() => {
  function gateFailure(p) {
    p = p || {};
    const cands = p.candidates || [];
    const count = (pred) => cands.filter(pred).length;
    return {
      channel: p.channel || '', action_type: p.action_type || '',
      target_url: p.target_url || '', target_id: p.target_id || '', connector_version: p.connector_version || '',
      dom_gate_result: p.dom_gate_result || 'fail',
      vision_enabled: !!p.vision_enabled, vision_attempted: !!p.vision_attempted,
      vision_decision: p.vision_decision || null,
      vision_confidence: typeof p.vision_confidence === 'number' ? p.vision_confidence : null,
      vision_risk_flags: p.vision_risk_flags || [],
      candidate_count: cands.length,
      textbox_candidates_count: count((c) => String(c.role || '').toLowerCase() === 'textbox'),
      contenteditable_candidates_count: count((c) => !!c.editable),
      candidates: cands.map((c) => ({
        tag: c.tag, role: c.role, aria: c.aria, text: c.text, parent_text: c.parent_text,
        rect: c.rect, accepted: c.accepted, reject_reason: c.reject_reason,
      })),
      final_failure_reason: p.final_failure_reason || '',
    };
  }
  return { gateFailure };
})();
if (typeof module !== 'undefined' && module.exports) module.exports = THGAutomation.Diagnostics;
