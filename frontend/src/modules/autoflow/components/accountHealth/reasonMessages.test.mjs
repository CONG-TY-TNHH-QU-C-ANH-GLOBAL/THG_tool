// Pure-mapper unit test for the Account Health UX layer (PR-E). No test framework
// is configured for the FE, so this transpiles the self-contained reasonMessages.ts
// with the installed TypeScript compiler and asserts against the real module.
//   Run from frontend/:  node src/modules/autoflow/components/accountHealth/reasonMessages.test.mjs
import { readFileSync } from 'node:fs';
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const ts = require('typescript');

const src = readFileSync(new URL('./reasonMessages.ts', import.meta.url), 'utf8');
const js = ts.transpileModule(src, { compilerOptions: { module: 'ES2020', target: 'ES2020' } }).outputText;
const mod = await import('data:text/javascript;base64,' + Buffer.from(js).toString('base64'));
const { mapReason, pickPrimaryReason, overallStatus, execState, accountExecState } = mod;

const KNOWN = [
  'connector_offline', 'actor_identity_unknown', 'actor_mismatch_blocked',
  'extension_version_outdated', 'account_cooldown_active', 'risk_ceiling_exceeded',
  'daily_limit_exceeded',
];

// Every known reason → friendly title/description/action, raw code never leaks into
// the title, and technical_code is kept for the admin view only.
for (const code of KNOWN) {
  const m = mapReason(code);
  assert.ok(m.title && m.description && m.action, `${code} must have title/description/action`);
  assert.ok(!m.title.includes('_'), `${code} title must not contain the raw code`);
  assert.strictEqual(m.technical_code, code, `${code} keeps technical_code`);
}

// Founder's specific mappings + severities.
assert.strictEqual(mapReason('connector_offline').title, 'Chrome profile chưa kết nối');
assert.strictEqual(mapReason('connector_offline').severity, 'blocked');
assert.strictEqual(mapReason('actor_identity_unknown').severity, 'warning');
assert.strictEqual(mapReason('account_cooldown_active').severity, 'waiting');
assert.strictEqual(mapReason('risk_ceiling_exceeded').severity, 'blocked');

// Unknown reason → friendly fallback (NOT the raw code), but code kept for admin.
const unknown = mapReason('some_future_code_xyz');
assert.strictEqual(unknown.title, 'Cần kiểm tra tài khoản');
assert.strictEqual(unknown.technical_code, 'some_future_code_xyz');

// Priority picker: actor_mismatch_blocked wins over connector_offline + cooldown.
assert.strictEqual(
  pickPrimaryReason(['account_cooldown_active', 'connector_offline', 'actor_mismatch_blocked']),
  'actor_mismatch_blocked',
);
assert.strictEqual(pickPrimaryReason([]), null);

// Overall status reduction.
assert.strictEqual(overallStatus([]).severity, 'ready');
assert.strictEqual(overallStatus([]).label, 'Sẵn sàng');
assert.strictEqual(overallStatus(['account_cooldown_active']).severity, 'waiting');
assert.strictEqual(overallStatus(['connector_offline', 'account_cooldown_active']).severity, 'blocked');

// P1.3E executability states: each exec_reason_code → the required Vietnamese label + severity.
assert.strictEqual(execState('ready').label, 'Sẵn sàng');
assert.strictEqual(execState('ready').severity, 'ready');
assert.strictEqual(execState('no_connector').label, 'Chưa kết nối Chrome');
assert.strictEqual(execState('no_connector').severity, 'blocked');
assert.strictEqual(execState('connector_stale').label, 'Mất kết nối Chrome');
assert.strictEqual(execState('pairing_pending').label, 'Đang chờ pair extension');
assert.strictEqual(execState('identity_mismatch').label, 'Sai Facebook profile');
assert.strictEqual(execState('session_blocked').label, 'Session bị chặn/checkpoint');
assert.strictEqual(execState('account_blocked').label, 'Đang bị chặn');
assert.strictEqual(execState('not_controllable').label, 'Bạn không có quyền dùng tài khoản này');
assert.ok(!execState('not_controllable').label.includes('_'), 'exec label must not leak the raw code');
// Unknown / passthrough (e.g. version) code → friendly fallback, never the raw code.
assert.ok(!execState('extension_update_required').label.includes('_'));

// accountExecState: green "Sẵn sàng" ONLY when executable === true.
assert.strictEqual(accountExecState({ executable: true, exec_reason_code: 'ready' }).severity, 'ready');
assert.strictEqual(accountExecState({ executable: true }).label, 'Sẵn sàng');
// Not executable → never green, even if a stale exec_reason_code says 'ready'.
assert.notStrictEqual(accountExecState({ executable: false, exec_reason_code: 'no_connector' }).severity, 'ready');
assert.strictEqual(accountExecState({ executable: false, exec_reason_code: 'identity_mismatch' }).label, 'Sai Facebook profile');
// Missing fields (old client) → safe not-ready default.
assert.strictEqual(accountExecState({}).severity, 'blocked');

console.log('Account Health reasonMessages: PASS');
