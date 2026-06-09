// automation_tab lifecycle test (incident PR-2 + PR-2.1). Stubs chrome.tabs +
// chrome.storage.session to prove: the remembered tab id is PERSISTED (survives an
// MV3 service-worker restart), a live tab is reused (navigated, no new tab), and a
// closed tab clears the stored id so the caller creates a fresh one.
//   Run: node src/automation_tab.test.mjs
import assert from 'node:assert';
import { createRequire } from 'node:module';

const liveTabs = new Set();
const navigations = [];
const sessionStore = {};
globalThis.chrome = {
  tabs: {
    get: async (id) => { if (liveTabs.has(id)) return { id, windowId: 100 }; throw new Error('no such tab'); },
    update: async (id, opts) => { navigations.push({ id, url: opts.url }); return { id }; },
  },
  storage: {
    session: {
      get: async (key) => ({ [key]: sessionStore[key] }),
      set: async (obj) => { Object.assign(sessionStore, obj); },
      remove: async (key) => { delete sessionStore[key]; },
    },
  },
};

const require = createRequire(import.meta.url);
const T = require('./automation_tab.js');

// No remembered tab → reuseIfAlive returns null (caller will create one).
assert.strictEqual(await T.reuseIfAlive('https://facebook.com/a'), null);

// remember PERSISTS to storage; a live remembered tab is reused (navigated).
liveTabs.add(5);
await T.remember(5);
assert.strictEqual(await T.getRemembered(), 5, 'id persisted to storage');
assert.strictEqual(sessionStore.thg_automation_tab_id, 5, 'written to session storage');
const reused = await T.reuseIfAlive('https://facebook.com/b');
assert.ok(reused && reused.id === 5, 'should reuse remembered live tab');
assert.deepStrictEqual(navigations.at(-1), { id: 5, url: 'https://facebook.com/b' });

// PR-2.1: the module keeps NO in-memory id — it reads storage each time, so a
// service-worker restart (which wipes module memory, not storage) still resolves it.
assert.strictEqual(await T.getRemembered(), 5, 'survives SW restart via storage');

// Tab closed by the user → reuseIfAlive clears the stale id + returns null.
liveTabs.delete(5);
assert.strictEqual(await T.isAlive(5), false);
assert.strictEqual(await T.reuseIfAlive('https://facebook.com/c'), null);
assert.strictEqual(await T.getRemembered(), 0, 'stale id cleared after tab closed');
assert.strictEqual(sessionStore.thg_automation_tab_id, undefined, 'storage cleared');

console.log('automation_tab: PASS');
