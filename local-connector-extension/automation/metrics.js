// Channel-aware metric shape (P7). name + dimensions only — CORE does NOT own a sink/emitter;
// the orchestrator passes an emit(name, dims) sink. Canonical name = channel.action_type.event
// and a FIXED dimension whitelist so every channel/action reports the same shape (enables
// per-channel dashboards + the circuit breaker keyed by org/channel/connector/account/action).
var THGAutomation = globalThis.THGAutomation || (globalThis.THGAutomation = {});
THGAutomation.Metrics = THGAutomation.Metrics || (() => {
  const DIMENSIONS = ['org_id', 'connector_id', 'account_id', 'channel', 'action_type',
    'failure_phase', 'failure_reason', 'adapter_version'];
  function name(channel, action_type, event) {
    return [channel || 'unknown', action_type || 'unknown', event || 'event'].join('.');
  }
  function dimensions(p) {
    p = p || {}; const out = {};
    for (const d of DIMENSIONS) out[d] = p[d] != null ? p[d] : null;
    return out;
  }
  function emit(sink, channel, action_type, event, dims) {
    const n = name(channel, action_type, event);
    if (typeof sink === 'function') sink(n, dimensions(Object.assign({ channel, action_type }, dims || {})));
    return n;
  }
  return { DIMENSIONS, name, dimensions, emit };
})();
if (typeof module !== 'undefined' && module.exports) module.exports = THGAutomation.Metrics;
