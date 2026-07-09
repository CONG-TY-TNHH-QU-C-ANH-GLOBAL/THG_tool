var THGContentCrawl = (() => {
  const CRAWLER_VERSION = 'scroll-target-v3-cursor';

  // Pure post-identity helpers (extractPostFBID / extractGroupFBID /
  // canonicalPostPermalink / looksLikePostURL / hashKey / stripPostQueryParams)
  // live in platforms/facebook/crawl_post_identity.js (THGCrawlIdentity).
  // Direct-post gate → THGCrawlDirectPost; result shaping → THGCrawlResult;
  // telemetry → THGCrawlProgress. crawl.js is the DOM bridge/orchestrator.
  const CP = () => globalThis.THGCrawlProgress;
  const ID = () => globalThis.THGCrawlIdentity;
  const DP = () => globalThis.THGCrawlDirectPost;
  const RES = () => globalThis.THGCrawlResult;
  const PACE = () => globalThis.THGCrawlPacing;

  // Multi-pass scan preferring URL forms whose post id is guaranteed to
  // be URL-resolvable. /permalink/ and story_fbid= carry the working
  // story_fbid; /posts/ may carry top_level_post_id (the dead-link bug).
  // Comment-count anchors (with comment_id=) often have the working
  // /permalink/ form embedded, so we DON'T exclude them — instead we
  // strip the trailing query params before returning.
  function postPermalink(article) {
    const anchors = Array.from(article.querySelectorAll('a[href]'));
    const isCandidate = (href) => {
      if (!href) return false;
      if (href.includes('/user/') || href.includes('/hashtag/')) return false;
      // Photo viewer URLs match Tier 3 (?fbid=) but their fbid is the
      // photo's, not the post's. Excluding them here lets the tier scan
      // fall through to /posts/ (Tier 4) or /permalink/ (Tier 1) for
      // the real canonical post URL. Catch all FB photo URL variants:
      // /photo/, /photo.php, and bare /photo?... (no trailing slash).
      if (/\/photo(\/|\.|\?|$)/.test(href)) return false;
      return true;
    };
    const tiers = [
      // Tier 1: /permalink/{id}/ — canonical form, always story_fbid.
      (h) => /\/permalink\/\d+/.test(h),
      // Tier 2: story_fbid= query — story_fbid spelled explicitly.
      (h) => /[?&]story_fbid=\d+/.test(h),
      // Tier 3: fbid= query — usually story_fbid on group surfaces.
      (h) => /[?&]fbid=\d+/.test(h),
      // Tier 4: /posts/{id}/ — last resort because FB sometimes renders
      // top_level_post_id here, producing dead links.
      (h) => /\/posts\/\d+/.test(h),
      // Tier 5: multi_permalinks legacy form.
      (h) => /multi_permalinks=\d+/.test(h),
    ];
    let match = null;
    for (const accept of tiers) {
      match = anchors.find(a => {
        const href = a.getAttribute('href') || '';
        return isCandidate(href) && accept(href);
      });
      if (match) break;
    }

    // Last resort: timestamp link (numeric text, no like/comment text).
    if (!match) {
      const timeLink = anchors.find(a => {
        const href = a.getAttribute('href') || '';
        if (!isCandidate(href)) return false;
        const txt = THGContentShared.textOf(a);
        return txt.length > 0 && txt.length < 15 && /\d/.test(txt) && !txt.includes('like') && !txt.includes('comment');
      });
      if (timeLink) return ID().stripPostQueryParams(THGContentShared.normalizeHref(timeLink.getAttribute('href')), location.origin);
    }

    return ID().stripPostQueryParams(THGContentShared.normalizeHref(match?.getAttribute('href') || location.href), location.origin);
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
  // missing a heartbeat must never block the crawl itself. This bridge supplies
  // the two impure inputs the pure builder needs: CRAWLER_VERSION and location.href.
  function emitProgress(task, accountId, stage, fetched, max, diag) {
    try {
      chrome.runtime.sendMessage(
        CP().buildCrawlProgressMessage({ crawlerVersion: CRAWLER_VERSION, task, accountId, stage, fetched, max, sourceUrl: location.href, diag })
      ).catch(() => { /* background not listening */ });
    } catch { /* runtime gone */ }
  }

  function dedupKey(article, expectedUrl, content, author) {
    const url = postPermalink(article);
    const isPagePermalink = !url || url === location.href || url === expectedUrl;
    if (!isPagePermalink) return url;
    return `c:${ID().hashKey((author?.author_profile_url || '') + '|' + content.slice(0, 240))}`;
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

  // ── P1.3E direct-post permalink target-aware extraction ──────────────────────
  // Broad crawl and direct-post permalink MUST NOT share candidate acceptance. For an
  // explicit direct-post import the extension must extract ONLY the requested post container,
  // prove its post id + group ref, reject sidebar/related/foreign-group/boilerplate, and
  // return a TYPED failure (in crawl_result.error) when the target is not rendered — never a
  // fake analyzable item. The backend directpost.Validate remains the authoritative second line.

  // The PURE direct-post gate (directPostMeaningful / directPostBoilerplate /
  // directPostGroupRef / directPostVerdict) lives in
  // platforms/facebook/crawl_direct_post.js (THGCrawlDirectPost). The impure scan
  // below stays here because it walks the DOM via postPermalink/authorFromArticle.

  // buildTargetCandidate extracts one article into the item shape (same helpers the broad loop
  // uses), scoped to a single container so foreign-group anchors elsewhere on the page cannot leak.
  function buildTargetCandidate(article, expectedUrl, groupFBID) {
    const content = THGContentShared.textOf(article);
    if (content.length < 20) return null;
    const author = authorFromArticle(article);
    const url = postPermalink(article);
    let postFBID = ID().extractPostFBID(url);
    if (!postFBID) {
      for (const a of article.querySelectorAll('a[href]')) {
        const id = ID().extractPostFBID(a.getAttribute('href') || '');
        if (id) { postFBID = id; break; }
      }
    }
    let sourceURL = '';
    if (url && ID().looksLikePostURL(url) && url !== location.href) sourceURL = url;
    else if (postFBID) sourceURL = ID().canonicalPostPermalink(groupFBID, postFBID);
    if (!sourceURL) sourceURL = expectedUrl || location.href;
    return {
      id: `dp:${postFBID || sourceURL}`, source_url: sourceURL,
      author_profile_url: author.author_profile_url, author_name: author.author_name,
      content, reactions: 0, comments: 0, shares: 0, post_fbid: postFBID, group_fbid: groupFBID, posted_at: ''
    };
  }

  // selectDirectPostTargetItem scans candidate articles for the ONE that is the requested post.
  // Returns {item} for a clean target, {reason} when the requested post id matched but the item
  // is poisoned, or {reason:''} when no candidate matched the target id yet (keep waiting).
  function selectDirectPostTargetItem(articles, target, expectedUrl, groupFBID) {
    let poison = '';
    for (const article of articles) {
      const item = buildTargetCandidate(article, expectedUrl, groupFBID);
      if (!item) continue;
      const v = DP().directPostVerdict(item, target);
      if (!v.match) continue;
      if (v.ok) return { item };
      if (!poison) poison = v.reason;
    }
    return { reason: poison };
  }

  // directPostResult builds the typed direct-post crawl_result via the shared
  // THGCrawlResult builder (landed_url + crawler version injected here).
  function directPostResult(task, items, exitReason, error, passes, maxArticles) {
    return RES().buildDirectPostResult({
      crawlerVersion: CRAWLER_VERSION, task, items, exitReason, error,
      passes, maxArticles, landedUrl: location.href || '',
    });
  }

  // crawlDirectPostTarget: bounded, target-only loop. Waits (with small nudges) for the target
  // post container to render, emits ONLY it, and returns a typed failure if it never appears.
  async function crawlDirectPostTarget(task, expectedUrl, accountId, target) {
    const groupFBID = ID().extractGroupFBID(expectedUrl || location.href);
    const MAX_ATTEMPTS = 12; // ~12s bounded; the target is one post, not a feed
    let maxArticles = 0;
    emitProgress(task, accountId, 'started', 0, 1);
    for (let attempt = 0; attempt < MAX_ATTEMPTS; attempt++) {
      if (attempt > 0) await new Promise(r => setTimeout(r, 1000));
      const articles = collectPostCandidates();
      if (articles.length > maxArticles) maxArticles = articles.length;
      const sel = selectDirectPostTargetItem(articles, target, expectedUrl, groupFBID);
      if (sel.item) {
        emitProgress(task, accountId, 'finished', 1, 1);
        return directPostResult(task, [sel.item], 'direct_post_target_found', '', attempt + 1, maxArticles);
      }
      if (sel.reason) { // requested post id matched but poisoned — fail typed, don't keep waiting
        emitProgress(task, accountId, 'finished', 0, 1);
        return directPostResult(task, [], sel.reason, sel.reason, attempt + 1, maxArticles);
      }
      try { window.scrollBy({ top: Math.floor(window.innerHeight * 0.6) }); } catch { /* best effort */ }
    }
    emitProgress(task, accountId, 'finished', 0, 1);
    return directPostResult(task, [], 'direct_post_target_not_rendered', 'direct_post_target_not_rendered', MAX_ATTEMPTS, maxArticles);
  }

  async function crawlVisibleFacebookPosts(task, expectedUrl, accountId) {
    // Safety-first: if Facebook parked this tab on a login wall / identity
    // checkpoint (redirect on navigate), stop gracefully with a typed reason
    // instead of scraping the verification page or grinding out the full pass
    // budget. Reuses the shared URL classifier — no bypass, no auto-resolve.
    // Runs before the wrong_page guard so a checkpoint reads as checkpoint.
    const entryRisk = CP().detectCrawlRisk(globalThis.THGNavReport, location.href);
    if (entryRisk) {
      emitProgress(task, accountId, 'finished', 0, 0, CP().zeroCrawlDiag(entryRisk));
      return { ok: false, error: CP().crawlRiskToReason(entryRisk) };
    }
    // Refuse to scrape if Facebook redirected us off the requested page (e.g.
    // newsfeed). Without this guard the extension silently scraped the wrong
    // page and shipped irrelevant posts to the classifier.
    if (expectedUrl && !locationMatchesExpected(expectedUrl)) {
      return {
        ok: false,
        error: `wrong_page: expected ${expectedUrl} but tab is on ${location.href}`
      };
    }
    // P1.3E: explicit direct-post permalink import → target-aware extraction (NOT the feed
    // scanner). Gated on a target identity in task.extras; broad crawl never sets this, so feed
    // behaviour is byte-identical below.
    const dpTarget = task && task.extras && task.extras.direct_post_target;
    if (dpTarget && (String(dpTarget.post_fbid || '').trim() || String(dpTarget.canonical_url || '').trim())) {
      return await crawlDirectPostTarget(task, expectedUrl, accountId, dpTarget);
    }
    const maxItems = Math.max(1, Math.min(200, Number(task?.crawl_plan?.max_items || 20)));
    // Recurring crawl cursor — when present, stop traversal as soon as we
    // re-encounter the post id from the previous run. The post id (not the
    // timestamp) is the dedup decision per the design mandate; FB feed
    // reorders/pins/async-injects too aggressively for timestamp-only logic.
    // See project_scheduled_intelligence.md cursor design mandate.
    const cursorPostID = String(task?.crawl_plan?.cursor_last_post_id || '').trim();
    const groupFBID = ID().extractGroupFBID(expectedUrl || location.href);
    // Facebook often loads only a few posts per scroll, especially in groups.
    // Give 50+ post crawls enough passes before deciding the feed is exhausted.
    const { maxPasses, minPassesBeforeStop } = PACE().crawlPacingBounds(maxItems);
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
    // Scroll forensic (PR-CRAWL1): records whether OUR scroll actually moved the
    // feed vs whether FB simply stopped loading. Decisive for "only 1 post,
    // exit=no_progress" — scroll_moved_ever=false ⇒ our scroll/throttle problem
    // (window minimized → rAF throttled, wrong scroll target); true but
    // max_articles=1 ⇒ FB did not load more despite scrolling (platform side).
    let passesRun = 0;
    let maxScrollY = 0;
    let maxArticles = 0;
    let maxDocHeight = 0;
    let scrollMovedEver = false;
    // PR-C1B live-telemetry counters (observability only — none of these change
    // scroll/wait/extraction pacing or the existing stop thresholds).
    let duplicateCount = 0;
    let foundCount = 0;
    let lastNewAtMs = Date.now();
    let riskSignal = ''; // set by the in-loop checkpoint/login/risk probe
    // Single place the heartbeat diagnostics are assembled, from the counters
    // above. Field-by-field so nothing but these typed values reaches the wire.
    const buildLoopDiag = (done) => {
      const c = CP().classifyCrawlProgress({
        risk: riskSignal, newCount: items.length, duplicateCount,
        scrollCount: passesRun, noProgressRounds: stagnantPasses,
        scrollMovedEver, done, reachedMax: items.length >= maxItems,
      });
      return {
        phase: c.phase, found_count: foundCount, new_count: items.length,
        duplicate_count: duplicateCount, scroll_count: passesRun,
        no_progress_rounds: stagnantPasses, scroll_moved_ever: scrollMovedEver,
        seconds_since_last_new: Math.round((Date.now() - lastNewAtMs) / 1000),
        safe_reason_code: c.safe_reason_code,
      };
    };
    emitProgress(task, accountId, 'started', 0, maxItems);
    for (let pass = 0; pass < maxPasses && items.length < maxItems; pass++) {
      // Small pause before grabbing elements, giving UI a moment to react to the previous scroll
      if (pass > 0) await new Promise(r => setTimeout(r, PACE().PACING.PRE_GRAB_PAUSE_MS));
      const itemsBeforePass = items.length;
      
      const articles = collectPostCandidates();
      // Safety probe: cheap per-pass URL check; the expensive body-text banner
      // check runs ONLY when a pass yielded zero posts (the checkpoint/block
      // interstitial signature), so the steady state adds no DOM scan. On a
      // detected wall, stop gracefully with a typed reason — never bypass.
      const urlRisk = CP().detectCrawlRisk(globalThis.THGNavReport, location.href);
      const risk = urlRisk || (articles.length === 0 ? CP().detectCrawlBanner(globalThis.THGContentProof) : '');
      if (risk) {
        riskSignal = risk;
        exitReason = CP().crawlRiskToReason(risk);
        break;
      }
      for (const article of articles) {
        const content = THGContentShared.textOf(article);
        if (content.length < 20) continue;
        foundCount++;
        const author = authorFromArticle(article);
        const key = dedupKey(article, expectedUrl, content, author);
        if (seen.has(key)) { duplicateCount++; continue; }
        seen.add(key);
        const url = postPermalink(article);
        // Try every anchor in the article for a post id — postPermalink may
        // have fallen back to a timestamp or the page URL, both of which
        // can lack /posts/N. The wider scan catches lazy-rendered anchors.
        let postFBID = ID().extractPostFBID(url);
        if (!postFBID) {
          for (const a of article.querySelectorAll('a[href]')) {
            const id = ID().extractPostFBID(a.getAttribute('href') || '');
            if (id) { postFBID = id; break; }
          }
        }
        // CURSOR HONOR — physical incremental optimization. If we hit the post
        // id from the previous run, we have caught up to the prior frontier:
        // everything below in the feed is already ingested. Stop traversal
        // entirely instead of scanning the rest of the feed.
        if (cursorPostID && postFBID && postFBID === cursorPostID) {
          cursorReached = true;
          exitReason = 'cursor_match';
          break;
        }
        // Source URL resolution priority:
        //   1. The anchor we scraped, IF it actually identifies a post.
        //   2. Synthesised canonical permalink from postFBID + groupFBID.
        //   3. The crawler's expected URL (the group/page shell) — last resort,
        //      will be rejected by the server-side validator unless rescued.
        let sourceURL = '';
        if (url && ID().looksLikePostURL(url) && url !== location.href) {
          sourceURL = url;
        } else if (postFBID) {
          sourceURL = ID().canonicalPostPermalink(groupFBID, postFBID);
        }
        if (!sourceURL) {
          sourceURL = expectedUrl || location.href;
        }
        items.push({
          id: key,
          source_url: sourceURL,
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
      if (newItemsThisPass) { lastNewItemPass = pass; lastNewAtMs = Date.now(); }
      console.log('[THG crawl]', {
        pass,
        articles_seen: articlesSeen,
        items_collected: items.length,
        scroll_y: scrollY,
        doc_height: docHeight,
        scroll_target: scrollInfo.label
      });
      if (items.length >= maxItems) {
        exitReason = 'maxItems';
        break;
      }
      // Facebook virtualizes the feed, so scrollHeight/article count can stay
      // flat while the viewport still moves. Count scroll movement as progress,
      // then stop only after enough active scrolling fails to produce new posts.
      const scrollMoved = prevScrollY >= 0 && Math.abs(scrollY - prevScrollY) > 24;
      // Scroll forensic accumulation.
      if (scrollMoved) scrollMovedEver = true;
      passesRun = pass + 1;
      if (scrollY > maxScrollY) maxScrollY = scrollY;
      if (articlesSeen > maxArticles) maxArticles = articlesSeen;
      if (docHeight > maxDocHeight) maxDocHeight = docHeight;
      const targetChanged = prevScrollTarget && prevScrollTarget !== scrollInfo.label;
      const pageMoved = docHeight !== prevHeight || articlesSeen !== prevArticles || items.length !== prevItemsLength || scrollMoved || targetChanged;
      if (pass > 0 && !pageMoved) stagnantPasses++;
      else stagnantPasses = 0;
      const stopReason = PACE().crawlStopReason({
        stagnantPasses, pass, minPassesBeforeStop,
        itemsLength: items.length, lastNewItemPass,
        scrollMovedEver, duplicateCount,
      });
      if (stopReason) { exitReason = stopReason; break; }
      // One heartbeat per continuing pass, emitted AFTER this pass's diagnostics
      // (passesRun, scrollMovedEver, stagnantPasses) are refreshed so telemetry
      // reflects the just-completed pass, not the previous one. Passes that broke
      // early above are covered by the single 'finished' emit below. Backend
      // rate-limits these so the cadence won't spam Telegram.
      emitProgress(task, accountId, 'scraping', items.length, maxItems, buildLoopDiag(false));
      prevHeight = docHeight;
      prevArticles = articlesSeen;
      prevItemsLength = items.length;
      prevScrollY = scrollY;
      prevScrollTarget = scrollInfo.label;
      
      // Facebook's infinite scroll is more reliable with steady viewport-sized
      // movement and an occasional larger push to wake lazy loading.
      scrollByTarget(scrollTarget, PACE().crawlScrollDeltaY({ pass, innerHeight: window.innerHeight }), articles, pass);
      // Wait for FB lazy-load. Productive safe passes wait less (PR-C2 commit 2);
      // barren/uncertain passes keep the cautious tiered wait. riskSignal is ''
      // here (a risk break happens earlier in the pass, before pacing).
      const waitMs = PACE().nextCrawlWaitMs({ pass, producedNewItems: newItemsThisPass, risk: riskSignal });
      await new Promise(resolve => setTimeout(resolve, waitMs));
    }
    console.log('[THG crawl] exit', { reason: exitReason, items: items.length, max: maxItems, cursor_reached: cursorReached });
    emitProgress(task, accountId, 'finished', items.length, maxItems, buildLoopDiag(true));
    return RES().buildBroadCrawlResult({
      crawlerVersion: CRAWLER_VERSION, task, items, exitReason, cursorReached,
      scrollDiag: {
        passes: passesRun,
        maxArticlesSeen: maxArticles,
        maxScrollY,
        maxDocHeight,
        scrollMovedEver,
        finalScrollTarget: prevScrollTarget,
        landedUrl: location.href || '',
      },
    });
  }

  return { crawlVisibleFacebookPosts, selectDirectPostTargetItem };
})();
globalThis.THGContentCrawl = THGContentCrawl;
// CommonJS export for the node test harness (content/*.test.mjs). No-op in the extension.
if (typeof module !== 'undefined' && module.exports) module.exports = THGContentCrawl;
