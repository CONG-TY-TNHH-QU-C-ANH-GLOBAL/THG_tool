// Facebook ADAPTER regression — the FB-specific labels/URLs live HERE, never in the core test.
//   Run: node local-connector-extension/test/facebook_adapter.test.js
const assert = require('assert');
const { makeEl, makeArticle, sizeVisible } = require('./fake_dom');
// Platform core (registry/contract/candidate) + the proven composer classifier the adapter wraps.
require('../automation/adapter_contract');
const Registry = require('../automation/channel_registry');
const Contract = require('../automation/adapter_contract');
require('../automation/candidate_schema');
require('../content/facebook/commenting/composer/comment_composer');
// FB adapter pieces (adapter.js self-registers into the registry on require).
const targetLocator = require('../channels/facebook/target_locator');
const failureReasons = require('../channels/facebook/failure_reasons');
require('../channels/facebook/candidate_collector');
const adapter = require('../channels/facebook/adapter');

const LIVE_URL = 'https://www.facebook.com/groups/1312868109620530/posts/2040078973566103/';

// targetLocator.parse — the live failing post
{
  const t = targetLocator.parse(LIVE_URL, 'comment');
  assert.strictEqual(t.channel, 'facebook');
  assert.strictEqual(t.kind, 'group_post');
  assert.strictEqual(t.id, '2040078973566103');
  assert.strictEqual(targetLocator.idFromUrl('https://www.facebook.com/watch/?v=99887766'), '99887766');
  assert.strictEqual(targetLocator.idFromUrl('https://www.facebook.com/groups/x/'), '');
}

// failureReasons.normalize — raw FB codes → neutral { phase, reason }
{
  assert.deepStrictEqual(failureReasons.normalize('comment_button_not_found'), { phase: 'gate1_discovery', reason: 'entry_not_found' });
  assert.deepStrictEqual(failureReasons.normalize('comment_box_not_found'), { phase: 'editor_acquisition', reason: 'composer_not_found' });
  assert.strictEqual(failureReasons.normalize('context_drift').reason, 'wrong_target');
  assert.strictEqual(failureReasons.normalize('totally_new').phase, 'unknown');
}

// Adapter satisfies the platform contract AND self-registered as 'facebook'.
{
  assert.strictEqual(Contract.validate(adapter).ok, true);
  assert.strictEqual(Registry.has('facebook'), true);
  assert.strictEqual(Registry.get('facebook'), adapter);
}

// collectCandidates wraps THGCommentComposer.findComposerEntry into generic candidates with FB
// metadata. The live "Write an answer…" shape (host unknown on permalink) is accepted.
{
  const ctx = { channel: 'facebook', action_type: 'comment', target: targetLocator.parse(LIVE_URL, 'comment') };
  const answerBox = makeEl({ role: 'textbox', ce: 'true', aria: 'Write an answer…', parentText: 'Write an answer…', w: 450, h: 20 });
  const deps = {
    article: makeArticle({ editables: [] }),               // target article subtree empty
    visible: sizeVisible, closestArticle: () => ({}),
    classifyHost: () => 'unknown',                          // permalink doctrine → unknown
    docEditables: () => [answerBox],
  };
  const cands = adapter.collectCandidates(ctx, deps);
  assert.strictEqual(cands.length, 1);
  assert.strictEqual(cands[0].channel, 'facebook');
  assert.strictEqual(cands[0].candidate_kind, 'comment_composer');
  assert.strictEqual(cands[0].accepted, true);
  assert.strictEqual(cands[0].channel_metadata.target_post_id, '2040078973566103');
  assert.strictEqual(adapter.validateCandidate(cands[0]).ok, true);
}

// performAction delegates to the executor; absent executor fails safely (no reimplementation).
(async () => {
  const ctx = { channel: 'facebook', action_type: 'comment', target: targetLocator.parse(LIVE_URL, 'comment'), meta: {} };
  const noExec = await adapter.performAction(ctx, { executor: null });
  assert.strictEqual(noExec.ok, false);
  assert.strictEqual(noExec.reason, 'executor_unavailable');
  let received = null;
  const stubExec = { executeOutbound: (cmd, opts) => { received = { cmd, opts }; return { ok: true, via: 'stub' }; } };
  const ran = await adapter.performAction(ctx, { executor: stubExec, command: { type: 'comment' }, options: { accountId: 49 } });
  assert.strictEqual(ran.ok, true);
  assert.strictEqual(received.cmd.type, 'comment');
  assert.strictEqual(received.opts.accountId, 49);

  // buildVisionPromptContext is generic (no raw FB DOM), kinds are FB candidate kinds.
  const vctx = adapter.buildVisionPromptContext(ctx, [{ candidate_kind: 'comment_composer', role: 'textbox', aria: 'Write an answer…', rect: null, accepted: true, reject_reason: '' }]);
  assert.strictEqual(vctx.channel, 'facebook');
  assert.ok(vctx.candidate_kinds.indexOf('comment_composer') !== -1);
  assert.strictEqual(vctx.safety.reject_create_post_composer, true);

  console.log('facebook adapter regression: PASS');
})();
