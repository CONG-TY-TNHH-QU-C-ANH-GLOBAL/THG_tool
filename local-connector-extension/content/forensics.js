/*
 * THG Connector Extension — PR8C-Forensics: content-script interaction recorder.
 *
 * Purpose (operator directive, 2026-06-04): the evidence proves Facebook fires a
 * `history.pushState` → home ~100ms AFTER our content script attaches and starts
 * working on the permalink. It does NOT yet prove WHAT we did triggers it. Before
 * any technology change (mbasic / GraphQL / Playwright), we must name the exact
 * last operation our content script performed before the reset — or exhaustively
 * rule the content-script interaction model out.
 *
 * This module is a forensic recorder, NOT a behaviour change. It:
 *   1. Monkey-patches the ISOLATED world's DOM primitives (querySelectorAll,
 *      querySelector, click, focus, dispatchEvent, innerHTML/innerText getters,
 *      MutationObserver.observe) so EVERY DOM op OUR content script performs is
 *      timestamped. Isolated-world patches affect only our code — the page's own
 *      (MAIN-world) calls are untouched — so the timeline is exactly our activity.
 *   2. Listens for `THG_FORENSIC_PUSHSTATE` window messages posted by the
 *      MAIN-world history interceptor (injected from src/outbox.js), which carry
 *      the precise pushState/replaceState timestamp + FB's stack trace.
 *   3. snapshot() correlates the two on one wall clock (Date.now) and reports the
 *      LAST op before the first home/feed pushState + the FB stack at that reset.
 *
 * Every patch calls through to the original and returns its value unchanged;
 * recording is wrapped in try/catch so instrumentation can never break delivery.
 * Patches are installed only for the duration of one executeComment (install →
 * snapshot → uninstall), so there is no steady-state overhead.
 */
