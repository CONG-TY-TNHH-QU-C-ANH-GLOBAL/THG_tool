package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	"github.com/thg/scraper/internal/browser"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// FacebookScraper handles scraping Facebook groups using chromedp.
type FacebookScraper struct {
	pool browser.Browser
	db   *store.Store
}

// NewFacebookScraper creates a new Facebook scraper.
func NewFacebookScraper(pool browser.Browser, db *store.Store) *FacebookScraper {
	return &FacebookScraper{pool: pool, db: db}
}

// EnsureLoggedIn checks if Facebook is logged in within the chromedp browser.
// If not logged in, it opens facebook.com and waits for the user to login manually.
func (s *FacebookScraper) EnsureLoggedIn(ctx context.Context) error {
	if s.pool == nil {
		return fmt.Errorf("browser pool not initialized")
	}
	// Use the parent context directly (not from pool) — it's the persistent anchor
	parentCtx := s.pool.ParentCtx()

	checkCtx, cancel := context.WithTimeout(parentCtx, 180*time.Second)
	defer cancel()

	// Navigate to Facebook
	log.Println("[FB] Checking login status...")
	if err := chromedp.Run(checkCtx,
		chromedp.Navigate("https://www.facebook.com"),
		chromedp.Sleep(5*time.Second),
	); err != nil {
		return fmt.Errorf("navigate facebook: %w", err)
	}

	// Check if logged in by looking for profile menu or avatar
	var loggedIn bool
	_ = chromedp.Run(checkCtx, chromedp.Evaluate(`
		// Check multiple indicators of being logged in
		!!document.querySelector('div[aria-label="Your profile"], div[aria-label="Trang cá nhân của bạn"], svg[aria-label="Your profile"], input[aria-label="Search Facebook"], input[aria-label="Tìm kiếm trên Facebook"]')
	`, &loggedIn))

	if loggedIn {
		log.Println("[FB] ✅ Already logged in to Facebook!")
		return nil
	}

	// Not logged in — wait for user to login manually
	log.Println("[FB] ❌ Not logged in to Facebook!")
	log.Println("[FB] 👉 Please login to Facebook in the Chrome window that just opened")
	log.Println("[FB] ⏳ Waiting up to 3 minutes for you to login...")

	// Poll every 5 seconds for login
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-checkCtx.Done():
			return fmt.Errorf("login timeout: please restart and login to Facebook")
		case <-ticker.C:
			var isLoggedIn bool
			_ = chromedp.Run(checkCtx, chromedp.Evaluate(`
				!!document.querySelector('div[aria-label="Your profile"], div[aria-label="Trang cá nhân của bạn"], svg[aria-label="Your profile"], input[aria-label="Search Facebook"], input[aria-label="Tìm kiếm trên Facebook"]')
			`, &isLoggedIn))

			if isLoggedIn {
				log.Println("[FB] ✅ Login successful! Session saved.")
				// Navigate away to release the page
				_ = chromedp.Run(checkCtx, chromedp.Navigate("about:blank"))
				return nil
			}
			log.Println("[FB] ⏳ Still waiting for login...")
		}
	}
}

