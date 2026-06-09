// automation_tab lifecycle test (incident PR-2). Stubs chrome.tabs to prove the
// reuse contract: live remembered tab → reuse (navigate, no new tab); closed tab →
// null (caller creates a fresh one).
//   Run: node src/automation_tab.test.mjs
import assert from 'node:assert';
import { createRequire } from 'node:module';

const liveTabs = new Set();
const navigations = [];
globalThis.chrome = {
  tabs: {
    get: async (id) => { if (liveTabs.has(id)) return { id, windowId: 100 }; throw new Error('no such tab'); },
    update: async (id, opts) => { navigations.push({ id, url: opts.url }); return { id }; },
  },
};

const require = createRequire(import.meta.url);
const T = require('./automation_tab.js');

// No remembered tab → reuseIfAlive returns null (caller will create one).
assert.strictEqual(await T.reuseIfAlive('https://facebook.com/a'), null);

// Remember a LIVE tab → reuse navigates it, no new tab created.
liveTabs.add(5);
T.remember(5);
assert.ok(await T.isAlive(5));
const reused = await T.reuseIfAlive('https://facebook.com/b');
assert.ok(reused && reused.id === 5, 'should reuse remembered live tab');
assert.deepStrictEqual(navigations.at(-1), { id: 5, url: 'https://facebook.com/b' });

// Remembered tab CLOSED by the user → reuseIfAlive returns null → caller creates new.
liveTabs.delete(5);
assert.strictEqual(await T.isAlive(5), false);
assert.strictEqual(await T.reuseIfAlive('https://facebook.com/c'), null);

console.log('automation_tab: PASS');
