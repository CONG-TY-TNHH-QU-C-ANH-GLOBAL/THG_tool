// Pure-logic test for the workspace-chat system-message severity mapping.
// Transpiles the self-contained chatSeverity.ts with the installed TypeScript
// compiler (no FE test runner) + asserts — same harness as telegram/logic.test.mjs.
//   Run from frontend/: node src/modules/autoflow/components/views/chatSeverity.test.mjs
import { readFileSync } from 'node:fs';
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const ts = require('typescript');
const src = readFileSync(new URL('./chatSeverity.ts', import.meta.url), 'utf8');
const js = ts.transpileModule(src, { compilerOptions: { module: 'ES2020', target: 'ES2020' } }).outputText;
const S = await import('data:text/javascript;base64,' + Buffer.from(js).toString('base64'));

const sev = S.systemMessageSeverity;

// Running crawl progress is neutral info — the red-progress bug this fixes.
assert.strictEqual(sev('system_crawl_progress', '{"safe_reason_code":""}', true), 'info');
assert.strictEqual(sev('system_crawl_progress', '{"safe_reason_code":"scrolling"}', true), 'info');
assert.strictEqual(sev('system_crawl_progress', '{"safe_reason_code":"duplicate_heavy"}', true), 'info');

// A progress heartbeat that paused on a wall is a warning (human required).
for (const code of ['checkpoint_suspected', 'login_required', 'risk_blocked']) {
  assert.strictEqual(sev('system_crawl_progress', `{"safe_reason_code":"${code}"}`, true), 'warning');
}

// Clean summary exits are success.
for (const reason of ['', 'completed', 'maxItems', 'cursor_match', 'end_of_feed']) {
  assert.strictEqual(sev('system_crawl_summary', `{"exit_reason":"${reason}"}`, true), 'success', `exit ${reason || '(empty)'}`);
}

// Stalled summary exits are warnings — informative, never error-red.
for (const reason of ['duplicate_heavy', 'no_progress', 'no_new_items_after_scroll', 'scroll_not_moving', 'pass_exhausted']) {
  assert.strictEqual(sev('system_crawl_summary', `{"exit_reason":"${reason}"}`, true), 'warning', `exit ${reason}`);
}

// Risk summary exits are warnings per the existing design (operator attention).
for (const reason of ['checkpoint_suspected', 'login_required', 'risk_blocked']) {
  assert.strictEqual(sev('system_crawl_summary', `{"exit_reason":"${reason}"}`, true), 'warning', `exit ${reason}`);
}

// Failures stay error-red.
assert.strictEqual(sev('system_crawl_failure', '{"reason":"chrome_crash"}', false), 'error');

// Other system events keep the generic semantic: red only on failure.
assert.strictEqual(sev('system_comment_posted', '{}', true), 'info');
assert.strictEqual(sev('system_comment_posted', '{}', false), 'error');

// Malformed/empty args never throw and fall back safely.
assert.strictEqual(sev('system_crawl_progress', 'not-json', true), 'info');
assert.strictEqual(sev('system_crawl_summary', '', true), 'success');
assert.strictEqual(sev('system_crawl_summary', '"a-json-string"', true), 'success');

// Every severity has a design-token color, and error maps to the hot token.
assert.deepStrictEqual(Object.keys(S.SEVERITY_COLOR).sort(), ['error', 'info', 'success', 'warning']);
assert.strictEqual(S.SEVERITY_COLOR.error, 'var(--hot)');
assert.strictEqual(S.SEVERITY_COLOR.info, 'var(--info)');

console.log('chatSeverity.test.mjs OK');
