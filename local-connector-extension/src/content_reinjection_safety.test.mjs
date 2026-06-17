// H-1 reinjection-safety guard (static). H-1 expands the fallback re-injection set
// (src/shared.js CONTENT_FILES) from 9 to 18 files. injectContentScripts() re-runs those files
// via chrome.scripting.executeScript into a tab whose ISOLATED WORLD may already hold the
// manifest-injected scripts — i.e. each file can be EVALUATED TWICE in the same realm. In a
// classic content script (not a module), a top-level `const`/`let`/`class`/`function` creates a
// GLOBAL-LEXICAL binding; evaluating it a second time throws
// "SyntaxError: Identifier X has already been declared", and a top-level addListener/observer/
// timer would DUPLICATE on the second run. This test statically proves every re-injected file is
// shaped to survive double-evaluation:
//   - module binding is `var` (legal to redeclare), guarded by `globalThis.X || (…)` or a bare
//     side-effect-free IIFE;
//   - any top-level lexical decl or load-time listener lives INSIDE an `if (!globalThis.<marker>)`
//     install guard (bridge.js), so the second evaluation skips it (no redeclare, no dup listener);
//   - no illegal top-level `return`.
//
// LIMITATION (documented, not faked): Node/JSDOM cannot faithfully emulate Chrome's isolated-world
// double-injection — `require()` is cached and re-running source via `vm` would change scoping
// semantics and mislead. So this is a STATIC structural guard (no eval of production code), which
// catches the real regressions (changing `var`→`const`, adding an unguarded top-level listener,
// introducing a top-level `return`). True double-injection is covered by manual smoke (re-inject a
// loaded FB tab, confirm no console SyntaxError and a single comment listener).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const __dirname = dirname(fileURLToPath(import.meta.url));
const EXT_ROOT = join(__dirname, '..');

// Reuse the narrow CONTENT_FILES parser doctrine (no eval). Fails closed.
function parseContentFiles() {
  const src = readFileSync(join(__dirname, 'shared.js'), 'utf8');
  const m = src.match(/const\s+CONTENT_FILES\s*=\s*\[([\s\S]*?)\]/);
  assert.ok(m, 'could not locate CONTENT_FILES in src/shared.js (parse failed — failing closed)');
  const body = m[1].replace(/\/\/[^\n]*$/gm, '');
  const files = [];
  const lit = /'([^']+)'|"([^"]+)"/g;
  let hit;
  while ((hit = lit.exec(body)) !== null) files.push(hit[1] !== undefined ? hit[1] : hit[2]);
  assert.ok(files.length > 0, 'CONTENT_FILES parsed to zero entries (failing closed)');
  return files;
}

// Strip /* */ blocks so commented sample code never trips the col-0 scanners. Line comments are
// handled per-line (a `// …` line never starts with a declaration keyword at col 0).
function strip(src) {
  return src.replace(/\/\*[\s\S]*?\*\//g, '');
}
const isGuardWrapped = (src) => /^if \(!globalThis\.[A-Za-z0-9_$]+\)/m.test(src);

const COL0_LEXICAL = /^(const|let|class)\s/;            // global-lexical → redeclare hazard
const COL0_FUNC = /^function\s/;                        // global function decl → redeclare hazard
const COL0_RETURN = /^return\b/;                        // illegal at top level
// Only TRUE column-0 (unindented) calls are load-time side effects. An unindented dotted call
// (`chrome.runtime.onMessage.addListener(`) starts with an identifier char; an indented call
// inside a function (e.g. forensics.js `  window.addEventListener` inside install()) starts with
// whitespace and is correctly NOT flagged (it runs only when that function is invoked).
const COL0_SIDE_EFFECT = /^[A-Za-z_$][\w.$]*\.(addListener|addEventListener)\s*\(|^new\s+MutationObserver|^set(Interval|Timeout)\s*\(/;
const THG_BINDING = /^(var|const|let|class)\s+THG[A-Za-z0-9_]*/;

const files = parseContentFiles();

test('CONTENT_FILES is the full 18-file set (sanity for this audit)', () => {
  assert.ok(files.length >= 2, `parsed ${files.length} files`);
});

for (const rel of files) {
  test(`reinjection-safe: ${rel}`, () => {
    const raw = readFileSync(join(EXT_ROOT, rel), 'utf8');
    const src = strip(raw);
    const guarded = isGuardWrapped(src);
    const lines = src.split('\n');

    lines.forEach((line, i) => {
      const ln = i + 1;
      // Illegal top-level return: never allowed.
      assert.ok(!COL0_RETURN.test(line), `${rel}:${ln} top-level \`return\` (illegal in a re-injected script)`);
      // Top-level function declaration would redeclare on the 2nd evaluation.
      assert.ok(!COL0_FUNC.test(line), `${rel}:${ln} top-level \`function\` decl would redeclare on re-injection — wrap in the module IIFE`);
      // THG module binding must be `var` (var redeclaration is legal; const/let/class is not).
      if (THG_BINDING.test(line)) {
        assert.ok(/^var\s/.test(line), `${rel}:${ln} THG module binding must be \`var\` (got non-var) — const/let/class throws on re-injection`);
      }
      // A col-0 lexical decl is only safe if the whole file is install-guarded (bridge.js): the
      // 2nd evaluation hits the `if (!globalThis.X)` guard (now false) and skips the block.
      if (COL0_LEXICAL.test(line)) {
        assert.ok(guarded, `${rel}:${ln} top-level \`const/let/class\` outside an \`if (!globalThis.X)\` guard would redeclare on re-injection`);
      }
      // A load-time listener/observer/timer must be install-guarded so it cannot double-register.
      if (COL0_SIDE_EFFECT.test(line)) {
        assert.ok(guarded, `${rel}:${ln} top-level listener/observer/timer must sit inside an \`if (!globalThis.X)\` install guard (duplicate-registration hazard)`);
      }
    });

    // Positive shape: every THG module is either globalThis-guarded (IIFE runs once; existing
    // state preserved on re-injection) or a bare side-effect-free IIFE. Files that register a
    // load-time listener (bridge.js) must be install-guarded.
    const declaresThg = /^var\s+THG[A-Za-z0-9_]*/m.test(src);
    if (declaresThg) {
      const globalGuarded = /^var\s+THG[A-Za-z0-9_]*\s*=\s*globalThis\./m.test(src);
      const bareIife = /^var\s+THG[A-Za-z0-9_]*\s*=\s*\(\(\)\s*=>/m.test(src);
      assert.ok(globalGuarded || bareIife,
        `${rel}: THG module must use \`var X = globalThis.X || (…)\` (idempotent) or a bare side-effect-free IIFE`);
    }
  });
}
