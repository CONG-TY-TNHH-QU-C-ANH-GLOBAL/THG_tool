var THGContentCrawl = (() => {
  const CRAWLER_VERSION = 'scroll-target-v3-cursor';

  // Extracts the Facebook-side post id from a permalink. Mirror of the Go
  // helper ExtractFacebookPostID; the JS side runs first and emits the id so
  // the server does not need URL fallback parsing. Empty when no canonical
  // id pattern matches.
  //
  // /permalink/ FIRST because that form always carries the URL-resolvable
  // story_fbid. /posts/ LAST because Facebook sometimes renders the
  // FB-internal top_level_post_id there which doesn't resolve as a URL
  // (the "content isn't available" production bug). When both forms exist
  // on the same article, this ordering picks the working one.
  function extractPostFBID(url) {
    if (!url) return '';
    // Photo viewer URLs (`/photo/?fbid=X` modern, `/photo.php?fbid=X` legacy)
    // carry the PHOTO's fbid, NOT the parent post's. Matching `?fbid=` on
    // these URLs poisons lead.post_fbid with a photo id that does not
    // resolve back to any post — repair pipelines synthesize invalid
    // canonical permalinks and the server rejects them at comment time.
    // Skip the `?fbid=` extraction step when the path is photo-shaped.
    const isPhotoURL = /\/photo(\/|\.|\?|$)/.test(url);
    let m = url.match(/\/permalink\/(\d+)/);
    if (m) return m[1];
    m = url.match(/story_fbid=(\d+)/);
    if (m) return m[1];
    // `set=gm.X` — Facebook's group-media link parameter. The `gm.` prefix
    // marks the value as the PARENT POST fbid attached to a photo viewer
    // URL. This is how photo-only article anchors still surface a real
    // post id — without this clause, photo URLs leave post_fbid empty and
    // the inbound source_url falls back to the group shell.
    m = url.match(/[?&]set=gm\.(\d+)/);
    if (m) return m[1];
    if (!isPhotoURL) {
      m = url.match(/[?&]fbid=(\d+)/);
      if (m) return m[1];
    }
    m = url.match(/\/posts\/(\d+)/);
    if (m) return m[1];
    return '';
  }

  function extractGroupFBID(url) {
    if (!url) return '';
    // Path form `/groups/{id}/...` — canonical when navigation is on a
    // group surface.
    let m = url.match(/\/groups\/(\d+)/);
    if (m) return m[1];
    // `idorvanity={id}` — Facebook's group id query param on photo viewer
    // URLs (paired with `set=gm.X` for the post fbid). Lets us reconstruct
    // the canonical permalink even when the crawler only saw a photo anchor.
    m = url.match(/[?&]idorvanity=(\d+)/);
    if (m) return m[1];
    return '';
  }

  // Build a canonical post permalink from the IDs we already extracted.
  // Mirror of the Go side fburl.CanonicalPostPermalink — so a lead whose
  // anchor was lazy-rendered still gets a real post URL on the dashboard's
  // "Mở bài viết" button.
  //
  // Uses the /permalink/ URL form (NOT /posts/). /permalink/ is Facebook's
  // canonical group-navigation path and reliably resolves regardless of
  // which internal id (story_fbid vs top_level_post_id) the caller passed.
  // The legacy /posts/ form rejects top_level_post_id post-2026 and was
  // the source of the "content isn't available" production bug.
  function canonicalPostPermalink(groupFBID, postFBID) {
    if (!postFBID) return '';
    if (groupFBID) return `https://www.facebook.com/groups/${groupFBID}/permalink/${postFBID}/`;
    return `https://www.facebook.com/permalink.php?story_fbid=${postFBID}`;
  }

  // True when the URL carries an identifier the dashboard can open as a
  // specific post (not just the group/page feed shell).
  //
  // Photo viewer URLs (/photo/?fbid=X, /photo.php?fbid=X) are EXCLUDED
  // even though they have `?fbid=`. The fbid in those URLs identifies
  // the photo, not the post; the comment system's identity gates check
  // article canonical permalink which on a photo viewer page is the
  // PARENT POST URL (different fbid) → identity_gate failure even if
  // the URL otherwise looked commentable. Reject upstream instead.
  function looksLikePostURL(u) {
    if (!u) return false;
    if (/\/photo(\/|\.|\?|$)/.test(u)) return false;
    return /\/posts\/|\/permalink\/|story_fbid=|multi_permalinks=|[?&]fbid=/.test(u);
  }

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
      if (timeLink) return stripPostQueryParams(THGContentShared.normalizeHref(timeLink.getAttribute('href')));
    }

    return stripPostQueryParams(THGContentShared.normalizeHref(match?.getAttribute('href') || location.href));
  }

  // Drop comment_id and tracking params from a candidate post URL so the
  // returned link opens at the top of the post, not on a specific comment.
  // The path (/permalink/{id}/ or /posts/{id}/) is preserved verbatim.
  function stripPostQueryParams(raw) {
    if (!raw) return raw;
    try {
      const u = new URL(raw, location.origin);
      const drop = [];
      u.searchParams.forEach((_v, k) => {
        if (k === 'comment_id' || k === 'reply_comment_id' || k === 'notif_id' ||
            k === 'notif_t' || k === 'ref' || k.indexOf('__') === 0) {
          drop.push(k);
        }
      });
      drop.forEach(k => u.searchParams.delete(k));
      return u.toString();
    } catch (e) {
      return raw;
    }
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

  // Coarse activity bucket for each safe reason code — lets the operator glance
  // "scrolling / stalled / blocked / done" without parsing the finer code.
  const CRAWL_PHASE_OF = {
    scrolling: 'scrolling',
    no_new_posts: 'stalled', duplicate_heavy: 'stalled', scroll_not_moving: 'stalled',
    login_required: 'blocked', checkpoint_suspected: 'blocked', risk_blocked: 'blocked',
    wrong_page: 'blocked', completed: 'completed', unknown: 'unknown',
  };

  // Maps a raw risk signal from the reused classifiers to its stable reason
  // code. '' when no risk. Kept separate so both the loop's graceful-stop and
  // the progress classifier share ONE mapping.
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
  // text). Observability only — it does NOT decide when the loop stops; the risk
  // codes are set by detectCrawlRisk/detectCrawlBanner in the loop.
  // s: { risk, newCount, duplicateCount, scrollCount, noProgressRounds,
  //      scrollMovedEver, done, reachedMax }
  function classifyCrawlProgress(s) {
    const code = pickCrawlReasonCode(s);
    return { phase: CRAWL_PHASE_OF[code] || 'unknown', safe_reason_code: code };
  }

  // Zero-counter diagnostics for a stop that happens before any scanning
  // (entry-time login/checkpoint wall).
  function zeroCrawlDiag(risk) {
    const c = classifyCrawlProgress({ risk, newCount: 0, duplicateCount: 0, scrollCount: 0, noProgressRounds: 0, scrollMovedEver: false, done: true, reachedMax: false });
    return {
      phase: c.phase, found_count: 0, new_count: 0, duplicate_count: 0,
      scroll_count: 0, no_progress_rounds: 0, scroll_moved_ever: false,
      seconds_since_last_new: 0, safe_reason_code: c.safe_reason_code,
    };
  }

  // Cheap, per-pass URL risk probe (no DOM scan). Reuses the content-script
  // classifier THGNavReport.classifyLanding so we DON'T reinvent detection.
  // Returns '' | 'login' | 'checkpoint'. Defensive: absent classifier → ''.
  function detectCrawlRisk() {
    const nav = globalThis.THGNavReport;
    if (nav && typeof nav.classifyLanding === 'function') {
      const cls = nav.classifyLanding(location.href);
      if (cls === 'login' || cls === 'checkpoint') return cls;
    }
    return '';
  }

  // Text-banner risk probe. Reads body text (via the reused proof.js classifier),
  // so it is ONLY called when a pass yielded zero posts — the checkpoint/block
  // interstitial signature — keeping the steady state free of extra DOM scans.
  // Returns '' | 'rate_limited' | 'blocked' | 'checkpoint'.
  function detectCrawlBanner() {
    const proof = globalThis.THGContentProof;
    if (proof && typeof proof.detectPlatformReject === 'function') {
      return proof.detectPlatformReject() || '';
    }
    return '';
  }

  // Pure builder for the thg_crawl_progress payload. The single seam telemetry
  // (PR-C1B) and checkpoint phase attach to. All inputs are explicit args so
  // this stays side-effect free; the impure location.href read lives in
  // emitProgress. Diagnostics are projected FIELD BY FIELD (whitelist by
  // construction) so no raw page text / DOM / secret can ever leak into a
  // heartbeat. Backward-compatible: omit diag → byte-identical to the C1A shape.
  function buildCrawlProgressMessage(task, accountId, stage, fetched, max, sourceUrl, diag) {
    const msg = {
      type: 'thg_crawl_progress',
      crawler_version: CRAWLER_VERSION,
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

  // Best-effort heartbeat to the background page so users on Telegram can
  // follow the crawl in real time. Failures are intentionally swallowed;
  // missing a heartbeat must never block the crawl itself.
  function emitProgress(task, accountId, stage, fetched, max, diag) {
    try {
      chrome.runtime.sendMessage(
        buildCrawlProgressMessage(task, accountId, stage, fetched, max, location.href, diag)
      ).catch(() => { /* background not listening */ });
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

  // ── P1.3E direct-post permalink target-aware extraction ──────────────────────
  // Broad crawl and direct-post permalink MUST NOT share candidate acceptance. For an
  // explicit direct-post import the extension must extract ONLY the requested post container,
  // prove its post id + group ref, reject sidebar/related/foreign-group/boilerplate, and
  // return a TYPED failure (in crawl_result.error) when the target is not rendered — never a
  // fake analyzable item. The backend directpost.Validate remains the authoritative second line.

  // UI-chrome tokens dropped before measuring "real" post text — mirror of the Go
  // directpost.MeaningfulText so "Facebook Facebook…" / "Like Comment Share" reduce to nothing.
  const DP_CHROME_TOKENS = new Set(['facebook', 'like', 'comment', 'comments', 'share', 'shares',
    'follow', 'following', 'reply', 'replies', 'reactions', 'react']);

  function directPostMeaningful(content) {
    const out = [];
    let prev = '';
    for (const f of String(content || '').split(/\s+/)) {
      const norm = f.toLowerCase().replace(/^[·.,:;!?()[\]{}"'…]+|[·.,:;!?()[\]{}"'…]+$/g, '');
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
    const m = String(url || '').match(/\/groups\/([^/?#]+)/);
    return m ? m[1] : '';
  }

  // directPostVerdict is the PURE per-item gate (mirror of the backend invariants, run here as a
  // pre-filter so a poisoned candidate never even leaves the browser). match=false means "not the
  // requested post id" (keep scanning); match=true+ok=false means "the requested post came back
  // poisoned" with a typed reason; match=true+ok=true means emit it.
  function directPostVerdict(item, target) {
    const tPost = String(target?.post_fbid || '').trim();
    const tGroup = String(target?.group_ref || '').trim();
    const obsPost = String(item?.post_fbid || '').trim() || extractPostFBID(item?.source_url || '');
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

  // buildTargetCandidate extracts one article into the item shape (same helpers the broad loop
  // uses), scoped to a single container so foreign-group anchors elsewhere on the page cannot leak.
  function buildTargetCandidate(article, expectedUrl, groupFBID) {
    const content = THGContentShared.textOf(article);
    if (content.length < 20) return null;
    const author = authorFromArticle(article);
    const url = postPermalink(article);
    let postFBID = extractPostFBID(url);
    if (!postFBID) {
      for (const a of article.querySelectorAll('a[href]')) {
        const id = extractPostFBID(a.getAttribute('href') || '');
        if (id) { postFBID = id; break; }
      }
    }
    let sourceURL = '';
    if (url && looksLikePostURL(url) && url !== location.href) sourceURL = url;
    else if (postFBID) sourceURL = canonicalPostPermalink(groupFBID, postFBID);
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
      const v = directPostVerdict(item, target);
      if (!v.match) continue;
      if (v.ok) return { item };
      if (!poison) poison = v.reason;
    }
    return { reason: poison };
  }

  function directPostResult(task, items, exitReason, error, passes, maxArticles) {
    const cr = {
      crawler_version: CRAWLER_VERSION,
      task_id: task?.task_id || '',
      intent: task?.intent || 'facebook_crawl',
      keywords: [],
      items,
      exit_reason: exitReason,
      direct_post: true,
      scroll_diag: {
        passes, max_articles_seen: maxArticles, max_scroll_y: 0, max_doc_height: 0,
        scroll_moved_ever: false, final_scroll_target: '', landed_url: location.href || '',
      },
    };
    if (error) cr.error = error; // a non-empty error drives the backend's typed direct-post failure
    return { ok: true, crawl_result: cr };
  }

  // crawlDirectPostTarget: bounded, target-only loop. Waits (with small nudges) for the target
  // post container to render, emits ONLY it, and returns a typed failure if it never appears.
  async function crawlDirectPostTarget(task, expectedUrl, accountId, target) {
    const groupFBID = extractGroupFBID(expectedUrl || location.href);
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
    const entryRisk = detectCrawlRisk();
    if (entryRisk) {
      emitProgress(task, accountId, 'finished', 0, 0, zeroCrawlDiag(entryRisk));
      return { ok: false, error: crawlRiskToReason(entryRisk) };
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
      const c = classifyCrawlProgress({
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
      if (pass > 0) await new Promise(r => setTimeout(r, 300));
      const itemsBeforePass = items.length;
      
      const articles = collectPostCandidates();
      // Safety probe: cheap per-pass URL check; the expensive body-text banner
      // check runs ONLY when a pass yielded zero posts (the checkpoint/block
      // interstitial signature), so the steady state adds no DOM scan. On a
      // detected wall, stop gracefully with a typed reason — never bypass.
      const urlRisk = detectCrawlRisk();
      const risk = urlRisk || (articles.length === 0 ? detectCrawlBanner() : '');
      if (risk) {
        riskSignal = risk;
        exitReason = crawlRiskToReason(risk);
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
        let postFBID = extractPostFBID(url);
        if (!postFBID) {
          for (const a of article.querySelectorAll('a[href]')) {
            const id = extractPostFBID(a.getAttribute('href') || '');
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
        if (url && looksLikePostURL(url) && url !== location.href) {
          sourceURL = url;
        } else if (postFBID) {
          sourceURL = canonicalPostPermalink(groupFBID, postFBID);
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
      // After each scroll pass send a heartbeat. Backend rate-limits these so
      // even an aggressive cadence here won't spam Telegram.
      emitProgress(task, accountId, 'scraping', items.length, maxItems, buildLoopDiag(false));
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
    emitProgress(task, accountId, 'finished', items.length, maxItems, buildLoopDiag(true));
    return {
      ok: true,
      crawl_result: {
        crawler_version: CRAWLER_VERSION,
        task_id: task?.task_id || '',
        intent: task?.intent || 'facebook_crawl',
        keywords: Array.isArray(task?.keywords) ? task.keywords : [],
        items,
        exit_reason: exitReason,
        cursor_reached: cursorReached,
        scroll_diag: {
          passes: passesRun,
          max_articles_seen: maxArticles,
          max_scroll_y: maxScrollY,
          max_doc_height: maxDocHeight,
          scroll_moved_ever: scrollMovedEver,
          final_scroll_target: prevScrollTarget,
          landed_url: location.href || '',
        }
      }
    };
  }

  return { crawlVisibleFacebookPosts, buildCrawlProgressMessage, classifyCrawlProgress, crawlRiskToReason, directPostBoilerplate, directPostMeaningful, directPostVerdict, directPostGroupRef, selectDirectPostTargetItem };
})();
globalThis.THGContentCrawl = THGContentCrawl;
// CommonJS export for the node test harness (content/*.test.mjs). No-op in the extension.
if (typeof module !== 'undefined' && module.exports) module.exports = THGContentCrawl;
