// Pure-logic tests for the workspace-chat system-message severity mapping.
// Transpiles the self-contained chatSeverity.ts with the installed TypeScript
// compiler (no FE test runner) — same harness as telegram/logic.test.mjs, but
// with explicit node:test cases so tooling recognizes them as tests.
//   Run from frontend/: node --test src/modules/autoflow/components/views/chatSeverity.test.mjs
import { test } from 'node:test';
import { readFileSync } from 'node:fs';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const ts = require('typescript');
const src = readFileSync(new URL('./chatSeverity.ts', import.meta.url), 'utf8');
const js = ts.transpileModule(src, { compilerOptions: { module: 'ES2020', target: 'ES2020' } }).outputText;
const S = await import('data:text/javascript;base64,' + Buffer.from(js).toString('base64'));

const sev = S.systemMessageSeverity;

test('running crawl progress is neutral info — the red-progress bug this fixes', () => {
  assert.equal(sev('system_crawl_progress', '{"safe_reason_code":""}', true), 'info');
  assert.equal(sev('system_crawl_progress', '{"safe_reason_code":"scrolling"}', true), 'info');
  assert.equal(sev('system_crawl_progress', '{"safe_reason_code":"duplicate_heavy"}', true), 'info');
});

test('a progress heartbeat paused on a wall is a warning (human required)', () => {
  for (const code of ['checkpoint_suspected', 'login_required', 'risk_blocked']) {
    assert.equal(sev('system_crawl_progress', `{"safe_reason_code":"${code}"}`, true), 'warning', code);
  }
});

test('clean summary exits are success', () => {
  for (const reason of ['', 'completed', 'maxItems', 'cursor_match', 'end_of_feed']) {
    assert.equal(sev('system_crawl_summary', `{"exit_reason":"${reason}"}`, true), 'success', `exit ${reason || '(empty)'}`);
  }
});

test('stalled summary exits are warnings — informative, never error-red', () => {
  for (const reason of ['duplicate_heavy', 'no_progress', 'no_new_items_after_scroll', 'scroll_not_moving', 'pass_exhausted']) {
    assert.equal(sev('system_crawl_summary', `{"exit_reason":"${reason}"}`, true), 'warning', `exit ${reason}`);
  }
});

test('risk summary exits are warnings (operator attention, no auto-handling)', () => {
  for (const reason of ['checkpoint_suspected', 'login_required', 'risk_blocked']) {
    assert.equal(sev('system_crawl_summary', `{"exit_reason":"${reason}"}`, true), 'warning', `exit ${reason}`);
  }
});

test('crawl failures stay error-red', () => {
  assert.equal(sev('system_crawl_failure', '{"reason":"chrome_crash"}', false), 'error');
});

test('other system events keep the generic semantic: red only on failure', () => {
  assert.equal(sev('system_comment_posted', '{}', true), 'info');
  assert.equal(sev('system_comment_posted', '{}', false), 'error');
});

test('malformed or empty args never throw and fall back safely', () => {
  assert.equal(sev('system_crawl_progress', 'not-json', true), 'info');
  assert.equal(sev('system_crawl_summary', '', true), 'success');
  assert.equal(sev('system_crawl_summary', '"a-json-string"', true), 'success');
});

test('every severity has a design-token color; error maps to the hot token', () => {
  assert.deepEqual(Object.keys(S.SEVERITY_COLOR).sort(), ['error', 'info', 'success', 'warning']);
  assert.equal(S.SEVERITY_COLOR.error, 'var(--hot)');
  assert.equal(S.SEVERITY_COLOR.info, 'var(--info)');
});