// ScrapeGroup scrapes posts from a Facebook group.
func (s *FacebookScraper) ScrapeGroup(ctx context.Context, group models.Group) ([]models.Post, error) {
	bCtx, err := s.pool.Acquire(30 * time.Second)
	if err != nil {
		return nil, fmt.Errorf("acquire browser: %w", err)
	}
	defer s.pool.Release(bCtx)

	// Create a timeout context for this scrape (3 minutes for deep scrolling)
	scrapeCtx, cancel := context.WithTimeout(bCtx.Ctx, 180*time.Second)
	defer cancel()

	// Navigate to group with chronological sorting
	groupURL := group.URL
	if !strings.Contains(groupURL, "sorting_setting") {
		if strings.Contains(groupURL, "?") {
			groupURL += "&sorting_setting=CHRONOLOGICAL"
		} else {
			groupURL += "?sorting_setting=CHRONOLOGICAL"
		}
	}

	log.Printf("[FB] Navigating to %s", group.Name)

	// Navigate and wait for content — inject stealth JS before Facebook loads
	if err := chromedp.Run(scrapeCtx,
		// Stealth: override webdriver detection before page loads
		chromedp.Evaluate(`
			Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
			Object.defineProperty(navigator, 'languages', { get: () => ['vi-VN', 'vi', 'en-US', 'en'] });
			Object.defineProperty(navigator, 'plugins', { get: () => [1, 2, 3, 4, 5] });
			window.chrome = { runtime: {} };
		`, nil),
		chromedp.Navigate(groupURL),
		chromedp.Sleep(5*time.Second), // initial wait for SPA
	); err != nil {
		return nil, fmt.Errorf("navigate %s: %w", group.Name, err)
	}

	// DEBUG: capture page title and URL to detect login walls / redirects
	var pageTitle, pageURL string
	_ = chromedp.Run(scrapeCtx,
		chromedp.Title(&pageTitle),
		chromedp.Location(&pageURL),
	)
	log.Printf("[FB] DEBUG: title=%q, url=%q", pageTitle, pageURL)

	// Dismiss cookie consent + revert any Facebook in-post translation to original
	_ = chromedp.Run(scrapeCtx,
		chromedp.Evaluate(`
			document.querySelectorAll('button[data-cookiebanner="accept_button"], button[title="Allow all cookies"], button[title="Cho phép tất cả cookie"]').forEach(b => b.click());
			// Nếu Facebook đang hiển thị bản dịch, click "Xem bản gốc" / "See Original" để lấy nội dung gốc
			document.querySelectorAll('span, div, a').forEach(el => {
				const t = el.textContent.trim();
				if (t === 'Xem bản gốc' || t === 'See Original' || t === 'See original') {
					el.click();
				}
			});
			'done'
		`, nil),
	)
	log.Printf("[FB] Page loaded, checking for feed...")

	// Wait for feed to appear (with timeout fallback)
	// Wait for feed with timeout (10s), then try clicking Discussion tab
	feedCtx, feedCancel := context.WithTimeout(scrapeCtx, 10*time.Second)
	feedErr := chromedp.Run(feedCtx,
		chromedp.WaitVisible(`div[role="feed"]`, chromedp.ByQuery),
	)
	feedCancel()

	if feedErr != nil {
		log.Printf("[FB] Feed not visible, trying to click Discussion tab...")
		// Click "Thảo luận" / "Discussion" tab
		_ = chromedp.Run(scrapeCtx,
			chromedp.Evaluate(`
				// Try clicking Discussion/Thảo luận tab
				const tabs = document.querySelectorAll('a[role="tab"], a[href*="/discussion"]');
				for (const tab of tabs) {
					if (tab.textContent.includes('Thảo luận') || tab.textContent.includes('Discussion')) {
						tab.click();
						break;
					}
				}
				'clicked'
			`, nil),
		)
		time.Sleep(5 * time.Second)

		// Retry waiting for feed
		feedCtx2, feedCancel2 := context.WithTimeout(scrapeCtx, 10*time.Second)
		feedErr = chromedp.Run(feedCtx2,
			chromedp.WaitVisible(`div[role="feed"]`, chromedp.ByQuery),
		)
		feedCancel2()
	}

	if feedErr != nil {
		// Still no feed — check why
		var bodyText string
		_ = chromedp.Run(scrapeCtx, chromedp.Text(`body`, &bodyText, chromedp.ByQuery))
		bodySnippet := bodyText
		if len(bodySnippet) > 500 {
			bodySnippet = bodySnippet[:500]
		}
		log.Printf("[FB] DEBUG: feed not found, body snippet: %s", bodySnippet)

		if strings.Contains(bodyText, "Join Group") || strings.Contains(bodyText, "Tham gia nhóm") {
			log.Printf("[FB] Not a member of %s, attempting auto-join", group.Name)
			return nil, s.handleAutoJoin(scrapeCtx, group)
		}
		if strings.Contains(bodyText, "Log in") || strings.Contains(bodyText, "Đăng nhập") {
			return nil, fmt.Errorf("[FB] ❌ Not logged in! Please login Facebook in Chrome profile first")
		}
		// Continue anyway — try to extract whatever articles are present
		log.Printf("[FB] ⚠️ Feed container not found, attempting extraction anyway...")
	} else {
		log.Printf("[FB] ✅ Feed loaded successfully")
	}

	// ===== ENSURE NEWEST POSTS ARE SHOWING =====
	// Facebook often shows featured/top posts instead of newest.
	// We must explicitly click to sort by newest.
	_ = chromedp.Run(scrapeCtx,
		chromedp.Evaluate(`
			(async () => {
				// 1. Click "Thảo luận" / "Discussion" tab if not active
				const tabs = document.querySelectorAll('a[role="tab"], a span');
				for (const tab of tabs) {
					const text = tab.textContent.trim();
					if (text === 'Thảo luận' || text === 'Discussion') {
						tab.click();
						break;
					}
				}

				// Wait for tab switch
				await new Promise(r => setTimeout(r, 2000));

				// 2. Click "New posts" / "Bài viết mới" notification if present
				const newPostBtns = document.querySelectorAll('div[role="button"], span[role="button"]');
				for (const btn of newPostBtns) {
					const text = btn.textContent.trim().toLowerCase();
					if (text.includes('new post') || text.includes('bài viết mới') || 
					    text.includes('new activity') || text.includes('hoạt động mới')) {
						btn.click();
						await new Promise(r => setTimeout(r, 2000));
						break;
					}
				}

				// 3. Click sort dropdown and select "Newest" / "Mới nhất"
				// Look for the sort menu button (usually near top of feed)
				const sortBtns = document.querySelectorAll('div[role="button"][aria-haspopup="menu"], div[role="button"][aria-haspopup="listbox"]');
				for (const btn of sortBtns) {
					const text = btn.textContent.trim().toLowerCase();
					if (text.includes('sắp xếp') || text.includes('sort') || 
					    text.includes('đề xuất') || text.includes('hoạt động gần') ||
					    text.includes('relevant') || text.includes('recent')) {
						btn.click();
						await new Promise(r => setTimeout(r, 1500));
						
						// Click "Mới nhất" / "Newest" / "New Activity" in dropdown menu
						const menuItems = document.querySelectorAll('div[role="menuitem"], div[role="menuitemradio"], div[role="option"]');
						for (const item of menuItems) {
							const itemText = item.textContent.trim().toLowerCase();
							if (itemText.includes('mới nhất') || itemText.includes('newest') || 
							    itemText.includes('new activity') || itemText.includes('hoạt động mới') ||
							    itemText.includes('recent activity')) {
								item.click();
								await new Promise(r => setTimeout(r, 2000));
								break;
							}
						}
						break;
					}
				}

				return 'done';
			})()
		`, nil),
	)
	log.Printf("[FB] Ensured newest posts sort order")
	time.Sleep(3 * time.Second)

	// DEBUG: count articles before scrolling
	var articleCount int64
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(`document.querySelectorAll('div[role="article"]').length`, &articleCount))
	log.Printf("[FB] DEBUG: %d articles visible before scrolling", articleCount)

	// ===== SCROLL-AND-EXTRACT: Extract posts DURING scrolling =====
	// Facebook uses virtual scrolling — old posts are removed from DOM as you scroll.
	// We must extract at each scroll step to catch posts before they vanish.

	const (
		maxScrolls     = 40
		scrollDistance = 1500                    // Smaller scroll = more accurate DOM loading
		scrollDelay    = 3000 * time.Millisecond // Slower = wait for DOM to fully render
		targetPosts    = 30
		noGrowthMax    = 10
	)

	allPosts := make(map[string]models.Post) // dedup by content hash
	var lastHeight int64
	noGrowthCount := 0

	for i := 0; i < maxScrolls; i++ {
		// Extract posts from CURRENT DOM state
		extracted, err := s.extractPosts(scrapeCtx, group)
		if err != nil {
			log.Printf("[FB] Extract error at scroll %d: %v", i, err)
		} else {
			for _, p := range extracted {
				if p.DedupHash != "" {
					allPosts[p.DedupHash] = p
				}
			}
		}

		// Check if we have enough posts
		if len(allPosts) >= targetPosts {
			log.Printf("[FB] ✅ Reached target: %d posts collected after %d scrolls", len(allPosts), i+1)
			break
		}

		// Scroll down
		var currentHeight int64
		if err := chromedp.Run(scrapeCtx,
			chromedp.Evaluate(fmt.Sprintf(`
				window.scrollBy(0, %d);
				Math.max(document.body.scrollHeight, document.documentElement.scrollHeight, document.body.offsetHeight)
			`, scrollDistance), &currentHeight),
		); err != nil {
			log.Printf("[FB] Scroll error: %v", err)
			break
		}

		time.Sleep(scrollDelay)

		// Check height growth
		if i >= 5 {
			if currentHeight <= lastHeight {
				noGrowthCount++
				// Extra wait when page isn't growing (helps background tabs load content)
				if noGrowthCount%3 == 0 {
					time.Sleep(2 * time.Second)
				}
				if noGrowthCount >= noGrowthMax {
					log.Printf("[FB] Auto-stop scroll at %d (no height growth), %d posts collected", i+1, len(allPosts))
					break
				}
			} else {
				noGrowthCount = 0
			}
		}
		lastHeight = currentHeight

		// Log progress
		if (i+1)%10 == 0 {
			log.Printf("[FB] Scrolled %d/%d, height: %d, posts so far: %d", i+1, maxScrolls, currentHeight, len(allPosts))
		}
	}

	// Final extraction pass
	if finalPosts, err := s.extractPosts(scrapeCtx, group); err == nil {
		for _, p := range finalPosts {
			if p.DedupHash != "" {
				allPosts[p.DedupHash] = p
			}
		}
	}

	// Convert map to slice
	posts := make([]models.Post, 0, len(allPosts))
	for _, p := range allPosts {
		posts = append(posts, p)
	}

	log.Printf("[FB] Extracted %d total unique posts from %s", len(posts), group.Name)

	// CLICK-INTO-POST: resolve group-only URLs by ACTUALLY clicking into each post
	// Anonymous groups don't have proper href in timestamp links, but clicking them
	// triggers JS navigation and the browser URL changes to /permalink/xxx/.
	grpURL := group.URL
	grpClean := strings.TrimRight(grpURL, "/")

	// Find posts that need URL resolution
	var resolveIndices []int
	for i, p := range posts {
		pClean := strings.TrimRight(p.URL, "/")
		hasPostID, _ := regexp.MatchString(`\d{10,}`, p.URL)
		if pClean == grpClean || !hasPostID {
			resolveIndices = append(resolveIndices, i)
		}
	}

	if len(resolveIndices) > 0 {
		log.Printf("[FB] 🔍 Resolving %d posts with group-only URLs via click-into-post...", len(resolveIndices))

		// Scroll back to top first
		_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(`window.scrollTo(0, 0)`, nil))
		time.Sleep(2 * time.Second)

		resolved := 0
		for _, idx := range resolveIndices {
			if resolved >= 15 { // limit to prevent very long runs
				break
			}

			contentSnippet := posts[idx].Content
			if len(contentSnippet) > 40 {
				contentSnippet = contentSnippet[:40]
			}
			// Escape for JS string
			contentSnippet = strings.ReplaceAll(contentSnippet, `\`, `\\`)
			contentSnippet = strings.ReplaceAll(contentSnippet, `"`, `\"`)
			contentSnippet = strings.ReplaceAll(contentSnippet, "'", `\'`)
			contentSnippet = strings.ReplaceAll(contentSnippet, "\n", " ")
			contentSnippet = strings.ReplaceAll(contentSnippet, "\r", "")

			// Step 1: Find the article and CLICK its timestamp/comment link
			clickJS := fmt.Sprintf(`
			(() => {
				const articles = document.querySelectorAll('div[role="article"]');
				for (const art of articles) {
					const text = (art.innerText || '').substring(0, 500);
					if (text.includes("%s")) {
						// Scroll article into view
						art.scrollIntoView({block: 'center'});

						// Try to click the timestamp link (most reliable for opening post)
						const allLinks = art.querySelectorAll('a[href]');
						for (const link of allLinks) {
							const linkText = (link.textContent || '').trim();
							// Timestamp patterns: "4m", "7m", "1h", "2h", etc.
							if (linkText.length > 0 && linkText.length <= 20 && /^\d+\s*[mhsdw]/i.test(linkText)) {
								link.click();
								return 'clicked_timestamp';
							}
						}

						// Fallback: click "X comments" link
						for (const link of allLinks) {
							const linkText = (link.textContent || '').trim().toLowerCase();
							if (/\d+\s*(comment|bình luận)/i.test(linkText)) {
								link.click();
								return 'clicked_comments';
							}
						}

						// Fallback: click any link that might open the post
						for (const link of allLinks) {
							const href = link.href || '';
							if (href.includes('/groups/') && !href.includes('/user/')) {
								link.click();
								return 'clicked_link';
							}
						}

						return 'no_clickable_link';
					}
				}
				return 'not_found';
			})()
			`, contentSnippet)

			var clickResult string
			if err := chromedp.Run(scrapeCtx, chromedp.Evaluate(clickJS, &clickResult)); err != nil {
				log.Printf("[FB] Click error for post %d: %v", idx, err)
				continue
			}

			if clickResult == "not_found" || clickResult == "no_clickable_link" {
				continue
			}

			// Step 2: Wait for URL to change (poll every 500ms, max 5s)
			var newURL string
			for attempt := 0; attempt < 10; attempt++ {
				time.Sleep(500 * time.Millisecond)
				if err := chromedp.Run(scrapeCtx, chromedp.Evaluate(`window.location.href`, &newURL)); err != nil {
					break
				}
				// Check if we navigated to a post URL
				if strings.Contains(newURL, "/permalink/") || strings.Contains(newURL, "/posts/") {
					hasID, _ := regexp.MatchString(`\d{10,}`, newURL)
					if hasID {
						break
					}
				}
			}

			// Step 3: Capture and save the URL
			if newURL != "" && newURL != grpURL {
				hasID, _ := regexp.MatchString(`\d{10,}`, newURL)
				if hasID {
					cleanURL := strings.Split(newURL, "?")[0]
					posts[idx].URL = cleanURL
					resolved++
					log.Printf("[FB] ✅ Resolved URL #%d: %s → %s", resolved, posts[idx].Author, cleanURL)
				}
			}

			// Step 4: Go back to feed
			_ = chromedp.Run(scrapeCtx,
				chromedp.Evaluate(`history.back()`, nil),
			)
			time.Sleep(2 * time.Second)

			// Wait for feed to be available again
			feedWaitCtx, feedWaitCancel := context.WithTimeout(scrapeCtx, 5*time.Second)
			_ = chromedp.Run(feedWaitCtx,
				chromedp.WaitVisible(`div[role="feed"]`, chromedp.ByQuery),
			)
			feedWaitCancel()
		}

		log.Printf("[FB] 🔍 Resolved %d/%d group-only URLs", resolved, len(resolveIndices))
	}

	// ===== PRE-SAVE URL FILTER =====
	// Now that click-into-post has resolved URLs, reject posts that STILL have no valid post link
	var validPosts []models.Post
	skippedNoURL := 0
	for _, p := range posts {
		pClean := strings.TrimRight(p.URL, "/")
		if p.URL == "" || pClean == grpClean {
			skippedNoURL++
			log.Printf("[FB] ⏩ Skipped post with no valid URL (after resolution): author=%s", p.Author)
			continue
		}
		validPosts = append(validPosts, p)
	}
	if skippedNoURL > 0 {
		log.Printf("[FB] 🛡️ URL filter: %d posts skipped (no valid URL after resolution), %d posts kept", skippedNoURL, len(validPosts))
	}
	posts = validPosts

	// Save to database
	saved := 0
	for i := range posts {
		if id, err := s.db.InsertPost(&posts[i]); err == nil {
			posts[i].ID = id // Set DB ID so classifier can link leads back to posts
			saved++
		}
	}
	log.Printf("[FB] Saved %d new posts from %s", saved, group.Name)

	// Update last scan timestamp
	_ = s.db.UpdateGroupLastScan(group.ID)

	return posts, nil
}

