// THGCommentingRung2 — the Rung-2 in-SPA navigation comment EXECUTORS (probeRung2Click +
// executeCommentViaRung2), split verbatim from commenting_outbound.js (Workstream A · PR7):
// move-only, behavior-preserving. executeCommentViaRung2 hands off to the direct executor
// (THGCommentingDirect.executeComment) after the in-SPA nav lands. Consumes THGOutboundDom +
// THGCommentingTarget + THGCommentingDirect. Chrome: globalThis.THGCommentingRung2 (loaded after
// execute/direct.js, before commenting_outbound.js); Node: module.exports.
globalThis.THGCommentingRung2 = globalThis.THGCommentingRung2 || (() => {
  const THGDom = globalThis.THGOutboundDom
    || (typeof require === 'function' ? require('../../dom/outbound_dom.js') : null);
  if (!THGDom) {
    throw new Error('THGOutboundDom is required before execute/rung2.js');
  }
  const { wait, clickLikeUser, waitFor } = THGDom;
  const THGTarget = globalThis.THGCommentingTarget
    || (typeof require === 'function' ? require('../commenting_target.js') : null);
  if (!THGTarget) {
    throw new Error('THGCommentingTarget is required before execute/rung2.js');
  }
  const { extractPostIdFromUrl } = THGTarget;
  const THGDirect = globalThis.THGCommentingDirect
    || (typeof require === 'function' ? require('./direct.js') : null);
  if (!THGDirect) {
    throw new Error('THGCommentingDirect is required before execute/rung2.js');
  }
  const { executeComment } = THGDirect;

  // probeRung2Click implements the content-script half of the Rung-2 probe.
  // From a stable, already-loaded FB page (home), it click-navigates toward the
  // target permalink the way a human clicking a link does — a genuine click on
  // an anchor whose href is the permalink, which FB's delegated client router
  // can intercept and turn into an in-SPA history.pushState (NOT a redirect-
  // eligible top-level load). It returns IMMEDIATELY after the click so the
  // background can measure the post-click URL trajectory (which survives a
  // top-load unload, unlike a content-script timer). No comment is typed.
  function probeRung2Click(message) {
    const targetUrl = String(message?.target_url || message?.targetUrl || '').trim();
    const targetId = extractPostIdFromUrl(targetUrl);
    const entry = location.href || '';
    let anchor = null;
    let method = '';
    if (targetId) {
      anchor = Array.from(document.querySelectorAll('a[href*="/posts/"], a[href*="/permalink/"], a[href*="story_fbid="]'))
        .find(el => String(el.getAttribute('href') || '').includes(targetId)) || null;
    }
    if (anchor) {
      method = 'existing_anchor';
    } else {
      anchor = document.createElement('a');
      anchor.href = targetUrl;
      anchor.setAttribute('role', 'link');
      anchor.textContent = 'thg-nav';
      anchor.style.cssText = 'position:fixed;left:8px;top:8px;width:12px;height:12px;opacity:0.01;z-index:2147483647;';
      document.body.appendChild(anchor);
      method = 'injected_anchor';
    }
    clickLikeUser(anchor);
    return { ok: true, clicked: true, method, entry_url: entry };
  }

  // executeCommentViaRung2 is the REAL delivery on the confirmed Rung-2
  // navigation. The probe proved a genuine anchor click → FB's in-SPA router
  // reaches AND holds on the permalink (no late redirect). So: from the stable
  // home page the content script started on, click-navigate to the permalink
  // (in-SPA, gesture-carrying — survives where chrome.tabs.update is bounced),
  // wait for the URL to land on the post, then hand off to the existing
  // permalink-page executor (executeComment), whose identity checkpoints +
  // proof are unchanged. The whole flow is visible in the user's tab.
  async function executeCommentViaRung2(message) {
    const content = String(message?.content || '').trim();
    if (!content) return { ok: false, error: 'outbox_content_empty' };
    if (content.length > 3000) return { ok: false, error: 'outbox_content_too_long' };
    const targetUrl = String(message?.target_url || message?.targetUrl || '').trim();
    const executionId = String(message?.execution_id || message?.executionId || '').trim();
    const targetId = extractPostIdFromUrl(targetUrl);

    // Progress logs (visible in the FB tab's DevTools Console) so we can see
    // exactly how far the flow got even if the background's response is lost.
    console.log('[THG rung2] start', { target_id: targetId, entry: location.href, execution_id: executionId });
    // Rung-2 in-SPA navigation: genuine anchor click → FB router pushState.
    const click = probeRung2Click({ target_url: targetUrl });
    console.log('[THG rung2] clicked', click);
    // Wait until the in-SPA nav lands on the permalink (URL carries the post id).
    const landed = await waitFor(
      () => !!targetId && (location.href || '').includes(targetId),
      7000, 200
    );
    console.log('[THG rung2] nav landed=', landed, 'url=', location.href);
    if (!landed) {
      return {
        ok: false,
        error: 'nav_redirected',
        proof: {
          success: false,
          failure_reason: 'redirected_feed',
          page_url_after: location.href || '',
          notes: 'c.rung2.nav_did_not_land: target_id=' + (targetId || '?') +
            ' landed_at=' + (location.href || '') +
            ' (in-SPA click nav did not reach the permalink within 7s)',
          execution_id: executionId,
        },
      };
    }
    // Settle for the post + composer to render after the in-SPA route change.
    await wait(900);
    // Hand off to the permalink-page executor (gate-1 confirms the article,
    // identity checkpoints + proof unchanged).
    console.log('[THG rung2] handing off to executeComment on', location.href);
    const r = await executeComment(content, targetUrl, executionId);
    console.log('[THG rung2] executeComment result', r && (r.ok ? 'OK' : (r.error || r.proof?.failure_reason)), r?.proof?.notes);
    return r;
  }

  const api = { probeRung2Click, executeCommentViaRung2 };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  return api;
})();
