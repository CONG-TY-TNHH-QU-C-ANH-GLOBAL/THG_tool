// THG Facebook crawl — direct-post permalink gate. Pure per-item validation
// extracted from content/crawl.js (PR-C1C). Mirrors the backend
// directpost.Validate invariants so a poisoned candidate never leaves the
// browser. No DOM access — the impure article scan (selectDirectPostTargetItem)
// stays in crawl.js. Depends on THGCrawlIdentity.extractPostFBID at call time.
globalThis.THGCrawlDirectPost = globalThis.THGCrawlDirectPost || (() => {
  // UI-chrome tokens dropped before measuring "real" post text — mirror of the
  // Go directpost.MeaningfulText so "Facebook Facebook…" / "Like Comment Share"
  // reduce to nothing.
  const DP_CHROME_TOKENS = new Set(['facebook', 'like', 'comment', 'comments', 'share', 'shares',
    'follow', 'following', 'reply', 'replies', 'reactions', 'react']);

  // Leading edge-punctuation uses a start-anchored pattern (linear, no
  // backtracking). Trailing punctuation is stripped WITHOUT a regex: an
  // end-anchored `/[…]+$/` has super-linear backtracking, so we scan the same
  // character set from the end instead. Same net result as the prior
  // `.replace(/^…+/,'').replace(/…+$/,'')`.
  const DP_EDGE_PUNCT_LEAD = /^[·.,:;!?()[\]{}"'…]+/;
  const DP_EDGE_PUNCT = new Set(['·', '.', ',', ':', ';', '!', '?', '(', ')', '[', ']', '{', '}', '"', "'", '…']);
  const DP_GROUP_REF_RE = /\/groups\/([^/?#]+)/;

  // Remove the maximal run of DP_EDGE_PUNCT characters from the END of value.
  function stripTrailingPunct(value) {
    let end = value.length;
    while (end > 0 && DP_EDGE_PUNCT.has(value[end - 1])) end--;
    return value.slice(0, end);
  }

  function directPostMeaningful(content) {
    const out = [];
    let prev = '';
    for (const f of String(content || '').split(/\s+/)) {
      const norm = stripTrailingPunct(f.toLowerCase().replace(DP_EDGE_PUNCT_LEAD, ''));
      if (!norm || DP_CHROME_TOKENS.has(norm)) continue;
      if (norm === prev) continue; // collapse the scraped-chrome repetition signature
      out.push(f);
      prev = norm;
    }
    return out.join(' ');
  }

  // True when content has < 12 meaningful code points after chrome stripping (boilerplate).
  function directPostBoilerplate(content) {
    return Array.from(directPostMeaningful(content).trim()).length < 12;
  }

  function directPostGroupRef(url) {
    const m = DP_GROUP_REF_RE.exec(String(url || ''));
    return m ? m[1] : '';
  }

  // directPostVerdict is the PURE per-item gate. match=false means "not the
  // requested post id" (keep scanning); match=true+ok=false means "the requested
  // post came back poisoned" with a typed reason; match=true+ok=true means emit it.
  function directPostVerdict(item, target) {
    const tPost = String(target?.post_fbid || '').trim();
    const tGroup = String(target?.group_ref || '').trim();
    const obsPost = String(item?.post_fbid || '').trim() || globalThis.THGCrawlIdentity.extractPostFBID(item?.source_url || '');
    if (tPost) {
      if (obsPost !== tPost) return { match: false };
    } else if (!obsPost) {
      return { match: false };
    }
    if (tGroup) {
      const ag = directPostGroupRef(item?.author_profile_url || '');
      if (ag && ag !== tGroup) return { match: true, ok: false, reason: 'direct_post_group_mismatch' };
      const sg = directPostGroupRef(item?.source_url || '');
      if (sg && /^\D/.test(sg) && sg !== tGroup) return { match: true, ok: false, reason: 'direct_post_group_mismatch' };
    }
    if (directPostBoilerplate(item?.content || '')) {
      return { match: true, ok: false, reason: 'direct_post_boilerplate_content' };
    }
    return { match: true, ok: true };
  }

  return Object.freeze({
    directPostMeaningful, directPostBoilerplate, directPostGroupRef, directPostVerdict,
  });
})();
// CommonJS export for the node test harness. No-op in the extension.
if (typeof module !== 'undefined' && module.exports) module.exports = globalThis.THGCrawlDirectPost;
