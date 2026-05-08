var THGContentCrawl = globalThis.THGContentCrawl || (() => {
  function postPermalink(article) {
    const anchors = Array.from(article.querySelectorAll('a[href]'));
    const match = anchors.find(a => {
      const href = a.getAttribute('href') || '';
      // Exclude profile links and common action links
      if (href.includes('/user/') || href.includes('/hashtag/') || href.includes('comment_id=')) return false;
      return /\/posts\/|\/permalink\/|story_fbid=|multi_permalinks=|\/groups\/[^/]+\/permalink\//i.test(href) || 
             (href.includes('__cft__') && !href.includes('/groups/') && !href.includes('facebook.com/groups/'));
    });
    
    // If we didn't find a standard post URL, try to find any link that looks like a timestamp
    if (!match) {
        const timeLink = anchors.find(a => {
            const href = a.getAttribute('href') || '';
            if (href.includes('/user/') || href.includes('/hashtag/')) return false;
            // Often timestamps have a hover tooltip or contain numbers/relative time
            const txt = THGContentShared.textOf(a);
            return txt.length > 0 && txt.length < 15 && /\d/.test(txt) && !txt.includes('like') && !txt.includes('comment');
        });
        if (timeLink) return THGContentShared.normalizeHref(timeLink.getAttribute('href'));
    }
    
    return THGContentShared.normalizeHref(match?.getAttribute('href') || location.href);
  }

  function authorFromArticle(article) {
    // 1. Prioritize structural headers (h2, h3, h4, strong) which usually contain the real author name
    // This correctly handles "Anonymous participant" which has no anchor link.
    const headers = Array.from(article.querySelectorAll('h2, h3, h4, strong, b'));
    for (const el of headers) {
      const name = THGContentShared.textOf(el);
      if (name.length < 2 || name.length > 80) continue;
      if (/^(like|comment|share|see more|follow)$/i.test(name)) continue;
      
      // Skip obfuscated Sponsored garbage (usually no spaces, contains numbers/weird caps, or exactly "Sponsored")
      if (name.length > 10 && !name.includes(' ') && /\d/.test(name)) continue;
      
      // Find the closest anchor if any, to get the profile URL
      const a = el.closest('a') || el.querySelector('a');
      const href = a ? THGContentShared.normalizeHref(a.getAttribute('href')) : '';
      
      return { author_name: name, author_profile_url: href };
    }

    // 2. Fallback to anchors
    const anchors = Array.from(article.querySelectorAll('a[href]'));
    for (const a of anchors) {
      const href = THGContentShared.normalizeHref(a.getAttribute('href'));
      if (!href || !THGContentShared.FACEBOOK_URL_RE.test(href)) continue;
      
      // Skip links that look like post permalinks, we want profile links
      if (/\/posts\/|\/permalink\/|story_fbid=/.test(href)) continue;

      const name = THGContentShared.textOf(a);
      if (name.length < 2 || name.length > 80) continue;
      if (/^(like|comment|share|see more|follow)$/i.test(name)) continue;
      
      // Skip obfuscated Sponsored garbage
      if (name.length > 10 && !name.includes(' ') && /\d/.test(name)) continue;

      return { author_name: name, author_profile_url: href };
    }
    return { author_name: '', author_profile_url: '' };
  }

  function expectedPathOf(url) {
    try {
      return new URL(url).pathname.replace(/\/+$/, '');
    } catch {
      return '';
    }
  }

  function locationMatchesExpected(expectedUrl) {
    const want = expectedPathOf(expectedUrl);
    if (!want) return false;
    const got = expectedPathOf(location.href);
    if (!got) return false;
    return got === want || got.startsWith(want + '/');
  }

  // Best-effort heartbeat to the background page so users on Telegram can
  // follow the crawl in real time. Failures are intentionally swallowed —
  // missing a heartbeat must never block the crawl itself.
  function emitProgress(task, accountId, stage, fetched, max) {
    try {
      chrome.runtime.sendMessage({
        type: 'thg_crawl_progress',
        task_id: task?.task_id || '',
        intent: task?.intent || 'facebook_crawl',
        account_id: accountId || 0,
        stage,
        fetched,
        max,
        source_url: location.href
      }).catch(() => { /* background not listening */ });
    } catch { /* runtime gone */ }
  }

  // Stable hash for content+author when Facebook hasn't rendered the permalink
  // yet. Using location.href as a fallback dedup key (the prior bug) caused
  // every post on the same group page to share one key, so the loop only ever
  // captured the first 1–3 unique items. djb2 is fine here — collision-resilient
  // enough for one crawl session.
  function hashKey(s) {
    let h = 5381;
    for (let i = 0; i < s.length; i++) h = ((h << 5) + h + s.charCodeAt(i)) >>> 0;
    return h.toString(16);
  }

  function dedupKey(article, expectedUrl, content, author) {
    const url = postPermalink(article);
    const isPagePermalink = !url || url === location.href || url === expectedUrl;
    if (!isPagePermalink) return url;
    return `c:${hashKey((author?.author_profile_url || '') + '|' + content.slice(0, 240))}`;
  }

  async function crawlVisibleFacebookPosts(task, expectedUrl, accountId) {
    // Refuse to scrape if Facebook redirected us off the requested page (e.g.
    // newsfeed). Without this guard the extension silently scraped the wrong
    // page and shipped irrelevant posts to the classifier.
    if (expectedUrl && !locationMatchesExpected(expectedUrl)) {
      return {
        ok: false,
        error: `wrong_page: expected ${expectedUrl} but tab is on ${location.href}`
      };
    }
    const maxItems = Math.max(1, Math.min(200, Number(task?.crawl_plan?.max_items || 20)));
    // Increase maxPasses dynamically based on maxItems (Facebook often only loads 2-4 posts per scroll)
    const maxPasses = Math.max(40, Math.ceil(maxItems * 1.5));
    const seen = new Set();
    const items = [];
    let stagnantPasses = 0;
    let prevHeight = 0;
    let prevArticles = 0;
    let exitReason = 'pass_exhausted';
    emitProgress(task, accountId, 'started', 0, maxItems);
    for (let pass = 0; pass < maxPasses && items.length < maxItems; pass++) {
      const articles = Array.from(document.querySelectorAll(
        '[role="article"], div[data-pagelet^="FeedUnit_"], div[role="feed"] > div'
      ));
      for (const article of articles) {
        const content = THGContentShared.textOf(article);
        if (content.length < 20) continue;
        const author = authorFromArticle(article);
        const key = dedupKey(article, expectedUrl, content, author);
        if (seen.has(key)) continue;
        seen.add(key);
        const url = postPermalink(article);
        items.push({
          id: key,
          source_url: url && url !== location.href ? url : (expectedUrl || location.href),
          author_profile_url: author.author_profile_url,
          author_name: author.author_name,
          content,
          reactions: 0,
          comments: 0,
          shares: 0
        });
        if (items.length >= maxItems) break;
      }
      const docHeight = document.body.scrollHeight;
      const articlesSeen = articles.length;
      console.log('[THG crawl]', {
        pass,
        articles_seen: articlesSeen,
        items_collected: items.length,
        scroll_y: Math.round(window.scrollY),
        doc_height: docHeight
      });
      // After each scroll pass send a heartbeat. Backend rate-limits these so
      // even an aggressive cadence here won't spam Telegram.
      emitProgress(task, accountId, 'scraping', items.length, maxItems);
      if (items.length >= maxItems) {
        exitReason = 'maxItems';
        break;
      }
      // No-progress detection: if scrollHeight and visible article count both
      // stay flat across several consecutive passes, the feed is exhausted (or
      // Facebook stopped lazy-loading) — stop instead of burning the remaining passes.
      if (pass > 0 && docHeight === prevHeight && articlesSeen === prevArticles) {
        stagnantPasses++;
        if (stagnantPasses >= 4) { // Increased to 4 to tolerate slow loading
          exitReason = 'no_progress';
          break;
        }
      } else {
        stagnantPasses = 0;
      }
      prevHeight = docHeight;
      prevArticles = articlesSeen;
      window.scrollBy({ top: Math.max(900, window.innerHeight * 0.9), behavior: 'smooth' });
      // Lazy-load gets slower deeper into a feed; ramp the wait so the first
      // few passes stay snappy and later passes give Facebook time to fetch.
      const waitMs = pass < 6 ? 1400 : 2500;
      await new Promise(resolve => setTimeout(resolve, waitMs));
    }
    console.log('[THG crawl] exit', { reason: exitReason, items: items.length, max: maxItems });
    emitProgress(task, accountId, 'finished', items.length, maxItems);
    return {
      ok: true,
      crawl_result: {
        task_id: task?.task_id || '',
        intent: task?.intent || 'facebook_crawl',
        keywords: Array.isArray(task?.keywords) ? task.keywords : [],
        items,
        exit_reason: exitReason
      }
    };
  }

  return { crawlVisibleFacebookPosts };
})();
globalThis.THGContentCrawl = THGContentCrawl;