// extractPosts parses posts from the Facebook DOM using chromedp.
// Uses content-hash dedup to prevent extracting the same post twice.
func (s *FacebookScraper) extractPosts(ctx context.Context, group models.Group) ([]models.Post, error) {
	// JavaScript to extract post data from modern Facebook DOM
	extractJS := `
	(() => {
		const posts = [];
		const seen = new Set();

		// Find posts via feed children (modern FB layout) - Most reliable to avoid comments
		let units = [];
		const feed = document.querySelector('div[role="feed"]');
		if (feed) {
			const feedChildren = feed.querySelectorAll(':scope > div');
			for (const child of feedChildren) {
				const textContent = child.innerText || '';
				// Must have some content, and usually has an ID or class
				if (textContent.length > 50) {
					units.push(child);
				}
			}
		}

		const totalUnits = units.length;

		for (const unit of units) {
			try {
				// Author — ONLY from post HEADER, not comments
				let author = '';
				let authorURL = '';

				// Strategy 1: First heading (h2/h3/h4) with a link — this is ALWAYS the post author
				const headings = unit.querySelectorAll('h2, h3, h4');
				for (const heading of headings) {
					const link = heading.querySelector('a');
					if (link) {
						const nameEl = link.querySelector('strong') || link.querySelector('span') || link;
						const name = nameEl.textContent.trim();
						// Validate: real name (2+ chars, not too long, not a URL fragment)
						if (name.length > 1 && name.length < 80 && !name.includes('.com') && !name.match(/^[a-zA-Z0-9]{8,}$/)) {
							author = name;
							authorURL = link.href;
							break;
						}
					}
				}

				// Strategy 2: First strong in the FIRST child div (skip comment areas)
				if (!author) {
					const firstChild = unit.children[0];
					if (firstChild) {
						const strong = firstChild.querySelector('strong');
						if (strong) {
							const name = strong.textContent.trim();
							if (name.length > 1 && name.length < 80 && !name.includes('.com')) {
								author = name;
								const linkEl = strong.closest('a');
								if (linkEl) authorURL = linkEl.href;
							}
						}
					}
				}

				// Content - POSITION-BASED extraction
				// On Facebook, post body is ABOVE the reaction bar (Like/Comment/Share)
				// Comments are BELOW the reaction bar. We use this as a boundary.
				let content = '';

				// Find the reaction/action bar — this separates post body from comments
				const reactionBar = unit.querySelector('span[role="toolbar"]') ||
					unit.querySelector('div[aria-label*="Like"]') ||
					unit.querySelector('div[aria-label*="Thích"]') ||
					unit.querySelector('div[aria-label*="like"]');

				// Get ALL dir="auto" text blocks in this article
				const allDirAutos = unit.querySelectorAll('div[dir="auto"]');

				if (reactionBar) {
					// POSITION-BASED: only take text BEFORE the reaction bar
					for (const d of allDirAutos) {
						// compareDocumentPosition: bit 4 (FOLLOWING) means d comes BEFORE reactionBar
						const position = reactionBar.compareDocumentPosition(d);
						const isBefore = (position & Node.DOCUMENT_POSITION_PRECEDING) !== 0;
						if (isBefore) {
							const t = d.textContent.trim();
							if (t.length > 15 && t !== author && !t.includes('·') && t.length < 5000) {
								content += t + '\n';
							}
						}
					}
					content = content.trim();
				}

				// Fallback 1: data-ad-preview (works on some FB versions)
				if (!content || content.length < 20) {
					const adPreview = unit.querySelector('div[data-ad-preview="message"]') ||
						unit.querySelector('div[data-ad-comet-preview="message"]');
					if (adPreview && adPreview.textContent.trim().length > 20) {
						content = adPreview.textContent.trim();
					}
				}

				// Fallback 2: FIRST dir="auto" with substantial text (most reliable single block)
				if (!content || content.length < 20) {
					for (const d of allDirAutos) {
						const t = d.textContent.trim();
						if (t.length > 20 && t !== author) {
							content = t;
							break; // Take ONLY the first one — it's the post body
						}
					}
				}

				if (content.length < 20) continue;

				// DEDUP
				const dedupKey = (author + '|||' + content.substring(0, 200)).toLowerCase();
				if (seen.has(dedupKey)) continue;
				seen.add(dedupKey);

				// Timestamp — try multiple sources
				// Timestamp AND Post URL — extract TOGETHER
				// On Facebook, the timestamp text ("4m", "7m", "1h") is always inside an <a> link
				// that points to the EXACT post URL. We extract BOTH from the same element.
				let postedAt = '';
				let postURL = '';

				// Helper: validate this is a GROUP POST url
				const isGroupPostURL = (href) => {
					if (!href) return false;
					if (!href.includes('/groups/')) return false;
					if (href.includes('/posts/') || href.includes('/permalink/') || href.includes('pfbid') || href.includes('story_fbid')) return true;
					// Also accept /groups/xxx/numeric_id (modern FB)
					const match = href.match(/\/groups\/[^\/]+\/(\d{10,})/);
					return !!match;
				};

				// STRATEGY 0: Timestamp link — MOST RELIABLE
				// Scan all <a> links for one with timestamp-like text
				const timestampPatterns = /^\d+\s*[mhsdw]\b|^\d+\s*(min|minute|hour|hr|phút|giờ|tháng|thg|jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)/i;
				const recentPatterns = /just\s*now|vừa\s*xong/i;
				const datePatterns = /^\d{1,2}\s+(tháng|thg|jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)/i;
				
				const allALinks = unit.querySelectorAll('a[href]');

				for (const link of allALinks) {
					// Check link text for timestamp patterns
					const linkText = (link.textContent || '').trim();
					if (linkText.length > 0 && linkText.length <= 25) {
						if (timestampPatterns.test(linkText) || recentPatterns.test(linkText) || datePatterns.test(linkText)) {
							postedAt = linkText;
							// Extract URL from this same link!
							const href = link.href || '';
							if (href.includes('/groups/') && href.includes('facebook.com')) {
								postURL = href.split('?')[0];
							}
							break;
						}
					}
					// Also check nested spans
					const spans = link.querySelectorAll('span[id], span');
					for (const span of spans) {
						const spanText = (span.textContent || '').trim();
						if (spanText.length > 0 && spanText.length <= 25) {
							if (timestampPatterns.test(spanText) || recentPatterns.test(spanText) || datePatterns.test(spanText)) {
								postedAt = spanText;
								const href = link.href || '';
								if (href.includes('/groups/') && href.includes('facebook.com')) {
									postURL = href.split('?')[0];
								}
								break;
							}
						}
					}
					if (postedAt) break;
				}

				// Fallback: abbr element (older FB)
				if (!postedAt) {
					const abbrEl = unit.querySelector('abbr');
					if (abbrEl) {
						postedAt = abbrEl.getAttribute('data-utime') || abbrEl.getAttribute('title') || abbrEl.textContent;
						// Try to get URL from parent <a>
						const parentLink = abbrEl.closest('a[href]');
						if (!postURL && parentLink && parentLink.href.includes('/groups/')) {
							postURL = parentLink.href.split('?')[0];
						}
					}
				}

				// 24-hour freshness filter
				const timeText = (postedAt || '').trim().toLowerCase();
				if (timeText) {
					const isOld =
						/\d+\s*d\b/i.test(timeText) ||
						/\d+\s*w\b/i.test(timeText) ||
						/\d+\s*y\b/i.test(timeText) ||
						/\d+\s*(day|days|week|weeks|month|months|year|years)/i.test(timeText) ||
						/\d+\s*(ngày|tuần|năm)/i.test(timeText) ||
						/yesterday|hôm qua/i.test(timeText) ||
						/^(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)/i.test(timeText) ||
						/^(tháng|thg)/i.test(timeText) ||
						/^\d{1,2}\s+(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)/i.test(timeText) ||
						/^\d{1,2}\s+(tháng|thg)/i.test(timeText) ||
						/^\d{1,2}\/\d{1,2}\/\d{2,4}/.test(timeText);

					if (isOld) {
						continue; // Skip — confirmed old post
					}
				}

				// URL FALLBACK STRATEGIES (only if timestamp link didn't give us a URL)
				// Strategy 1: Standard permalink patterns
				if (!postURL) {
					const permalinks = unit.querySelectorAll('a[href*="/posts/"], a[href*="/permalink/"], a[href*="story_fbid"], a[href*="pfbid"]');
					for (const link of permalinks) {
						if (isGroupPostURL(link.href)) {
							postURL = link.href.split('?')[0];
							break;
						}
					}
				}

				// Strategy 2: ANY link in post that's a valid group post URL
				if (!postURL) {
					for (const link of allALinks) {
						const href = link.href || '';
						if (isGroupPostURL(href)) {
							if (authorURL && href.split('?')[0] === authorURL) continue;
							postURL = href.split('?')[0];
							break;
						}
					}
				}

				// Strategy 3: Link with long numeric ID
				if (!postURL) {
					for (const link of allALinks) {
						const href = link.href || '';
						if (!href.includes('/groups/')) continue;
						if (href.includes('/user/') || href.includes('/photo') || href.includes('/reel')) continue;
						if (authorURL && href.split('?')[0] === authorURL) continue;
						const numMatch = href.match(/\/(\d{10,})/);
						if (numMatch) {
							postURL = href.split('?')[0];
							break;
						}
					}
				}

				// Final validation
				if (postURL && authorURL && postURL === authorURL) {
					postURL = '';
				}

				// NO FALLBACK: leave postURL empty if we can't find a real post link
				// This ensures we never send group-level URLs to the dashboard/Telegram
				// Go-level filter will reject posts with empty or group-only URLs

				// Reactions
				let reactions = 0;
				const reactionSpan = unit.querySelector('span[role="toolbar"]');
				if (reactionSpan) {
					const match = reactionSpan.textContent.match(/(\d+)/);
					if (match) reactions = parseInt(match[1]);
				}

				posts.push({
					author, authorURL,
					content: content.substring(0, 2000),
					postedAt, url: postURL, reactions
				});

				// Also extract COMMENTS as separate entries for AI classification
				// Comments are nested div[role="article"] inside the main post
				const commentArticles = unit.querySelectorAll('div[role="article"]');
				for (const comment of commentArticles) {
					try {
						// Get commenter name
						const commentAuthorEl = comment.querySelector('a[role="link"] span');
						const commentAuthor = commentAuthorEl ? commentAuthorEl.textContent.trim() : '';
						if (!commentAuthor || commentAuthor === author) continue; // skip post author's own comments

						// Get comment text — first div[dir="auto"] in the comment
						let commentText = '';
						const commentDirAutos = comment.querySelectorAll('div[dir="auto"]');
						for (const d of commentDirAutos) {
							const t = d.textContent.trim();
							if (t.length > 10 && t !== commentAuthor) {
								commentText = t;
								break;
							}
						}
						if (!commentText || commentText.length < 10) continue;

						// Dedup check
						const commentDedupKey = (commentAuthor + '|||' + commentText.substring(0, 200)).toLowerCase();
						if (seen.has(commentDedupKey)) continue;
						seen.add(commentDedupKey);

						// Get commenter profile URL
						const commentAuthorLink = comment.querySelector('a[role="link"]');
						const commentAuthorURL = commentAuthorLink ? commentAuthorLink.href.split('?')[0] : '';

						posts.push({
							author: commentAuthor,
							authorURL: commentAuthorURL,
							content: commentText.substring(0, 2000),
							postedAt: postedAt, // same timestamp as parent post
							url: postURL,       // link to parent post
							reactions: 0
						});
					} catch(e) { continue; }
				}

			} catch(e) { continue; }
		}
		return JSON.stringify({total: totalUnits, extracted: posts.length, posts});
	})()
	`

	var resultJSON string
	if err := chromedp.Run(ctx, chromedp.Evaluate(extractJS, &resultJSON)); err != nil {
		return nil, err
	}

	// Parse JSON result
	type postItem struct {
		Author    string `json:"author"`
		AuthorURL string `json:"authorURL"`
		Content   string `json:"content"`
		PostedAt  string `json:"postedAt"`
		URL       string `json:"url"`
		Reactions int    `json:"reactions"`
	}
	type extractResult struct {
		Total     int        `json:"total"`
		Extracted int        `json:"extracted"`
		Posts     []postItem `json:"posts"`
	}

	var result extractResult
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		return nil, fmt.Errorf("parse posts JSON: %w", err)
	}

	log.Printf("[FB] DEBUG: %d DOM elements scanned, %d posts extracted", result.Total, result.Extracted)

	now := time.Now()
	var posts []models.Post
	var skippedOld int

	for _, rp := range result.Posts {
		if rp.Content == "" {
			continue
		}

		// ===== GO-LEVEL FRESHNESS FILTER (if/else) =====
		// Parse the timestamp text from JS (e.g. "4m", "7h", "3d", "yesterday")
		// and reject anything older than ~24 hours.
		timeText := strings.TrimSpace(strings.ToLower(rp.PostedAt))
		isFresh := false

		if timeText == "" {
			// No timestamp found → ACCEPT (trust CHRONOLOGICAL sort — visible posts are recent)
			isFresh = true
			log.Printf("[FB] ℹ️ No timestamp found, accepting (chronological sort): author=%s", rp.Author)
		} else if regexp.MustCompile(`^\d+\s*[mM]($|\s)`).MatchString(timeText) ||
			regexp.MustCompile(`^\d+\s*(min|minute|minutes)`).MatchString(timeText) ||
			strings.Contains(timeText, "phút") {
			// Minutes ago → FRESH
			isFresh = true
		} else if regexp.MustCompile(`^\d+\s*[hH]($|\s)`).MatchString(timeText) ||
			regexp.MustCompile(`^\d+\s*(hour|hours|hr|hrs)`).MatchString(timeText) ||
			strings.Contains(timeText, "giờ") {
			// Hours ago → check if < 24h
			re := regexp.MustCompile(`^(\d+)`)
			if m := re.FindStringSubmatch(timeText); len(m) > 1 {
				if hours, err := strconv.Atoi(m[1]); err == nil && hours < 24 {
					isFresh = true
				}
			}
		} else if regexp.MustCompile(`^\d+\s*[sS]($|\s)`).MatchString(timeText) ||
			regexp.MustCompile(`^\d+\s*(sec|second|seconds)`).MatchString(timeText) ||
			strings.Contains(timeText, "giây") {
			// Seconds ago → FRESH
			isFresh = true
		} else if regexp.MustCompile(`just\s*now`).MatchString(timeText) ||
			strings.Contains(timeText, "vừa xong") || strings.Contains(timeText, "vừa") {
			// Just now → FRESH
			isFresh = true
		} else if regexp.MustCompile(`\d+\s*[dD]($|\s)`).MatchString(timeText) ||
			regexp.MustCompile(`\d+\s*(day|days)`).MatchString(timeText) ||
			strings.Contains(timeText, "ngày") ||
			regexp.MustCompile(`\d+\s*[wW]($|\s)`).MatchString(timeText) ||
			regexp.MustCompile(`\d+\s*(week|weeks)`).MatchString(timeText) ||
			strings.Contains(timeText, "tuần") ||
			regexp.MustCompile(`\d+\s*(month|months|year|years)`).MatchString(timeText) ||
			strings.Contains(timeText, "tháng") || strings.Contains(timeText, "thg") ||
			strings.Contains(timeText, "năm") ||
			strings.Contains(timeText, "yesterday") || strings.Contains(timeText, "hôm qua") ||
			regexp.MustCompile(`^(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)`).MatchString(timeText) ||
			regexp.MustCompile(`^\d{1,2}/\d{1,2}/\d{2,4}`).MatchString(timeText) {
			// Days/weeks/months/years/yesterday/date formats → OLD, REJECT
			skippedOld++
			log.Printf("[FB] ⏩ Skipped old post: author=%s, timestamp=%q", rp.Author, timeText)
			continue
		} else {
			// Unknown timestamp format → ACCEPT (trust chronological sort, let AI classify)
			isFresh = true
			log.Printf("[FB] ℹ️ Unknown timestamp %q, accepting (chronological sort): author=%s", timeText, rp.Author)
		}

		if !isFresh {
			skippedOld++
			log.Printf("[FB] ⏩ Skipped old post: author=%s, timestamp=%q", rp.Author, timeText)
			continue
		}

		// NOTE: URL filter moved to ScrapeGroup() — AFTER click-into-post resolution
		// Posts without URLs still have a chance to be resolved before being rejected

		dedupHash := store.DedupHash(
			string(models.PlatformFacebook), "post",
			rp.URL, rp.Author, rp.PostedAt, rp.Content,
		)

		posts = append(posts, models.Post{
			Platform:  models.PlatformFacebook,
			GroupID:   group.ID,
			GroupName: group.Name,
			URL:       rp.URL,
			Author:    rp.Author,
			AuthorURL: rp.AuthorURL,
			Content:   rp.Content,
			Reactions: rp.Reactions,
			PostedAt:  now,
			DedupHash: dedupHash,
		})
	}

	if skippedOld > 0 {
		log.Printf("[FB] 🛡️ Freshness filter: %d old skipped, %d accepted", skippedOld, len(posts))
	}

	return posts, nil
}

