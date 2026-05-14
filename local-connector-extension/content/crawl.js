var THGContentCrawl = (() => {
  const CRAWLER_VERSION = 'scroll-target-v3-cursor';

  // Extracts the Facebook-side post id from a permalink. Mirror of the Go
  // helper ExtractFacebookPostID; the JS side runs first and emits the id so
  // the server does not need URL fallback parsing. Empty when no canonical
  // id pattern matches.
  function extractPostFBID(url) {
    if (!url) return '';
    let m = url.match(/\/posts\/(\d+)/);
    if (m) return m[1];
    m = url.match(/\/permalink\/(\d+)/);
    if (m) return m[1];
    m = url.match(/story_fbid=(\d+)/);
    if (m) return m[1];
    m = url.match(/[?&]fbid=(\d+)/);
    if (m) return m[1];
    return '';
  }

  function extractGroupFBID(url) {
    if (!url) return '';
    const m = url.match(/\/groups\/(\d+)/);
    return m ? m[1] : '';
  }

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
  // follow the crawl in real time. Failures are intentionally swallowed;
  // missing a heartbeat must never block the crawl itself.
  function emitProgress(task, accountId, stage, fetched, max) {
    try {
      chrome.runtime.sendMessage({
        type: 'thg_crawl_progress',
        crawler_version: CRAWLER_VERSION,
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
  // captured the first 1-3 unique items. djb2 is fine here: collision-resilient
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

  function climbPostContainer(node) {
    let best = null;
    for (let el = node; el && el !== document.body; el = el.parentElement) {
      if (!(el instanceof Element)) continue;
      if (el.getAttribute('role') === 'article') return el;
      const rect = el.getBoundingClientRect();
      const text = THGContentShared.textOf(el);
      if (text.length >= 20 && rect.height >= 80 && rect.height <= window.innerHeight * 2.8 && rect.width >= 260) {
        best = el;
      }
    }
    return best;
  }

  function collectPostCandidates() {
    const out = new Set();
    const push = node => {
      if (!(node instanceof Element)) return;
      const rect = node.getBoundingClientRect();
      if (rect.width < 240 || rect.height < 60) return;
      if (rect.bottom < -window.innerHeight || rect.top > window.innerHeight * 2.5) return;
      if (node.closest('[role="navigation"], [role="banner"], [role="complementary"]')) return;
      if (THGContentShared.textOf(node).length < 20) return;
      out.add(node);
    };
    document.querySelectorAll('[role="article"], div[data-pagelet^="FeedUnit_"], div[aria-posinset]').forEach(push);
    document.querySelectorAll('div[data-ad-preview="message"], div[data-ad-comet-preview="message"]').forEach(node => {
      push(node.closest('[role="article"], div[data-pagelet^="FeedUnit_"], div[aria-posinset]') || climbPostContainer(node));
    });
    return Array.from(out);
  }

  function documentScroller() {
    return document.scrollingElement || document.documentElement || document.body;
  }

  function isDocumentScroller(el) {
    const root = documentScroller();
    return !el || el === root || el === document.documentElement || el === document.body;
  }

  function scrollTargetLabel(el) {
    if (isDocumentScroller(el)) return 'document';
    const role = el.getAttribute('role') || '';
    const pagelet = el.getAttribute('data-pagelet') || '';
    return `${el.tagName.toLowerCase()}${role ? `[role=${role}]` : ''}${pagelet ? `[pagelet=${pagelet}]` : ''}`;
  }

  function scrollMetrics(el) {
    const root = documentScroller();
    if (isDocumentScroller(el)) {
      return {
        label: 'document',
        top: Math.round(window.scrollY || root.scrollTop || document.body.scrollTop || 0),
        clientHeight: window.innerHeight,
        scrollHeight: Math.max(root.scrollHeight || 0, document.body.scrollHeight || 0, document.documentElement.scrollHeight || 0)
      };
    }
    return {
      label: scrollTargetLabel(el),
      top: Math.round(el.scrollTop || 0),
      clientHeight: el.clientHeight || 0,
      scrollHeight: el.scrollHeight || 0
    };
  }

  function findScrollTarget() {
    const root = documentScroller();
    const rootDelta = Math.max(root.scrollHeight || 0, document.body.scrollHeight || 0) - window.innerHeight;
    let best = { el: root, score: Math.max(0, rootDelta) };
    const nodes = Array.from(document.querySelectorAll('main, div, section'));
    for (const el of nodes) {
      const delta = (el.scrollHeight || 0) - (el.clientHeight || 0);
      if (delta < 180) continue;
      const rect = el.getBoundingClientRect();
      if (rect.height < 280 || rect.width < 360) continue;
      if (rect.bottom < 80 || rect.top > window.innerHeight - 80) continue;
      const style = window.getComputedStyle(el);
      if (!/(auto|scroll|overlay)/i.test(style.overflowY || '') && el.getAttribute('role') !== 'main' && el.getAttribute('role') !== 'feed') continue;
      const role = el.getAttribute('role') || '';
      const score = delta + rect.height * 2 + (role === 'feed' ? 1200 : 0) + (role === 'main' ? 800 : 0);
      if (score > best.score) best = { el, score };
    }
    return best.el;
  }

  function dispatchWheel(deltaY) {
    try {
      const x = Math.floor(window.innerWidth * 0.55);
      const y = Math.floor(window.innerHeight * 0.75);
      const el = document.elementFromPoint(x, y) || document.body;
      el.dispatchEvent(new WheelEvent('wheel', {
        bubbles: true,
        cancelable: true,
        deltaY,
        deltaMode: 0,
        clientX: x,
        clientY: y
      }));
    } catch { /* best effort */ }
  }

  function scrollByTarget(target, deltaY, articles, pass) {
    const last = articles[articles.length - 1];
    if (last && pass % 4 === 3) {
      try { last.scrollIntoView({ block: 'end', behavior: 'smooth' }); } catch { /* ignore */ }
    }
    dispatchWheel(deltaY);
    if (isDocumentScroller(target)) {
      window.scrollBy({ top: deltaY, behavior: 'smooth' });
      const root = documentScroller();
      root.scrollTop = Math.min(root.scrollHeight, (root.scrollTop || window.scrollY || 0) + deltaY);
      document.dispatchEvent(new Event('scroll', { bubbles: true }));
      window.dispatchEvent(new Event('scroll'));
      return;
    }
    target.scrollBy({ top: deltaY, behavior: 'smooth' });
    target.scrollTop = Math.min(target.scrollHeight, (target.scrollTop || 0) + deltaY);
    target.dispatchEvent(new Event('scroll', { bubbles: true }));
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
    // Recurring crawl cursor — when present, stop traversal as soon as we
    // re-encounter the post id from the previous run. The post id (not the
    // timestamp) is the dedup decision per the design mandate; FB feed
    // reorders/pins/async-injects too aggressively for timestamp-only logic.
    // See project_scheduled_intelligence.md cursor design mandate.
    const cursorPostID = String(task?.crawl_plan?.cursor_last_post_id || '').trim();
    const groupFBID = extractGroupFBID(expectedUrl || location.href);
    // Facebook often loads only a few posts per scroll, especially in groups.
    // Give 50+ post crawls enough passes before deciding the feed is exhausted.
    const maxPasses = Math.max(70, Math.min(260, Math.ceil(maxItems * 3)));
    const minPassesBeforeStop = Math.min(maxPasses - 1, Math.max(18, Math.ceil(maxItems * 0.7)));
    const seen = new Set();
    const items = [];
    let stagnantPasses = 0;
    let lastNewItemPass = 0;
    let prevHeight = 0;
    let prevArticles = 0;
    let prevItemsLength = 0;
    let prevScrollY = -1;
    let prevScrollTarget = '';
    let exitReason = 'pass_exhausted';
    let cursorReached = false;
    emitProgress(task, accountId, 'started', 0, maxItems);
    for (let pass = 0; pass < maxPasses && items.length < maxItems; pass++) {
      // Small pause before grabbing elements, giving UI a moment to react to the previous scroll
      if (pass > 0) await new Promise(r => setTimeout(r, 300));
      const itemsBeforePass = items.length;
      
      const articles = collectPostCandidates();
      for (const article of articles) {
        const content = THGContentShared.textOf(article);
        if (content.length < 20) continue;
        const author = authorFromArticle(article);
        const key = dedupKey(article, expectedUrl, content, author);
        if (seen.has(key)) continue;
        seen.add(key);
        const url = postPermalink(article);
        const postFBID = extractPostFBID(url);
        // CURSOR HONOR — physical incremental optimization. If we hit the post
        // id from the previous run, we have caught up to the prior frontier:
        // everything below in the feed is already ingested. Stop traversal
        // entirely instead of scanning the rest of the feed.
        if (cursorPostID && postFBID && postFBID === cursorPostID) {
          cursorReached = true;
          exitReason = 'cursor_match';
          break;
        }
        items.push({
          id: key,
          source_url: url && url !== location.href ? url : (expectedUrl || location.href),
          author_profile_url: author.author_profile_url,
          author_name: author.author_name,
          content,
          reactions: 0,
          comments: 0,
          shares: 0,
          post_fbid: postFBID,
          group_fbid: groupFBID,
          posted_at: ''
        });
        if (items.length >= maxItems) break;
      }
      if (cursorReached) break;
      const scrollTarget = findScrollTarget();
      const scrollInfo = scrollMetrics(scrollTarget);
      const docHeight = scrollInfo.scrollHeight;
      const articlesSeen = articles.length;
      const scrollY = scrollInfo.top;
      const newItemsThisPass = items.length > itemsBeforePass;
      if (newItemsThisPass) lastNewItemPass = pass;
      console.log('[THG crawl]', {
        pass,
        articles_seen: articlesSeen,
        items_collected: items.length,
        scroll_y: scrollY,
        doc_height: docHeight,
        scroll_target: scrollInfo.label
      });
      // After each scroll pass send a heartbeat. Backend rate-limits these so
      // even an aggressive cadence here won't spam Telegram.
      emitProgress(task, accountId, 'scraping', items.length, maxItems);
      if (items.length >= maxItems) {
        exitReason = 'maxItems';
        break;
      }
      // Facebook virtualizes the feed, so scrollHeight/article count can stay
      // flat while the viewport still moves. Count scroll movement as progress,
      // then stop only after enough active scrolling fails to produce new posts.
      const scrollMoved = prevScrollY >= 0 && Math.abs(scrollY - prevScrollY) > 24;
      const targetChanged = prevScrollTarget && prevScrollTarget !== scrollInfo.label;
      const pageMoved = docHeight !== prevHeight || articlesSeen !== prevArticles || items.length !== prevItemsLength || scrollMoved || targetChanged;
      if (pass > 0 && !pageMoved) {
        stagnantPasses++;
        if (stagnantPasses >= 10 && pass >= minPassesBeforeStop) {
          exitReason = 'no_progress';
          break;
        }
      } else {
        stagnantPasses = 0;
      }
      if (pass >= minPassesBeforeStop && items.length > 0 && pass - lastNewItemPass >= 16) {
        exitReason = 'no_new_items_after_scroll';
        break;
      }
      prevHeight = docHeight;
      prevArticles = articlesSeen;
      prevItemsLength = items.length;
      prevScrollY = scrollY;
      prevScrollTarget = scrollInfo.label;
      
      // Facebook's infinite scroll is more reliable with steady viewport-sized
      // movement and an occasional larger push to wake lazy loading.
      const viewportStep = Math.max(Math.floor(window.innerHeight * 0.95), 700);
      scrollByTarget(scrollTarget, pass % 6 === 5 ? viewportStep * 2 : viewportStep, articles, pass);
      // Lazy-load gets slower deeper into a feed
      const waitMs = pass < 8 ? 2200 : 3600;
      await new Promise(resolve => setTimeout(resolve, waitMs));
    }
    console.log('[THG crawl] exit', { reason: exitReason, items: items.length, max: maxItems, cursor_reached: cursorReached });
    emitProgress(task, accountId, 'finished', items.length, maxItems);
    return {
      ok: true,
      crawl_result: {
        crawler_version: CRAWLER_VERSION,
        task_id: task?.task_id || '',
        intent: task?.intent || 'facebook_crawl',
        keywords: Array.isArray(task?.keywords) ? task.keywords : [],
        items,
        exit_reason: exitReason,
        cursor_reached: cursorReached
      }
    };
  }

  return { crawlVisibleFacebookPosts };
})();
globalThis.THGContentCrawl = THGContentCrawl;
