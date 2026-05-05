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

  async function crawlVisibleFacebookPosts(task) {
    const maxItems = Math.max(1, Math.min(50, Number(task?.crawl_plan?.max_items || 20)));
    const seen = new Set();
    const items = [];
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
      if (items.length >= maxItems) break;
      window.scrollBy({ top: Math.max(700, window.innerHeight * 0.85), behavior: 'smooth' });
      await new Promise(resolve => setTimeout(resolve, 1400));
    }
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
