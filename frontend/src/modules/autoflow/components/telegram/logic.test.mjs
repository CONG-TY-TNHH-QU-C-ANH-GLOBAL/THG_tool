// Pure-logic test for the Telegram integration UI. Transpiles the self-contained logic.ts with
// the installed TypeScript compiler (no FE test runner) + asserts.
//   Run from frontend/: node src/modules/autoflow/components/telegram/logic.test.mjs
import { readFileSync } from 'node:fs';
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const ts = require('typescript');
const src = readFileSync(new URL('./logic.ts', import.meta.url), 'utf8');
const js = ts.transpileModule(src, { compilerOptions: { module: 'ES2020', target: 'ES2020' } }).outputText;
const L = await import('data:text/javascript;base64,' + Buffer.from(js).toString('base64'));

// (1)(2)(3) status tone for the three SaaS states.
assert.strictEqual(L.statusTone('not_connected'), 'off');
assert.strictEqual(L.statusTone('connected'), 'ok');
assert.strictEqual(L.statusTone('needs_attention'), 'warn');

// (3) needs-attention remediation reasons derived from a degraded status.
const reasons = L.needsAttentionReasons({
  status: 'needs_attention', enabled: true, bot_configured: false, webhook_last_err: 'boom',
  bound_users: 0, alert_recipients: 0, flags: { TELEGRAM_BOT_ENABLED: false, TELEGRAM_NOTIFY_ENABLED: false },
});
for (const k of ['bot_disabled', 'token_missing', 'webhook_error', 'no_bound_users', 'no_alert_recipients', 'notify_disabled']) {
  assert.ok(reasons.includes(k), `expected reason ${k}`);
}
// A healthy status yields no remediation reasons.
assert.deepStrictEqual(
  L.needsAttentionReasons({ status: 'connected', enabled: true, bot_configured: true, webhook_last_err: '', bound_users: 2, alert_recipients: 1, flags: { TELEGRAM_BOT_ENABLED: true, TELEGRAM_NOTIFY_ENABLED: true } }),
  [],
);

// (5)(6) expiry countdown rendering + expired transition.
assert.strictEqual(L.formatCountdown(125), '2:05');
assert.strictEqual(L.formatCountdown(5), '0:05');
assert.strictEqual(L.formatCountdown(0), '0:00');
const future = new Date(Date.now() + 60000).toISOString();
assert.ok(L.secondsLeft(future, Date.now()) > 0 && !L.isCodeExpired(future, Date.now()));
assert.ok(L.isCodeExpired(new Date(Date.now() - 1000).toISOString(), Date.now()));

// (7) revoke is role/ownership aware.
assert.strictEqual(L.canRevokeBinding(true, 5, 9), true);   // admin → any
assert.strictEqual(L.canRevokeBinding(false, 9, 9), true);  // member → own
assert.strictEqual(L.canRevokeBinding(false, 5, 9), false); // member → other → no

// (8) alert preference sanitisation + channel-filter validation.
assert.deepStrictEqual(L.sanitizeAlertTypes(['connector_offline', 'evil', 'automation_paused']), ['connector_offline', 'automation_paused']);
assert.strictEqual(L.isValidChannelFilter('facebook', ['all', 'facebook']), true);
assert.strictEqual(L.isValidChannelFilter('myspace', ['all', 'facebook']), false);
// (13) falls back to the default catalog (facebook/taobao/1688/all) when backend list is empty.
for (const ch of ['all', 'facebook', 'taobao', '1688']) {
  assert.strictEqual(L.isValidChannelFilter(ch, []), true, `channel ${ch} must be valid generically`);
}

// (9) test-notification gating.
assert.strictEqual(L.canTestNotification(true, 1), true);
assert.strictEqual(L.canTestNotification(false, 1), false);
assert.strictEqual(L.canTestNotification(true, 0), false);

// (10)(11) admin-only tables gating.
assert.strictEqual(L.canManageAllBindings(false), false);
assert.strictEqual(L.canManageAllBindings(true), true);

// (12) action execution is a hard-off constant.
assert.strictEqual(L.actionsExecutionEnabled(), false);

// (14) no execution action is exposed in the control-action catalog.
for (const forbidden of ['comment', 'post', 'send_comment', 'execute']) {
  assert.ok(!L.CONTROL_ACTIONS.includes(forbidden), `control actions must not expose ${forbidden}`);
}
// the six expected alert types exist (channel-neutral catalog).
assert.strictEqual(L.ALERT_TYPES.length, 6);

console.log('Telegram integration UI logic: PASS');
