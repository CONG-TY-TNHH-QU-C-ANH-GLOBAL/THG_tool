// Minimal fake DOM for gate1 composer/button unit tests — no jsdom. The production modules
// are pure + injected-deps by design, so plain objects exercising getAttribute / tagName /
// getBoundingClientRect / querySelector(All) / contains are enough.

// makeEl builds a fake editable/button node. Empty attrs return '' (matching real getAttribute
// semantics the modules rely on via `|| ''`).
function makeEl({ tag = 'DIV', role = '', aria = '', placeholder = '', ce = '', parentText = '', w = 450, h = 20 } = {}) {
  const attrs = { role, 'aria-label': aria, placeholder, contenteditable: ce };
  return {
    tagName: tag,
    parentElement: parentText ? { textContent: parentText } : null,
    getAttribute: (n) => (n in attrs ? attrs[n] : null),
    getBoundingClientRect: () => ({ width: w, height: h, left: 0, top: 0 }),
  };
}

// makeArticle dispatches the exact selectors the modules query. `editables` answers EDITABLE_SEL
// (in-article subtree); `buttons` answers BUTTON_SEL; `contains` reports subtree membership;
// `permalink` toggles the post-shape anchor.
function makeArticle({ buttons = [], editables = [], contenteditables = [], permalink = true, containsList = [] } = {}) {
  return {
    contains: (el) => containsList.indexOf(el) !== -1,
    querySelector: (sel) => (permalink && sel.indexOf('/posts/') !== -1 ? { tagName: 'A' } : null),
    querySelectorAll: (sel) => {
      if (sel === '[contenteditable="true"]') return contenteditables;
      if (sel.indexOf('role="textbox"') !== -1) return editables;   // EDITABLE_SEL
      if (sel.indexOf('role="button"') !== -1) return buttons;      // BUTTON_SEL
      return [];
    },
  };
}

// alwaysVisible / sizeVisible helpers for deps.visible injection.
const alwaysVisible = () => true;
const sizeVisible = (el) => {
  const r = el.getBoundingClientRect();
  return r.width > 8 && r.height > 8;
};

module.exports = { makeEl, makeArticle, alwaysVisible, sizeVisible };