// handleAutoJoin attempts to join a Facebook group.
func (s *FacebookScraper) handleAutoJoin(ctx context.Context, group models.Group) error {
	log.Printf("[FB] Attempting to join group: %s", group.Name)

	// Try clicking Join button
	joinSelectors := []string{
		`div[role="button"]:has(span:contains("Join"))`,
		`div[aria-label="Join group"]`,
		`div[aria-label="Tham gia nhóm"]`,
	}

	for _, sel := range joinSelectors {
		var nodes []*cdp.Node
		if err := chromedp.Run(ctx, chromedp.Nodes(sel, &nodes, chromedp.ByQuery)); err == nil && len(nodes) > 0 {
			if err := chromedp.Run(ctx, chromedp.Click(sel, chromedp.ByQuery)); err == nil {
				log.Printf("[FB] Clicked join button for %s", group.Name)
				// Update group join state
				// In a real scenario, check if instant or pending
				return nil
			}
		}
	}

	return fmt.Errorf("could not find join button for %s", group.Name)
}

// ScrapeComments scrapes comments from a specific post.
func (s *FacebookScraper) ScrapeComments(ctx context.Context, postURL string, postID int64) ([]models.Comment, error) {
	bCtx, err := s.pool.Acquire(30 * time.Second)
	if err != nil {
		return nil, fmt.Errorf("acquire browser: %w", err)
	}
	defer s.pool.Release(bCtx)

	scrapeCtx, cancel := context.WithTimeout(bCtx.Ctx, 60*time.Second)
	defer cancel()

	if err := chromedp.Run(scrapeCtx,
		chromedp.Navigate(postURL),
		chromedp.Sleep(5*time.Second),
	); err != nil {
		return nil, fmt.Errorf("navigate to post: %w", err)
	}

	// Click "View more comments" buttons
	for i := 0; i < 3; i++ {
		var nodes []*cdp.Node
		_ = chromedp.Run(scrapeCtx,
			chromedp.Nodes(`div[role="button"]:has-text("View more comments"), div[role="button"]:has-text("Xem thêm bình luận")`, &nodes, chromedp.ByQuery),
		)
		if len(nodes) == 0 {
			break
		}
		_ = chromedp.Run(scrapeCtx,
			chromedp.Click(nodes[0].FullXPath(), chromedp.BySearch),
			chromedp.Sleep(2*time.Second),
		)
	}

	// Extract comments
	extractJS := `
	(() => {
		const comments = [];
		const commentEls = document.querySelectorAll('div[role="article"] ul div[role="article"]');
		for (const el of commentEls) {
			try {
				const authorEl = el.querySelector('a[role="link"] span');
				const contentEl = el.querySelector('div[dir="auto"]');
				if (!contentEl) continue;
				comments.push({
					author: authorEl ? authorEl.textContent.trim() : '',
					authorURL: authorEl ? authorEl.closest('a').href : '',
					content: contentEl.textContent.trim().substring(0, 1000)
				});
			} catch(e) { continue; }
		}
		return JSON.stringify(comments);
	})()
	`

	var resultJSON string
	if err := chromedp.Run(scrapeCtx, chromedp.Evaluate(extractJS, &resultJSON)); err != nil {
		return nil, err
	}

	type rawComment struct {
		Author    string `json:"author"`
		AuthorURL string `json:"authorURL"`
		Content   string `json:"content"`
	}

	var rawComments []rawComment
	if err := json.Unmarshal([]byte(resultJSON), &rawComments); err != nil {
		return nil, err
	}

	now := time.Now()
	var comments []models.Comment
	for _, rc := range rawComments {
		if rc.Content == "" {
			continue
		}
		dedupHash := store.DedupHash(
			string(models.PlatformFacebook), "comment",
			postURL, rc.Author, "", rc.Content,
		)
		comment := models.Comment{
			PostID:    postID,
			Platform:  models.PlatformFacebook,
			Author:    rc.Author,
			AuthorURL: rc.AuthorURL,
			Content:   rc.Content,
			PostedAt:  now,
			DedupHash: dedupHash,
		}
		if _, err := s.db.InsertComment(&comment); err == nil {
			comments = append(comments, comment)
		}
	}

	log.Printf("[FB] Extracted %d comments from post", len(comments))
	return comments, nil
}

