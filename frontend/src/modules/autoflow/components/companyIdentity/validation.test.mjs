// Pure-validator test for the Company Identity form. Transpiles the self-contained
// validation.ts with the installed TypeScript compiler (no FE test runner) + asserts.
//   Run from frontend/: node src/modules/autoflow/components/companyIdentity/validation.test.mjs
import { readFileSync } from 'node:fs';
import assert from 'node:assert';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const ts = require('typescript');
const src = readFileSync(new URL('./validation.ts', import.meta.url), 'utf8');
const js = ts.transpileModule(src, { compilerOptions: { module: 'ES2020', target: 'ES2020' } }).outputText;
const { normalizeWebsite, validateWebsite, validateContact, validateCta } =
  await import('data:text/javascript;base64,' + Buffer.from(js).toString('base64'));

// normalizeWebsite: bare domain → https://, scheme preserved, empty → ''.
assert.strictEqual(normalizeWebsite('thgfulfill.com'), 'https://thgfulfill.com');
assert.strictEqual(normalizeWebsite('https://thgfulfill.com'), 'https://thgfulfill.com');
assert.strictEqual(normalizeWebsite('  '), '');

// validateWebsite: empty OK, valid OK, junk rejected.
assert.strictEqual(validateWebsite(''), null);
assert.strictEqual(validateWebsite('thgfulfill.com'), null);
assert.strictEqual(validateWebsite('https://thg.vn/abc'), null);
assert.ok(validateWebsite('not a url'));        // has space → URL() throws or no dot
assert.ok(validateWebsite('justtext'));         // no dot in hostname

// validateContact: lenient — telegram/zalo/phone/plain text/email OK; lone bad email rejected.
for (const ok of ['', 't.me/thgfulfill', 'Zalo: 0901234567', '0901234567', 'inbox shop nhé', 'a@b.com']) {
  assert.strictEqual(validateContact(ok), null, `contact should pass: "${ok}"`);
}
assert.ok(validateContact('broken@email'));     // looks like email, malformed

// validateCta: empty OK, normal OK, too long / multi-link rejected.
assert.strictEqual(validateCta('Inbox mình nhé'), null);
assert.ok(validateCta('x'.repeat(201)));
assert.ok(validateCta('xem https://a.com và https://b.com'));

console.log('Company Identity validation: PASS');
