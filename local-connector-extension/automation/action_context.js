// ActionContext — the neutral carrier threaded through every automation phase (locate →
// collect → validate → perform → verify). Platform CORE. channel + action_type drive adapter
// resolution and every reusable record. All tenant/identity values are INJECTED by the caller;
// nothing (org/account/connector id) is hardcoded.
var THGAutomation = globalThis.THGAutomation || (globalThis.THGAutomation = {});
THGAutomation.ActionContext = THGAutomation.ActionContext || (() => {
  function create(p) {
    p = p || {};
    if (!p.channel) throw new Error('action_context: channel is required');
    if (!p.action_type) throw new Error('action_context: action_type is required');
    return {
      channel: p.channel, action_type: p.action_type, target: p.target || null,
      org_id: p.org_id != null ? p.org_id : null, account_id: p.account_id != null ? p.account_id : null,
      connector_id: p.connector_id != null ? p.connector_id : null,
      execution_id: p.execution_id || '', outbound_id: p.outbound_id != null ? p.outbound_id : null,
      connector_version: p.connector_version || '', meta: p.meta || {},
    };
  }
  function withTarget(ctx, target) { return Object.assign({}, ctx, { target }); }
  return { create, withTarget };
})();
if (typeof module !== 'undefined' && module.exports) module.exports = THGAutomation.ActionContext;
