// The AutomationChannelAdapter contract. Every channel adapter (facebook now; taobao/1688
// later) MUST implement these methods so the orchestrator and the Vision fallback can drive any
// channel through one shape. Platform CORE — knows NOTHING about any channel's DOM/URLs/labels.
var THGAutomation = globalThis.THGAutomation || (globalThis.THGAutomation = {});
THGAutomation.AdapterContract = THGAutomation.AdapterContract || (() => {
  const REQUIRED = [
    'parseTarget', 'locateTarget', 'collectCandidates', 'validateCandidate',
    'performAction', 'verifyAction', 'buildVisionPromptContext', 'normalizeFailureReason',
  ];
  function validate(adapter) {
    if (!adapter || typeof adapter !== 'object') return { ok: false, missing: REQUIRED.slice() };
    const missing = REQUIRED.filter((m) => typeof adapter[m] !== 'function');
    return { ok: missing.length === 0, missing };
  }
  function assertValid(adapter, channel) {
    const r = validate(adapter);
    if (!r.ok) throw new Error('adapter_contract[' + (channel || '?') + ']: missing methods: ' + r.missing.join(', '));
    return true;
  }
  return { REQUIRED, validate, assertValid };
})();
if (typeof module !== 'undefined' && module.exports) module.exports = THGAutomation.AdapterContract;
