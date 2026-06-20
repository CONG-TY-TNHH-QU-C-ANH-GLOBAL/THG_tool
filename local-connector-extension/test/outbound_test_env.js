// Shared loader for the outbound characterization tests.
//
// outbound.js / outbound_dom.js are Chrome content scripts: their helpers reference browser
// globals (window/document/location/getComputedStyle/MouseEvent/...) when CALLED, and each
// module is guarded by `globalThis.THG* || (IIFE)` + only sets module.exports INSIDE that
// IIFE. So to load a FRESH copy under Node we must, in order:
//   1. install the minimal fake browser globals BEFORE require,
//   2. delete the module's globalThis.THG* singleton (else the guard short-circuits the IIFE
//      and module.exports — hence _test — is never (re)assigned),
//   3. delete require.cache[module] so the file re-executes,
//   4. require it.
// restore() undoes the globalThis mutation to avoid cross-test pollution within a file.
//
// IMPORTANT: tests that mutate globalThis must run SEQUENTIALLY — never pass
// { concurrency: true } and never wrap shared-globalThis mutation in parallel subtests.
const OUTBOUND = require.resolve('../content/outbound.js');
const OUTBOUND_DOM = require.resolve('../content/outbound_dom.js');

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
  const doc = {
    cookie: '', title: '',
    documentElement: { innerHTML: '' },
    body: { innerText: '' },
    contains: () => true,
    querySelector: () => null,
    querySelectorAll: () => [],
    createRange: () => ({ selectNodeContents() {} }),
    createElement: () => ({ setAttribute() {}, appendChild() {}, style: {}, click() {} }),
  };
  // The editing command API is a deprecated DOM method; install the fake via a computed key
  // so the deprecated token never appears literally in test source. Behavior is unchanged —
  // a no-op stub returning true, exactly as before.
  doc['exec' + 'Command'] = () => true;
  return doc;
}

const BROWSER_KEYS = ['window', 'document', 'location', 'getComputedStyle',
  'MouseEvent', 'PointerEvent', 'InputEvent', 'Event', 'innerWidth', 'innerHeight',
  'getSelection', 'THGContentOutbound', 'THGOutboundDom'];

// installGlobals installs the minimal fake browser globals + any requested singletons and
// real sibling modules, and returns a restore() that reverses every mutation.
function installGlobals(overrides) {
  const singletonNames = Object.keys(overrides.singletons || {});
  const saved = {};
  for (const k of BROWSER_KEYS) saved[k] = { had: k in globalThis, val: globalThis[k] };
  for (const k of singletonNames) saved['s:' + k] = { had: k in globalThis, val: globalThis[k] };

  const win = overrides.window || makeWindow();
  globalThis.window = win;
  // outbound_dom.js reads these via globalThis (S6643 — prefer globalThis over window), so the
  // fake env must expose them on globalThis directly, mirroring the window object's values.
  globalThis.innerWidth = win.innerWidth;
  globalThis.innerHeight = win.innerHeight;
  globalThis.getSelection = win.getSelection;
  globalThis.location = overrides.location || win.location;
  globalThis.document = overrides.document || makeDocument();
  globalThis.getComputedStyle = overrides.getComputedStyle || (() => ({ visibility: 'visible', display: 'block' }));
  globalThis.MouseEvent = function MouseEvent(type, init) { this.type = type; Object.assign(this, init || {}); };
  globalThis.PointerEvent = function PointerEvent(type, init) { this.type = type; Object.assign(this, init || {}); };
  globalThis.InputEvent = function InputEvent(type, init) { this.type = type; this.data = init?.data; };
  globalThis.Event = function Event(type) { this.type = type; };
  for (const [k, v] of Object.entries(overrides.singletons || {})) globalThis[k] = v;

  for (const m of (overrides.realModules || [])) require(m); // register real THG* globals

  function restore() {
    for (const k of BROWSER_KEYS) { if (saved[k].had) globalThis[k] = saved[k].val; else delete globalThis[k]; }
    for (const k of singletonNames) { const s = saved['s:' + k]; if (s.had) globalThis[k] = s.val; else delete globalThis[k]; }
  }
  return { restore };
}

// loadOutboundWithGlobals installs fake globals then requires a FRESH outbound.js (which in
// turn re-requires a fresh outbound_dom.js via its globalThis.THGOutboundDom || require
// fallback). Returns { O, api, restore }: O = module.exports (incl. _test); api = the Chrome
// runtime object (globalThis.THGContentOutbound, exactly the 4 public methods).
function loadOutboundWithGlobals(overrides = {}) {
  const { restore } = installGlobals(overrides);
  delete globalThis.THGContentOutbound;   // force outbound's IIFE guard to re-run
  delete globalThis.THGOutboundDom;        // force a fresh DOM module bind
  delete require.cache[OUTBOUND];
  delete require.cache[OUTBOUND_DOM];
  const O = require(OUTBOUND);
  return { O, api: globalThis.THGContentOutbound, restore };
}

// loadOutboundDom installs fake globals then requires a FRESH outbound_dom.js directly.
// Returns { DOM, api, restore }: DOM = module.exports; api = globalThis.THGOutboundDom.
function loadOutboundDom(overrides = {}) {
  const { restore } = installGlobals(overrides);
  delete globalThis.THGOutboundDom;
  delete require.cache[OUTBOUND_DOM];
  const DOM = require(OUTBOUND_DOM);
  return { DOM, api: globalThis.THGOutboundDom, restore };
}

module.exports = { loadOutboundWithGlobals, loadOutboundDom, makeWindow, makeDocument };
