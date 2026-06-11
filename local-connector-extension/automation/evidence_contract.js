// Bounded evidence/proof envelope for automation fallbacks (P4 safety). Platform CORE,
// channel-neutral. HARD RULE: evidence may NEVER carry secrets or a full DOM dump. assertSafe
// throws on any forbidden key; sanitize strips them. Only a compact candidate list
// (THGAutomation.Candidate[]) + a bounded screenshot reference/crop may travel — never
// cookies/localStorage/sessionStorage/tokens/raw page HTML. There is deliberately no rawHtml
// field; full-DOM evidence is impossible by construction.
var THGAutomation = globalThis.THGAutomation || (globalThis.THGAutomation = {});
THGAutomation.Evidence = THGAutomation.Evidence || (() => {
  const FORBIDDEN = ['cookie', 'localstorage', 'sessionstorage', 'token', 'password',
    'authorization', 'cuser', 'accesstoken', 'csrf', 'fbdtsg', 'secret'];
  function isForbiddenKey(k) {
    const s = String(k).toLowerCase().replace(/[^a-z0-9]/g, '');
    return FORBIDDEN.some((f) => s === f || s.indexOf(f) !== -1);
  }
  function findForbidden(obj, path, hits) {
    path = path || ''; hits = hits || [];
    if (!obj || typeof obj !== 'object') return hits;
    for (const k of Object.keys(obj)) {
      const here = path ? path + '.' + k : k;
      if (isForbiddenKey(k)) hits.push(here);
      else if (obj[k] && typeof obj[k] === 'object') findForbidden(obj[k], here, hits);
    }
    return hits;
  }
  function sanitize(obj) {
    if (!obj || typeof obj !== 'object') return obj;
    if (Array.isArray(obj)) return obj.map(sanitize);
    const out = {};
    for (const k of Object.keys(obj)) if (!isForbiddenKey(k)) out[k] = sanitize(obj[k]);
    return out;
  }
  function assertSafe(payload) {
    const hits = findForbidden(payload);
    if (hits.length) throw new Error('evidence_contract: forbidden keys in payload: ' + hits.join(', '));
    return true;
  }
  function build(p) {
    p = p || {};
    const env = {
      channel: p.channel || '', action_type: p.action_type || '', target: p.target || null,
      failure_reason: p.failure_reason || '', screenshot_crop: p.screenshot_crop || null,
      dom_candidates: (p.dom_candidates || []).map(sanitize), spatial_context: sanitize(p.spatial_context || {}),
      safety_constraints: p.safety_constraints || {}, connector_version: p.connector_version || '',
    };
    assertSafe(env);
    return env;
  }
  return { FORBIDDEN, isForbiddenKey, findForbidden, sanitize, assertSafe, build };
})();
if (typeof module !== 'undefined' && module.exports) module.exports = THGAutomation.Evidence;
