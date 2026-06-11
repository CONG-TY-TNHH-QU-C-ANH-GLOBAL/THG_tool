// Generic DOM candidate schema (P3). ONE shape for every channel's UI candidates (comment
// composer, search box, supplier-contact button, …). Platform CORE: fromElement reads only
// generic DOM (getAttribute / tagName / getBoundingClientRect) with injected visible/editable
// predicates, so CORE carries no channel selectors. Channel specifics live in channel_metadata,
// which the adapter fills (e.g. target_post_id, host relationship, FB reject reasons).
var THGAutomation = globalThis.THGAutomation || (globalThis.THGAutomation = {});
THGAutomation.Candidate = THGAutomation.Candidate || (() => {
  function create(p) {
    p = p || {};
    return {
      channel: p.channel || '', candidate_id: p.candidate_id || '', candidate_kind: p.candidate_kind || '',
      tag: p.tag || '', role: p.role || '', aria: p.aria || '', text: p.text || '', parent_text: p.parent_text || '',
      rect: p.rect || null, visible: !!p.visible, enabled: p.enabled !== false, editable: !!p.editable,
      channel_metadata: p.channel_metadata || {}, accepted: !!p.accepted, reject_reason: p.reject_reason || '',
    };
  }
  function fromElement(el, extra, deps) {
    extra = extra || {}; deps = deps || {};
    const attr = (n) => (el && el.getAttribute ? (el.getAttribute(n) || '') : '');
    const rect = el && el.getBoundingClientRect ? el.getBoundingClientRect() : null;
    return create({
      channel: extra.channel || '', candidate_id: extra.candidate_id || '', candidate_kind: extra.candidate_kind || '',
      tag: (el && el.tagName) || '', role: attr('role'), aria: attr('aria-label'), text: extra.text || '',
      parent_text: (el && el.parentElement && el.parentElement.textContent) ? el.parentElement.textContent.trim().slice(0, 80) : '',
      rect: rect ? { x: rect.left, y: rect.top, w: rect.width, h: rect.height } : null,
      visible: deps.visible ? !!deps.visible(el) : true,
      editable: deps.editable ? !!deps.editable(el) : false,
      channel_metadata: extra.channel_metadata || {}, accepted: !!extra.accepted, reject_reason: extra.reject_reason || '',
    });
  }
  function accept(c) { c.accepted = true; c.reject_reason = ''; return c; }
  function reject(c, reason) { c.accepted = false; c.reject_reason = reason || 'rejected'; return c; }
  return { create, fromElement, accept, reject };
})();
if (typeof module !== 'undefined' && module.exports) module.exports = THGAutomation.Candidate;
