// THG Facebook crawl telemetry/policy — pure helpers extracted from
// content/crawl.js (PR-C1B follow-up) so the crawl entrypoint stays a thin
// bridge. Everything here is a pure function of its arguments:
//   - No DOM/page text ever enters a payload (diagnostics are projected
//     field-by-field — whitelist by construction).
//   - Risk detection is DEPENDENCY-INJECTED (navReport / contentProof passed
//     in), so this module has no hard dependency on sibling content scripts and
//     is unit-testable in node without stubbing globals.
// It introduces NO new detection heuristics — it only reuses the classifiers
// the caller injects (THGNavReport.classifyLanding / THGContentProof.detectPlatformReject).
var THGCrawlProgress = globalThis.THGCrawlProgress || (() => {
  // Coarse activity bucket for each safe reason code — lets the operator glance
  // "scrolling / stalled / blocked / done" without parsing the finer code.
  const CRAWL_PHASE_OF = {
    scrolling: 'scrolling',
    no_new_posts: 'stalled', duplicate_heavy: 'stalled', scroll_not_moving: 'stalled',
    login_required: 'blocked', checkpoint_suspected: 'blocked', risk_blocked: 'blocked',
    wrong_page: 'blocked', completed: 'completed', unknown: 'unknown',
  };

  // Maps a raw risk signal from the reused classifiers to its stable reason
  // code. '' when no risk. Shared by the loop's graceful-stop and the classifier.
  function crawlRiskToReason(risk) {
    if (risk === 'login') return 'login_required';
    if (risk === 'checkpoint') return 'checkpoint_suspected';
    if (risk === 'rate_limited' || risk === 'blocked') return 'risk_blocked';
    return '';
  }

  // Flat reason picker (Sonar-friendly early returns). Risk always wins so a
  // checkpoint/login/block is never masked by a "scrolling" label.
  function pickCrawlReasonCode(s) {
    const risk = crawlRiskToReason(s.risk);
    if (risk) return risk;
    if (s.done && s.reachedMax) return 'completed';
    if (s.scrollCount > 0 && !s.scrollMovedEver) return 'scroll_not_moving';
    if (s.newCount === 0 && s.duplicateCount >= 3) return 'duplicate_heavy';
    if (s.newCount === 0 && s.noProgressRounds > 0) return 'no_new_posts';
    return 'scrolling';
  }

  // Pure classifier: given the loop's already-computed counters + a risk signal,
  // name WHAT is happening as a stable {phase, safe_reason_code} (never raw page
  // text). Observability only — it does NOT decide when the loop stops.
  // s: { risk, newCount, duplicateCount, scrollCount, noProgressRounds,
  //      scrollMovedEver, done, reachedMax }
  function classifyCrawlProgress(s) {
    const code = pickCrawlReasonCode(s);
    return { phase: CRAWL_PHASE_OF[code] || 'unknown', safe_reason_code: code };
  }

  // Zero-counter diagnostics for a stop before any scanning (entry-time wall).
  function zeroCrawlDiag(risk) {
    const c = classifyCrawlProgress({ risk, newCount: 0, duplicateCount: 0, scrollCount: 0, noProgressRounds: 0, scrollMovedEver: false, done: true, reachedMax: false });
    return {
      phase: c.phase, found_count: 0, new_count: 0, duplicate_count: 0,
      scroll_count: 0, no_progress_rounds: 0, scroll_moved_ever: false,
      seconds_since_last_new: 0, safe_reason_code: c.safe_reason_code,
    };
  }

  // Pure builder for the thg_crawl_progress payload. crawlerVersion is injected
  // (single source of truth stays in crawl.js). Diagnostics are projected FIELD
  // BY FIELD (whitelist by construction) so no raw page text / DOM / secret can
  // ever leak. Omit diag → byte-identical to the pre-diagnostics shape.
  function buildCrawlProgressMessage(crawlerVersion, task, accountId, stage, fetched, max, sourceUrl, diag) {
    const msg = {
      type: 'thg_crawl_progress',
      crawler_version: crawlerVersion,
      task_id: task?.task_id || '',
      intent: task?.intent || 'facebook_crawl',
      account_id: accountId || 0,
      stage,
      fetched,
      max,
      source_url: sourceUrl
    };
    if (diag) {
      msg.phase = diag.phase;
      msg.found_count = diag.found_count;
      msg.new_count = diag.new_count;
      msg.duplicate_count = diag.duplicate_count;
      msg.scroll_count = diag.scroll_count;
      msg.no_progress_rounds = diag.no_progress_rounds;
      msg.scroll_moved_ever = diag.scroll_moved_ever;
      msg.seconds_since_last_new = diag.seconds_since_last_new;
      msg.safe_reason_code = diag.safe_reason_code;
    }
    return msg;
  }

  // Cheap URL risk probe (no DOM scan). navReport is injected (crawl.js passes
  // globalThis.THGNavReport). Returns '' | 'login' | 'checkpoint'.
  function detectCrawlRisk(navReport, href) {
    if (navReport && typeof navReport.classifyLanding === 'function') {
      const cls = navReport.classifyLanding(href);
      if (cls === 'login' || cls === 'checkpoint') return cls;
    }
    return '';
  }

  // Text-banner risk probe. contentProof is injected (crawl.js passes
  // globalThis.THGContentProof). Reads body text, so the caller only invokes it
  // on a zero-post pass. Returns '' | 'rate_limited' | 'blocked' | 'checkpoint'.
  function detectCrawlBanner(contentProof) {
    if (contentProof && typeof contentProof.detectPlatformReject === 'function') {
      return contentProof.detectPlatformReject() || '';
    }
    return '';
  }

  return {
    CRAWL_PHASE_OF, crawlRiskToReason, pickCrawlReasonCode, classifyCrawlProgress,
    zeroCrawlDiag, buildCrawlProgressMessage, detectCrawlRisk, detectCrawlBanner,
  };
})();
globalThis.THGCrawlProgress = THGCrawlProgress;
// CommonJS export for the node test harness. No-op in the extension.
if (typeof module !== 'undefined' && module.exports) module.exports = THGCrawlProgress;
