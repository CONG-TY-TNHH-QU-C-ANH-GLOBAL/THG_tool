// Window Respect policy (PR-2 — Browser Window Respect + Persistent Comment Tab).
//
// INVARIANT: the user-owned browser window is sacred. Automation may open / focus /
// navigate / type / submit / verify / report. It must NOT, BY DEFAULT:
//   - close the tab after execution,
//   - minimize the window,
//   - resize / move the window or change its maximized / fullscreen state.
//
// Every flag defaults to the SAFE value (no window management). A debug / operator
// build can flip them, but normal customer automation never touches the window.
var THGWindowPolicy = globalThis.THGWindowPolicy || (() => {
  const policy = {
    autoCloseAfterExecution: false,  // leave the Facebook tab open so the user can inspect
    minimizeAfterExecution: false,   // never minimize the user's window
    manageWindowSize: false,         // never force state:'normal' over a maximized/fullscreen window
  };
  return {
    shouldCloseTabAfterExecution: () => policy.autoCloseAfterExecution === true,
    shouldMinimizeAfterExecution: () => policy.minimizeAfterExecution === true,
    shouldManageWindowSize: () => policy.manageWindowSize === true,
    // focusUpdate returns the chrome.windows.update payload that focuses a window
    // WITHOUT resizing it — only forcing state:'normal' when window management is
    // explicitly enabled (otherwise a maximized window would snap to half-screen).
    focusUpdate: () => (policy.manageWindowSize === true ? { state: 'normal', focused: true } : { focused: true }),
    _policy: policy, // debug override surface only
  };
})();

if (typeof module !== 'undefined' && module.exports) {
  module.exports = THGWindowPolicy;
}
