var THGContentCrawl = globalThis.THGContentCrawl || (() => {
  function postPermalink(article) {
    const anchors = Array.from(article.querySelectorAll('a[href]'));
    const match = anchors.find(a => {
      const href = a.getAttribute('href') || '';
      return /\/posts\/|\/permalink\/|story_fbid=|multi_permalinks=|\/groups\/[^/]+\/permalink\//i.test(href);
    });
    return THGContentShared.normalizeHref(match?.getAttribute('href') || location.href);
  }

  function authorFromArticle(article) {
    const anchors = Array.from(article.querySelectorAll('a[href]'));
    for (const a of anchors) {
      const href = THGContentShared.normalizeHref(a.getAttribute('href'));
      if (!href || !THGContentShared.FACEBOOK_URL_RE.test(href)) continue;
      const name = THGContentShared.textOf(a);
      if (name.length < 2 || name.length > 80) continue;
      if (/^(like|comment|share|see more|follow)$/i.test(name)) continue;
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
    const maxItems = Math.max(1, Math.min(50, Number(task?.crawl_plan?.max_items || 20)));
    const seen = new Set();
    const items = [];
    emitProgress(task, accountId, 'started', 0, maxItems);
    for (let pass = 0; pass < 8 && items.length < maxItems; pass++) {
      const articles = Array.from(document.querySelectorAll('[role="article"], div[data-pagelet^="FeedUnit_"]'));
      for (const article of articles) {
        const content = THGContentShared.textOf(article);
        if (content.length < 20) continue;
        const url = postPermalink(article);
        const key = `${url}|${content.slice(0, 120)}`;
        if (seen.has(key)) continue;
        seen.add(key);
        const author = authorFromArticle(article);
        items.push({
          id: key,
          source_url: url,
          author_profile_url: author.author_profile_url,
          author_name: author.author_name,
          content,
          reactions: 0,
          comments: 0,
          shares: 0
        });
        if (items.length >= maxItems) break;
      }
      // After each scroll pass send a heartbeat. Backend rate-limits these so
      // even an aggressive cadence here won't spam Telegram.
      emitProgress(task, accountId, 'scraping', items.length, maxItems);
      if (items.length >= maxItems) break;
      window.scrollBy({ top: Math.max(700, window.innerHeight * 0.85), behavior: 'smooth' });
      await new Promise(resolve => setTimeout(resolve, 1400));
    }
    emitProgress(task, accountId, 'finished', items.length, maxItems);
    return {
      ok: true,
      crawl_result: {
        task_id: task?.task_id || '',
        intent: task?.intent || 'facebook_crawl',
        keywords: Array.isArray(task?.keywords) ? task.keywords : [],
        items
      }
    };
  }

  return { crawlVisibleFacebookPosts };
})();
globalThis.THGContentCrawl = THGContentCrawl;
