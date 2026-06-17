// H-1 regression guard. The fallback re-injection list `CONTENT_FILES` in
// src/shared.js MUST stay a byte-for-byte, same-order mirror of the manifest
// content_scripts[0].js array. If they drift, a re-injected Facebook tab loads
// bridge.js while the comment-execution chain (THGExecDedup, THGCommentExecutor,
// THGCommentSM, …) is missing → ReferenceError / outbound_not_ready on the
// fallback delivery path. This test reads both files STATICALLY (fs.readFileSync
// + a narrow parser) — it never imports/evals the production runtime files, so
// it does not depend on, or force, any module format. It FAILS CLOSED: any parse
// ambiguity is a test failure, not a silent pass.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const __dirname = dirname(fileURLToPath(import.meta.url));
const MANIFEST_PATH = join(__dirname, '..', 'manifest.json');
const SHARED_PATH = join(__dirname, 'shared.js');

// readManifestContentScripts returns the ordered js[] of the single Facebook
// content_scripts entry. Fails closed if the manifest shape is not exactly what
// the runtime relies on (one content_scripts entry with a non-empty js array).
function readManifestContentScripts() {
  const raw = readFileSync(MANIFEST_PATH, 'utf8');
  const manifest = JSON.parse(raw); // throws on malformed JSON → fail closed
  const entries = manifest.content_scripts;
  assert.ok(Array.isArray(entries) && entries.length === 1,
    `manifest.content_scripts must be a single-entry array (got ${Array.isArray(entries) ? entries.length : typeof entries})`);
  const js = entries[0] && entries[0].js;
  assert.ok(Array.isArray(js) && js.length > 0,
    'manifest.content_scripts[0].js must be a non-empty array');
  for (const f of js) {
    assert.equal(typeof f, 'string', `manifest js entry is not a string: ${JSON.stringify(f)}`);
  }
  return js;
}

// parseContentFilesArray extracts the CONTENT_FILES string literals from
// src/shared.js WITHOUT importing or evaluating the file. Strategy:
//   1. locate `const CONTENT_FILES = [ … ]` (non-greedy to the first closing
//      bracket — the array body holds only string literals, no nested brackets);
//   2. strip `//` line comments from the captured body so an inline comment can
//      never inject a false path (file paths are relative, never contain `//`);
//   3. collect every quoted literal in source order.
// Any failure to confidently locate/parse the block throws → fail closed.
function parseContentFilesArray() {
  const src = readFileSync(SHARED_PATH, 'utf8');
  const m = src.match(/const\s+CONTENT_FILES\s*=\s*\[([\s\S]*?)\]/);
  assert.ok(m, 'could not locate `const CONTENT_FILES = [ … ]` in src/shared.js (parse failed — failing closed)');
  const body = m[1].replace(/\/\/[^\n]*$/gm, ''); // drop line comments (paths never contain `//`)
  const files = [];
  const lit = /'([^']+)'|"([^"]+)"/g;
  let hit;
  while ((hit = lit.exec(body)) !== null) {
    files.push(hit[1] !== undefined ? hit[1] : hit[2]);
  }
  assert.ok(files.length > 0, 'CONTENT_FILES parsed to zero entries (parse failed — failing closed)');
  return files;
}

test('CONTENT_FILES parses to a concrete non-empty list', () => {
  const files = parseContentFilesArray();
  assert.ok(files.length >= 2, `expected a real content-script list, parsed ${files.length}`);
});

test('CONTENT_FILES contains no missing or extra files vs manifest', () => {
  const manifest = readManifestContentScripts();
  const content = parseContentFilesArray();
  const missing = manifest.filter((f) => !content.includes(f)); // in manifest, absent from re-inject
  const extra = content.filter((f) => !manifest.includes(f));   // in re-inject, absent from manifest
  assert.deepEqual(missing, [],
    `CONTENT_FILES is MISSING manifest content scripts (re-injection would load an incomplete chain): ${JSON.stringify(missing)}`);
  assert.deepEqual(extra, [],
    `CONTENT_FILES has EXTRA files not in the manifest: ${JSON.stringify(extra)}`);
});

test('CONTENT_FILES matches manifest content scripts exactly and in the same order', () => {
  const manifest = readManifestContentScripts();
  const content = parseContentFilesArray();
  assert.equal(content.length, manifest.length,
    `count mismatch: CONTENT_FILES=${content.length} vs manifest=${manifest.length}`);
  // Pinpoint the first out-of-order slot for a precise failure message.
  for (let i = 0; i < manifest.length; i++) {
    assert.equal(content[i], manifest[i],
      `order/content mismatch at index ${i}: CONTENT_FILES[${i}]=${JSON.stringify(content[i])} but manifest[${i}]=${JSON.stringify(manifest[i])}`);
  }
  assert.deepEqual(content, manifest, 'CONTENT_FILES must equal manifest content_scripts[0].js exactly');
});
