// Shared loader for outbound.js characterization tests.
//
// outbound.js is a Chrome content script: its helpers reference browser globals
// (window/document/location/getComputedStyle/MouseEvent/...) when CALLED, and the module
// is guarded by `globalThis.THGContentOutbound || (IIFE)` + only sets module.exports
// INSIDE that IIFE. So to load a FRESH copy under Node we must, in order:
//   1. install the minimal fake browser globals BEFORE require,
//   2. delete globalThis.THGContentOutbound (else the guard short-circuits the IIFE and
//      module.exports — hence _test — is never (re)assigned),
//   3. delete require.cache[outbound] so the file re-executes,
//   4. require it.
// restore() undoes the globalThis mutation to avoid cross-test pollution within a file.
//
// IMPORTANT: tests that mutate globalThis must run SEQUENTIALLY — never pass
// { concurrency: true } and never wrap shared-globalThis mutation in parallel subtests.
const OUTBOUND = require.resolve('../content/outbound.js');

function makeWindow() {
  const location = { href: 'https://www.facebook.com/' };
  return {
    innerWidth: 1000, innerHeight: 800, scrollY: 0,
    scrollBy() {}, scrollTo() {},
    getSelection: () => ({ removeAllRanges() {}, addRange() {} }),
    location,
  };
}

function makeDocument() {
  return {
    cookie: '', title: '',
    documentElement: { innerHTML: '' },
    body: { innerText: '' },
    contains: () => true,
    querySelector: () => null,
    querySelectorAll: () => [],
    createRange: () => ({ selectNodeContents() {} }),
    execCommand: () => true,
    createElement: () => ({ setAttribute() {}, appendChild() {}, style: {}, click() {} }),
  };
}

const BROWSER_KEYS = ['window', 'document', 'location', 'getComputedStyle',
  'MouseEvent', 'PointerEvent', 'InputEvent', 'Event', 'THGContentOutbound'];

// loadOutboundWithGlobals installs fake globals then requires a FRESH outbound.js.
//   overrides.window / .document / .location / .getComputedStyle — replace the defaults
//   overrides.singletons  — { THGContentProof, THGNavReport, ... } set on globalThis
//   overrides.realModules — sibling module paths (relative to test/) to require first;
//                           each registers its own real globalThis.THG* singleton
// Returns { O, api, restore }: O = module.exports (incl. _test); api = the Chrome
// runtime object (globalThis.THGContentOutbound, exactly the 4 public methods).
function loadOutboundWithGlobals(overrides = {}) {
  const singletonNames = Object.keys(overrides.singletons || {});
  const saved = {};
  for (const k of BROWSER_KEYS) saved[k] = { had: k in globalThis, val: globalThis[k] };
  for (const k of singletonNames) saved['s:' + k] = { had: k in globalThis, val: globalThis[k] };

  const win = overrides.window || makeWindow();
  globalThis.window = win;
  globalThis.location = overrides.location || win.location;
  globalThis.document = overrides.document || makeDocument();
  globalThis.getComputedStyle = overrides.getComputedStyle || (() => ({ visibility: 'visible', display: 'block' }));
  globalThis.MouseEvent = function MouseEvent(type, init) { this.type = type; Object.assign(this, init || {}); };
  globalThis.PointerEvent = function PointerEvent(type, init) { this.type = type; Object.assign(this, init || {}); };
  globalThis.InputEvent = function InputEvent(type, init) { this.type = type; this.data = init?.data; };
  globalThis.Event = function Event(type) { this.type = type; };
  for (const [k, v] of Object.entries(overrides.singletons || {})) globalThis[k] = v;

  for (const m of (overrides.realModules || [])) require(m); // register real THG* globals

  delete globalThis.THGContentOutbound;       // force the IIFE guard to re-run
  delete require.cache[OUTBOUND];          // force the file to re-execute
  const O = require(OUTBOUND);

  function restore() {
    for (const k of BROWSER_KEYS) { if (saved[k].had) globalThis[k] = saved[k].val; else delete globalThis[k]; }
    for (const k of singletonNames) { const s = saved['s:' + k]; if (s.had) globalThis[k] = s.val; else delete globalThis[k]; }
  }
  return { O, api: globalThis.THGContentOutbound, restore };
}

module.exports = { loadOutboundWithGlobals, makeWindow, makeDocument };