// CandidateComment represents a comment with job-seeking intent from a specific post.
type CandidateComment struct {
	Author    string
	AuthorURL string
	Content   string
	PostURL   string
}

// jobSeekingSignals are keywords that indicate the commenter is looking for a job.
var jobSeekingSignals = []string{
	"tìm việc", "cần việc", "xin việc", "đang tìm việc", "muốn tìm việc",
	"tìm job", "cần job", "xin job", "mình cần việc", "tôi cần việc",
	"open to work", "looking for job", "seeking job", "job opportunity", "looking for work",
	"ib mình", "inbox mình", "inbox me", "dm me", "liên hệ mình", "nhắn mình",
	"chưa có việc", "đang thất nghiệp", "thất nghiệp", "mất việc",
	"muốn chuyển việc", "tìm cơ hội", "seeking opportunity",
	"ứng tuyển", "apply", "cv mình", "gửi cv",
}

// HasJobSeekingIntent returns true if the text shows job-seeking signals.
func HasJobSeekingIntent(text string) bool {
	lower := strings.ToLower(text)
	for _, sig := range jobSeekingSignals {
		if strings.Contains(lower, sig) {
			return true
		}
	}
	return false
}

// ScrapeJobSeekerComments navigates to a post, scrapes all comments, and returns
// only those whose text shows job-seeking intent. Used by the recruitment agent.
func (s *FacebookScraper) ScrapeJobSeekerComments(ctx context.Context, postURL string) ([]CandidateComment, error) {
	bCtx, err := s.pool.Acquire(30 * time.Second)
	if err != nil {
		return nil, fmt.Errorf("acquire browser: %w", err)
	}
	defer s.pool.Release(bCtx)

	scrapeCtx, cancel := context.WithTimeout(bCtx.Ctx, 60*time.Second)
	defer cancel()

	if err := chromedp.Run(scrapeCtx,
		chromedp.Navigate(postURL),
		chromedp.Sleep(5*time.Second),
	); err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}

	// Expand top-level comments (up to 3 clicks)
	for range 3 {
		var done bool
		_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(`
		(() => {
			for (const el of document.querySelectorAll('div[role="button"]')) {
				const t = (el.innerText || '').trim().toLowerCase();
				if (t.includes('xem thêm') || t.includes('view more') || t.includes('more comment')) {
					el.click(); return true;
				}
			}
			return false;
		})()`, &done))
		if !done {
			break
		}
		_ = chromedp.Run(scrapeCtx, chromedp.Sleep(1500*time.Millisecond))
	}

	const extractJS = `
	JSON.stringify((() => {
		const results = [];
		const seen = new Set();
		const commentEls = document.querySelectorAll('div[role="article"] ul div[role="article"]');
		for (const el of commentEls) {
			try {
				const authorLink = el.querySelector('a[role="link"]');
				const authorSpan = authorLink ? authorLink.querySelector('span') : null;
				const author = authorSpan ? authorSpan.textContent.trim() : '';
				const authorURL = authorLink ? authorLink.href.split('?')[0] : '';
				const contentEl = el.querySelector('div[dir="auto"]');
				const content = contentEl ? contentEl.textContent.trim() : '';
				if (!author || !content || content.length < 5) continue;
				const key = author + '|' + content.substring(0, 100);
				if (seen.has(key)) continue;
				seen.add(key);
				results.push({ author, authorURL, content: content.substring(0, 1000) });
			} catch(e) { continue; }
		}
		return results;
	})())`

	var rawJSON string
	if err := chromedp.Run(scrapeCtx, chromedp.Evaluate(extractJS, &rawJSON)); err != nil {
		return nil, fmt.Errorf("extract comments: %w", err)
	}

	var raw []struct {
		Author    string `json:"author"`
		AuthorURL string `json:"authorURL"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		return nil, fmt.Errorf("parse comments: %w", err)
	}

	log.Printf("[FB] Scanned %d comments on %s for job-seeking intent", len(raw), postURL)

	var candidates []CandidateComment
	for _, c := range raw {
		if HasJobSeekingIntent(c.Content) {
			candidates = append(candidates, CandidateComment{
				Author:    c.Author,
				AuthorURL: c.AuthorURL,
				Content:   c.Content,
				PostURL:   postURL,
			})
		}
	}
	log.Printf("[FB] Found %d job-seeker comments on %s", len(candidates), postURL)
	return candidates, nil
}

// DiscoveredGroup holds metadata about a group found via Facebook Search.
type DiscoveredGroup struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	MemberCount string `json:"memberCount"`
	Privacy     string `json:"privacy"` // "Public" or "Private"
	Description string `json:"description"`
}

// SearchGroups searches Facebook for groups matching the query.
// Returns discovered groups filtered for public groups with 1K+ members.
func (s *FacebookScraper) SearchGroups(ctx context.Context, query string) ([]DiscoveredGroup, error) {
	if s.pool == nil {
		return nil, fmt.Errorf("browser pool not initialized")
	}

	bCtx, err := s.pool.Acquire(30 * time.Second)
	if err != nil {
		return nil, fmt.Errorf("acquire browser: %w", err)
	}
	defer s.pool.Release(bCtx)

	scrapeCtx, cancel := context.WithTimeout(bCtx.Ctx, 90*time.Second)
	defer cancel()

	// URL-encode the query
	searchURL := fmt.Sprintf("https://www.facebook.com/search/groups/?q=%s", strings.ReplaceAll(query, " ", "%%20"))
	log.Printf("[FB Search] Searching groups: %q → %s", query, searchURL)

	// Navigate to Facebook group search
	if err := chromedp.Run(scrapeCtx,
		chromedp.Evaluate(`
			Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
			window.chrome = { runtime: {} };
		`, nil),
		chromedp.Navigate(searchURL),
		chromedp.Sleep(5*time.Second),
	); err != nil {
		return nil, fmt.Errorf("navigate search: %w", err)
	}

	// Check if logged in
	var loggedIn bool
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(`
		!!document.querySelector('input[aria-label="Search Facebook"], input[aria-label="Tìm kiếm trên Facebook"]')
	`, &loggedIn))
	if !loggedIn {
		return nil, fmt.Errorf("not logged in to Facebook — please login first")
	}

	// Scroll to load more results (3 scrolls)
	for i := 0; i < 3; i++ {
		_ = chromedp.Run(scrapeCtx,
			chromedp.Evaluate(`window.scrollBy(0, 1200)`, nil),
		)
		time.Sleep(2 * time.Second)
	}

	// Extract group cards from search results
	extractJS := `
	JSON.stringify((() => {
		const groups = [];
		const seen = new Set();
		
		// Find all links that point to Facebook groups
		const allLinks = document.querySelectorAll('a[href*="/groups/"]');
		
		for (const link of allLinks) {
			const href = (link.href || '').split('?')[0].replace(/\/$/, '');
			
			// Must be a group link (not a post within a group)
			if (!href.match(/facebook\.com\/groups\/[^\/]+$/)) continue;
			// Skip duplicates
			if (seen.has(href)) continue;
			
			// Get the closest container element
			const container = link.closest('div[class]') || link.parentElement;
			if (!container) continue;
			
			// Extract group name from the link or nearby heading
			let name = '';
			const heading = link.querySelector('span') || link;
			name = heading.textContent.trim();
			if (!name || name.length < 3 || name.length > 100) continue;
			
			// Skip navigation/header links
			if (name.toLowerCase().includes('search') || name.toLowerCase().includes('tìm kiếm')) continue;
			
			// Extract full text of the container for metadata
			const fullText = container.textContent || '';
			
			// Member count
			let memberCount = '';
			const memberMatch = fullText.match(/([\d,.]+[KkMm]?)\s*(members|thành viên)/i);
			if (memberMatch) {
				memberCount = memberMatch[1] + ' ' + memberMatch[2];
			}
			
			// Privacy
			let privacy = 'Unknown';
			if (fullText.toLowerCase().includes('public') || fullText.toLowerCase().includes('công khai')) {
				privacy = 'Public';
			} else if (fullText.toLowerCase().includes('private') || fullText.toLowerCase().includes('riêng tư')) {
				privacy = 'Private';
			}
			
			// Description snippet
			let description = '';
			const descSpans = container.querySelectorAll('span');
			for (const span of descSpans) {
				const t = span.textContent.trim();
				if (t.length > 30 && t.length < 300 && t !== name && !t.includes('members') && !t.includes('thành viên')) {
					description = t;
					break;
				}
			}
			
			seen.add(href);
			groups.push({
				name: name,
				url: href,
				memberCount: memberCount,
				privacy: privacy,
				description: description.substring(0, 200)
			});
		}
		
		return groups;
	})())`

	var rawJSON string
	if err := chromedp.Run(scrapeCtx, chromedp.Evaluate(extractJS, &rawJSON)); err != nil {
		return nil, fmt.Errorf("extract groups: %w", err)
	}

	var groups []DiscoveredGroup
	if err := json.Unmarshal([]byte(rawJSON), &groups); err != nil {
		return nil, fmt.Errorf("parse groups: %w", err)
	}

	// Filter: prefer public groups with decent member counts
	var filtered []DiscoveredGroup
	for _, g := range groups {
		// Skip private groups
		if g.Privacy == "Private" {
			continue
		}
		// Skip groups with very low member count (< 1K)
		if g.MemberCount != "" && !hasEnoughMembers(g.MemberCount) {
			continue
		}
		filtered = append(filtered, g)
	}

	log.Printf("[FB Search] Found %d total groups, %d public groups with 1K+ members", len(groups), len(filtered))

	// Limit to top 10
	if len(filtered) > 10 {
		filtered = filtered[:10]
	}

	return filtered, nil
}

// hasEnoughMembers checks if the member count string represents 1000+ members.
func hasEnoughMembers(memberStr string) bool {
	lower := strings.ToLower(memberStr)
	// Contains K or M suffix → likely 1K+ or 1M+
	if strings.Contains(lower, "k") || strings.Contains(lower, "m") {
		return true
	}
	// Parse raw number
	numStr := strings.ReplaceAll(memberStr, ",", "")
	numStr = strings.ReplaceAll(numStr, ".", "")
	// Extract digits
	var digits string
	for _, c := range numStr {
		if c >= '0' && c <= '9' {
			digits += string(c)
		}
	}
	if digits == "" {
		return true // can't determine, include it
	}
	n, err := strconv.Atoi(digits)
	if err != nil {
		return true
	}
	return n >= 1000
}
