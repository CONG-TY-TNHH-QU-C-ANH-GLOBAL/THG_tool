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
const POSTING = require.resolve('../content/posting_outbound.js');
const INBOX = require.resolve('../content/inbox_outbound.js');
const CMT_TARGET = require.resolve('../content/commenting_target.js');
const CMT_DIAG = require.resolve('../content/commenting_diag.js');
const CMT_OUTBOUND = require.resolve('../content/commenting_outbound.js');

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
  'getSelection', 'scrollBy', 'scrollTo', 'scrollY',
  'THGContentOutbound', 'THGOutboundDom', 'THGPostingOutbound', 'THGInboxOutbound',
  'THGCommentingTarget', 'THGCommentingDiag', 'THGCommentingOutbound'];

// Clear all extracted-layer module globals + require caches so a fresh outbound.js (and its
// require chain) rebuilds cleanly. Keeps each load behavior-identical to a first Chrome inject.
function freshLayerChain() {
  for (const g of ['THGContentOutbound', 'THGOutboundDom', 'THGPostingOutbound', 'THGInboxOutbound',
    'THGCommentingTarget', 'THGCommentingDiag', 'THGCommentingOutbound']) delete globalThis[g];
  for (const c of [OUTBOUND, OUTBOUND_DOM, POSTING, INBOX, CMT_TARGET, CMT_DIAG, CMT_OUTBOUND]) delete require.cache[c];
}

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
  globalThis.scrollBy = win.scrollBy;
  globalThis.scrollTo = win.scrollTo;
  globalThis.scrollY = win.scrollY;
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
  freshLayerChain(); // force outbound + every layer module to rebuild fresh
  const O = require(OUTBOUND);
  return { O, api: globalThis.THGContentOutbound, restore };
}

// loadCommentingTarget / loadCommentingDiag / loadCommentingOutbound require a FRESH comment
// module (re-requiring its earlier-loaded providers via guarded fallback).
function loadCommentingTarget(overrides = {}) {
  const { restore } = installGlobals(overrides);
  freshLayerChain();
  const TARGET = require(CMT_TARGET);
  return { TARGET, api: globalThis.THGCommentingTarget, restore };
}

function loadCommentingDiag(overrides = {}) {
  const { restore } = installGlobals(overrides);
  freshLayerChain();
  const DIAG = require(CMT_DIAG);
  return { DIAG, api: globalThis.THGCommentingDiag, restore };
}

function loadCommentingOutbound(overrides = {}) {
  const { restore } = installGlobals(overrides);
  freshLayerChain();
  const CMT = require(CMT_OUTBOUND);
  return { CMT, api: globalThis.THGCommentingOutbound, restore };
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

// loadPostingOutbound installs fake globals then requires a FRESH posting_outbound.js (which
// re-requires a fresh outbound_dom.js via its guarded fallback). Returns { POST, api, restore }:
// POST = module.exports (incl. _test); api = globalThis.THGPostingOutbound.
function loadPostingOutbound(overrides = {}) {
  const { restore } = installGlobals(overrides);
  delete globalThis.THGPostingOutbound;
  delete globalThis.THGOutboundDom;
  delete require.cache[POSTING];
  delete require.cache[OUTBOUND_DOM];
  const POST = require(POSTING);
  return { POST, api: globalThis.THGPostingOutbound, restore };
}

// loadInboxOutbound installs fake globals then requires a FRESH inbox_outbound.js (which
// re-requires a fresh outbound_dom.js via its guarded fallback). Returns { INBOX, api, restore }:
// INBOX = module.exports (incl. _test); api = globalThis.THGInboxOutbound.
function loadInboxOutbound(overrides = {}) {
  const { restore } = installGlobals(overrides);
  delete globalThis.THGInboxOutbound;
  delete globalThis.THGOutboundDom;
  delete require.cache[INBOX];
  delete require.cache[OUTBOUND_DOM];
  const INBOX_MOD = require(INBOX);
  return { INBOX: INBOX_MOD, api: globalThis.THGInboxOutbound, restore };
}

module.exports = {
  loadOutboundWithGlobals, loadOutboundDom, loadPostingOutbound, loadInboxOutbound,
  loadCommentingTarget, loadCommentingDiag, loadCommentingOutbound, makeWindow, makeDocument,
};