var THGForensics = globalThis.THGForensics || (() => {
  const RING = 600;
  let armed = false;
  let entryTs = 0;
  let timeline = [];      // READ ops (querySelectorAll/innerText/...) — high volume
  let actions = [];       // MUTATING ops (click/focus/dispatchEvent/observe) — few, never flooded
  const counts = {};      // op → total count (exact, unbounded)
  const pushStates = [];  // { ts, url, method, stack } absolute wall clock
  const saved = {};       // originals for uninstall

  // PR8D: the proof-collection phase (snapshotCommentCount + findCommentNode)
  // fires hundreds of innerText/querySelectorAll READS, which flooded the single
  // ring and pushed the few interesting CLICKS out of the snapshot. Route
  // mutating ops to a separate `actions` buffer so the click that (e.g.) opened
  // a picker is ALWAYS visible regardless of read volume.
  const ACTION_OPS = { click: 1, focus: 1, dispatchEvent: 1, 'MutationObserver.observe': 1 };

  const now = () => Date.now();

  function rec(op, detail) {
    counts[op] = (counts[op] || 0) + 1;
    if (!armed) return;
    const entry = { t: now() - entryTs, op, detail: detail == null ? '' : String(detail).slice(0, 120) };
    if (ACTION_OPS[op]) {
      actions.push(entry);
      if (actions.length > 150) actions.splice(0, actions.length - 150);
    } else {
      timeline.push(entry);
      if (timeline.length > RING) timeline.splice(0, timeline.length - RING);
    }
  }

  function tagDesc(el) {
    if (!el || !el.tagName) return String((el && el.nodeName) || '');
    let s = el.tagName.toLowerCase();
    try {
      const role = el.getAttribute && el.getAttribute('role');
      if (role) s += '[role=' + role + ']';
      const al = el.getAttribute && el.getAttribute('aria-label');
      if (al) s += '[al=' + String(al).slice(0, 24) + ']';
    } catch (_) {}
    return s;
  }

  // ── MAIN-world pushState bridge ─────────────────────────────────────────
  // The injected MAIN-world patch (src/outbox.js forensicPatchMain) posts every
  // history.pushState/replaceState/popstate here with FB's stack trace.
  window.addEventListener('message', (ev) => {
    if (ev.source !== window) return;
    const d = ev.data;
    if (!d || d.source !== 'THG_FORENSIC_PUSHSTATE') return;
    pushStates.push({
      ts: d.ts || now(),
      url: String(d.url || ''),
      method: String(d.method || 'pushState'),
      stack: String(d.stack || '').slice(0, 700),
    });
    if (pushStates.length > 40) pushStates.splice(0, pushStates.length - 40);
  }, true);

  function patch() {
    const protos = [Document.prototype, Element.prototype, DocumentFragment.prototype];
    saved.protos = protos;
    saved.qsa = protos.map(p => p.querySelectorAll);
    saved.qs = protos.map(p => p.querySelector);
    protos.forEach((p) => {
      const origAll = p.querySelectorAll;
      p.querySelectorAll = function (sel) {
        const r = origAll.apply(this, arguments);
        try { rec('querySelectorAll', sel + ' →' + (r ? r.length : 0)); } catch (_) {}
        return r;
      };
      const origOne = p.querySelector;
      p.querySelector = function (sel) {
        const r = origOne.apply(this, arguments);
        try { rec('querySelector', sel + ' →' + (r ? 1 : 0)); } catch (_) {}
        return r;
      };
    });

    saved.click = HTMLElement.prototype.click;
    HTMLElement.prototype.click = function () {
      try { rec('click', tagDesc(this)); } catch (_) {}
      return saved.click.apply(this, arguments);
    };

    saved.focus = HTMLElement.prototype.focus;
    HTMLElement.prototype.focus = function () {
      try { rec('focus', tagDesc(this)); } catch (_) {}
      return saved.focus.apply(this, arguments);
    };

    saved.dispatch = EventTarget.prototype.dispatchEvent;
    EventTarget.prototype.dispatchEvent = function (e) {
      try { rec('dispatchEvent', ((e && e.type) || '') + ' ' + tagDesc(this)); } catch (_) {}
      return saved.dispatch.apply(this, arguments);
    };

    saved.innerHTMLDesc = Object.getOwnPropertyDescriptor(Element.prototype, 'innerHTML');
    if (saved.innerHTMLDesc && saved.innerHTMLDesc.get) {
      const origGet = saved.innerHTMLDesc.get;
      Object.defineProperty(Element.prototype, 'innerHTML', {
        configurable: true,
        enumerable: saved.innerHTMLDesc.enumerable,
        set: saved.innerHTMLDesc.set,
        get: function () {
          const v = origGet.call(this);
          try { counts.innerHTML_bytes = (counts.innerHTML_bytes || 0) + (v ? v.length : 0); rec('innerHTML.get', (this.tagName || '') + ' ' + (v ? v.length : 0) + 'B'); } catch (_) {}
          return v;
        },
      });
    }

    saved.innerTextDesc = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'innerText');
    if (saved.innerTextDesc && saved.innerTextDesc.get) {
      const origGet = saved.innerTextDesc.get;
      Object.defineProperty(HTMLElement.prototype, 'innerText', {
        configurable: true,
        enumerable: saved.innerTextDesc.enumerable,
        set: saved.innerTextDesc.set,
        get: function () {
          const v = origGet.call(this);
          try { counts.innerText_bytes = (counts.innerText_bytes || 0) + (v ? v.length : 0); rec('innerText.get', (this.tagName || '') + ' ' + (v ? v.length : 0) + 'B'); } catch (_) {}
          return v;
        },
      });
    }

    if (window.MutationObserver) {
      saved.observe = MutationObserver.prototype.observe;
      MutationObserver.prototype.observe = function () {
        try { rec('MutationObserver.observe', tagDesc(arguments[0])); } catch (_) {}
        return saved.observe.apply(this, arguments);
      };
    }
  }

  function unpatch() {
    try {
      if (saved.protos) {
        saved.protos.forEach((p, i) => { p.querySelectorAll = saved.qsa[i]; p.querySelector = saved.qs[i]; });
      }
      if (saved.click) HTMLElement.prototype.click = saved.click;
      if (saved.focus) HTMLElement.prototype.focus = saved.focus;
      if (saved.dispatch) EventTarget.prototype.dispatchEvent = saved.dispatch;
      if (saved.innerHTMLDesc) Object.defineProperty(Element.prototype, 'innerHTML', saved.innerHTMLDesc);
      if (saved.innerTextDesc) Object.defineProperty(HTMLElement.prototype, 'innerText', saved.innerTextDesc);
      if (saved.observe && window.MutationObserver) MutationObserver.prototype.observe = saved.observe;
    } catch (_) {}
  }

  function isHomeish(u) {
    try {
      const x = new URL(u, location.href);
      const p = x.pathname.replace(/\/+$/, '');
      return p === '' || p === '/' || p === '/home.php' || p === '/feed' || p === '/feed.php';
    } catch { return false; }
  }

  function install() {
    if (armed) uninstall();
    entryTs = now();
    timeline = [];
    actions = [];
    for (const k in counts) delete counts[k];
    try { patch(); armed = true; } catch (_) { armed = false; }
  }

  function uninstall() {
    if (!armed) return;
    armed = false;
    unpatch();
  }

  function isArmed() { return armed; }

  // snapshot correlates our op timeline with the MAIN-world pushState events.
  // last_op_before_reset = the final op we performed at/before the first
  // home/feed pushState that fired after entry — i.e. the prime suspect.
  function snapshot() {
    const homePs = pushStates
      .filter(p => p.ts >= entryTs && isHomeish(p.url))
      .sort((a, b) => a.ts - b.ts)[0] || null;
    const resetT = homePs ? (homePs.ts - entryTs) : 0;
    // last_op_before_reset is the last MUTATING action (click/focus/dispatch)
    // at/before the reset — the trigger candidate. Actions are kept in their own
    // un-flooded buffer so this survives a heavy proof-read phase.
    let lastOp = null;
    if (homePs) {
      for (const e of actions) {
        if (e.t <= resetT) lastOp = e; else break;
      }
    } else if (actions.length) {
      lastOp = actions[actions.length - 1];
    }
    return {
      entry_ts: entryTs,
      counts: { ...counts },
      actions: actions.slice(-100),
      timeline: timeline.slice(-50),
      push_states: pushStates
        .filter(p => p.ts >= entryTs - 500)
        .slice(-10)
        .map(p => ({ t: p.ts - entryTs, url: p.url, method: p.method, stack: String(p.stack).slice(0, 500) })),
      reset_t_ms: resetT,
      reset_stack: homePs ? String(homePs.stack).slice(0, 500) : '',
      last_op_before_reset: lastOp,
    };
  }

  return { install, uninstall, isArmed, snapshot, mark: rec };
})();
globalThis.THGForensics = THGForensics;
