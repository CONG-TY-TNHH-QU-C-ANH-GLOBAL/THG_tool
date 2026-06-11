// Platform CORE regression — channel-neutral. MUST contain NO channel-specific UI label text,
// URLs, or DOM selectors as fixtures; all test data uses the generic 'demo' channel. If a real
// channel's labels/URLs appear here, the core has leaked channel knowledge — they belong only
// in that channel's adapter test (see facebook_adapter.test.js).
//   Run: node local-connector-extension/test/automation_core.test.js
const assert = require('assert');
require('../automation/adapter_contract');   // load order: contract before registry
const Registry = require('../automation/channel_registry');
const Contract = require('../automation/adapter_contract');
const Candidate = require('../automation/candidate_schema');
const Evidence = require('../automation/evidence_contract');
const ActionContext = require('../automation/action_context');
const Diagnostics = require('../automation/diagnostics');
const Metrics = require('../automation/metrics');

const fakeAdapter = () => ({
  parseTarget() {}, locateTarget() {}, collectCandidates() {}, validateCandidate() {},
  performAction() {}, verifyAction() {}, buildVisionPromptContext() {}, normalizeFailureReason() {},
});

// AdapterContract
{
  assert.strictEqual(Contract.validate(fakeAdapter()).ok, true);
  const bad = Contract.validate({ parseTarget() {} });
  assert.strictEqual(bad.ok, false);
  assert.ok(bad.missing.indexOf('performAction') !== -1);
  assert.throws(() => Contract.assertValid({}, 'demo'), /missing methods/);
}

// ChannelRegistry (generic channel keys only)
{
  Registry.clear();
  Registry.register('demo', fakeAdapter());
  assert.strictEqual(Registry.has('demo'), true);
  assert.strictEqual(typeof Registry.get('demo').performAction, 'function');
  assert.deepStrictEqual(Registry.list(), ['demo']);
  assert.throws(() => Registry.register('broken', { parseTarget() {} }), /missing methods/);
  assert.throws(() => Registry.register('', fakeAdapter()), /non-empty string/);
  Registry.clear();
  assert.deepStrictEqual(Registry.list(), []);
}

// Candidate schema — generic DOM read, channel specifics only in channel_metadata
{
  const c = Candidate.create({ channel: 'demo', role: 'textbox', editable: true });
  assert.strictEqual(c.accepted, false);
  assert.strictEqual(c.enabled, true);
  assert.deepStrictEqual(c.channel_metadata, {});
  const el = {
    tagName: 'DIV', parentElement: { textContent: '  parent  ' },
    getAttribute: (n) => ({ role: 'textbox', 'aria-label': 'label' }[n] || ''),
    getBoundingClientRect: () => ({ left: 1, top: 2, width: 30, height: 12 }),
  };
  const fe = Candidate.fromElement(el, { channel: 'demo', candidate_kind: 'box' }, { visible: () => true, editable: () => true });
  assert.strictEqual(fe.tag, 'DIV');
  assert.strictEqual(fe.role, 'textbox');
  assert.strictEqual(fe.parent_text, 'parent');
  assert.deepStrictEqual(fe.rect, { x: 1, y: 2, w: 30, h: 12 });
  assert.strictEqual(fe.editable, true);
  Candidate.reject(fe, 'because'); assert.strictEqual(fe.accepted, false); assert.strictEqual(fe.reject_reason, 'because');
  Candidate.accept(fe); assert.strictEqual(fe.accepted, true); assert.strictEqual(fe.reject_reason, '');
}

// Evidence — secrets/full-DOM stripped; assertSafe is the strict throw-gate, build() defensively
// sanitizes so the emitted envelope can never carry a forbidden key.
{
  assert.throws(() => Evidence.assertSafe({ a: { localStorage: {} } }), /forbidden keys/);
  assert.throws(() => Evidence.assertSafe({ access_token: 'z' }), /forbidden keys/);
  const cleaned = Evidence.sanitize({ keep: 1, token: 'secret', nested: { c_user: '9', ok: 2 } });
  assert.deepStrictEqual(cleaned, { keep: 1, nested: { ok: 2 } });
  const env = Evidence.build({ channel: 'demo', action_type: 'act', spatial_context: { cookie: 'x=1', ok: 1 }, dom_candidates: [{ role: 'box', token: 'NO' }] });
  assert.strictEqual(env.spatial_context.cookie, undefined);
  assert.strictEqual(env.spatial_context.ok, 1);
  assert.strictEqual(env.dom_candidates[0].token, undefined);
  assert.strictEqual(env.dom_candidates[0].role, 'box');
  Evidence.assertSafe(env); // the built envelope passes the strict gate
  assert.strictEqual('raw_html' in env, false); // no full-DOM field exists by construction
}

// ActionContext — ids injected, never hardcoded; channel + action_type required
{
  assert.throws(() => ActionContext.create({ action_type: 'act' }), /channel is required/);
  assert.throws(() => ActionContext.create({ channel: 'demo' }), /action_type is required/);
  const ctx = ActionContext.create({ channel: 'demo', action_type: 'act', org_id: 5, account_id: 49 });
  assert.strictEqual(ctx.org_id, 5);
  assert.strictEqual(ctx.connector_id, null);
  const c2 = ActionContext.withTarget(ctx, { id: 'T' });
  assert.strictEqual(c2.target.id, 'T');
  assert.strictEqual(ctx.target, null); // immutable copy
}

// Diagnostics — channel-aware shape, counts derived from candidates
{
  const d = Diagnostics.gateFailure({
    channel: 'demo', action_type: 'act', target_id: 'T', final_failure_reason: 'entry_not_found',
    candidates: [
      { role: 'textbox', editable: true, accepted: false, reject_reason: 'r1' },
      { role: 'button', editable: false, accepted: false, reject_reason: 'r2' },
    ],
  });
  assert.strictEqual(d.candidate_count, 2);
  assert.strictEqual(d.textbox_candidates_count, 1);
  assert.strictEqual(d.contenteditable_candidates_count, 1);
  assert.strictEqual(d.final_failure_reason, 'entry_not_found');
  assert.strictEqual(d.candidates[0].reject_reason, 'r1');
}

// Metrics — canonical name + fixed dimension whitelist
{
  assert.strictEqual(Metrics.name('demo', 'act', 'gate1_fail'), 'demo.act.gate1_fail');
  const dims = Metrics.dimensions({ org_id: 5, channel: 'demo', action_type: 'act', extra: 'dropped' });
  assert.strictEqual(dims.org_id, 5);
  assert.strictEqual(dims.failure_phase, null);
  assert.strictEqual('extra' in dims, false); // only whitelisted dims survive
  let captured = null;
  const n = Metrics.emit((name, d) => { captured = { name, d }; }, 'demo', 'act', 'used', { connector_id: 7 });
  assert.strictEqual(n, 'demo.act.used');
  assert.strictEqual(captured.d.connector_id, 7);
}

console.log('automation platform core regression: PASS');
