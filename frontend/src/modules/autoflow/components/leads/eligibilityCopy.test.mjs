import { readFileSync } from 'node:fs';
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const ts = require('typescript');
const src = readFileSync(new URL('./eligibilityCopy.ts', import.meta.url), 'utf8');
const js = ts.transpileModule(src, { compilerOptions: { module: 'ES2020', target: 'ES2020' } }).outputText;
const { eligibilityLine } = await import('data:text/javascript;base64,' + Buffer.from(js).toString('base64'));

// No eligibility → no line (engagement still renders without it).
assert.strictEqual(eligibilityLine(undefined), null);

// Eligible → dynamic count line, ok tone.
assert.deepStrictEqual(
  eligibilityLine({ eligibility_state: 'eligible', eligible_actor_count: 3 }),
  { text: 'Có thể comment bằng 3 tài khoản Facebook sẵn sàng.', tone: 'ok' },
);

// no_ready_account → warn tone + backend message.
assert.deepStrictEqual(
  eligibilityLine({ eligibility_state: 'no_ready_account', ineligibility_message_vi: 'Chưa thể comment: chưa có tài khoản Facebook sẵn sàng.' }),
  { text: 'Chưa thể comment: chưa có tài khoản Facebook sẵn sàng.', tone: 'warn' },
);

// coverage_full / already_commented → mute tone, backend message verbatim.
assert.deepStrictEqual(
  eligibilityLine({ eligibility_state: 'coverage_full', ineligibility_message_vi: 'Đã đủ số tài khoản tiếp cận lead này.' }),
  { text: 'Đã đủ số tài khoản tiếp cận lead này.', tone: 'mute' },
);
assert.deepStrictEqual(
  eligibilityLine({ eligibility_state: 'already_commented_by_this_actor', ineligibility_message_vi: 'Các tài khoản Facebook sẵn sàng đều đã comment lead này.' }),
  { text: 'Các tài khoản Facebook sẵn sàng đều đã comment lead này.', tone: 'mute' },
);

// Missing message falls back to a safe default.
assert.strictEqual(eligibilityLine({ eligibility_state: 'coverage_full', ineligibility_message_vi: '' }).text, 'Chưa đủ điều kiện comment.');

console.log('eligibilityCopy mapper: PASS');
