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
const { mapReason, pickPrimaryReason, overallStatus } = mod;

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

console.log('Account Health reasonMessages: PASS');
