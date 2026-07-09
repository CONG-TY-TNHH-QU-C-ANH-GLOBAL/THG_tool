// THG Facebook crawl — result shaping. Pure builders for the crawl_result wire
// object extracted from content/crawl.js (PR-C1C). Byte-identical to the prior
// inline literals; all impure inputs (crawler version, landed URL) are injected.
// This is the wire contract to /api/connectors/crawl-result — shape is pinned
// by characterization tests; do not change field names/values here.
globalThis.THGCrawlResult = globalThis.THGCrawlResult || (() => {
  // scroll_diag sub-object shared by both result shapes.
  function buildScrollDiag(d) {
    return {
      passes: d.passes,
      max_articles_seen: d.maxArticlesSeen,
      max_scroll_y: d.maxScrollY,
      max_doc_height: d.maxDocHeight,
      scroll_moved_ever: d.scrollMovedEver,
      final_scroll_target: d.finalScrollTarget,
      landed_url: d.landedUrl,
    };
  }

  // Direct-post result. direct_post scroll_diag is always zeroed except passes /
  // max_articles_seen / landed_url (the target loop does not track scroll forensics).
  // A non-empty error drives the backend's typed direct-post failure.
  function buildDirectPostResult(d) {
    const cr = {
      crawler_version: d.crawlerVersion,
      task_id: d.task?.task_id || '',
      intent: d.task?.intent || 'facebook_crawl',
      keywords: [],
      items: d.items,
      exit_reason: d.exitReason,
      direct_post: true,
      scroll_diag: buildScrollDiag({
        passes: d.passes, maxArticlesSeen: d.maxArticles, maxScrollY: 0, maxDocHeight: 0,
        scrollMovedEver: false, finalScrollTarget: '', landedUrl: d.landedUrl,
      }),
    };
    if (d.error) cr.error = d.error;
    return { ok: true, crawl_result: cr };
  }

  // Broad feed crawl result.
  function buildBroadCrawlResult(d) {
    return {
      ok: true,
      crawl_result: {
        crawler_version: d.crawlerVersion,
        task_id: d.task?.task_id || '',
        intent: d.task?.intent || 'facebook_crawl',
        keywords: Array.isArray(d.task?.keywords) ? d.task.keywords : [],
        items: d.items,
        exit_reason: d.exitReason,
        cursor_reached: d.cursorReached,
        scroll_diag: buildScrollDiag(d.scrollDiag),
      },
    };
  }

  return Object.freeze({ buildScrollDiag, buildDirectPostResult, buildBroadCrawlResult });
})();
// CommonJS export for the node test harness. No-op in the extension.
if (typeof module !== 'undefined' && module.exports) module.exports = globalThis.THGCrawlResult;
