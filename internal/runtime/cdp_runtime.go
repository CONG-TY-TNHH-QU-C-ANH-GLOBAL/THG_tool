package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/thg/scraper/internal/fburl"
)

// Ensure json is used for unmarshaling extracted post data.

// CDPRuntime connects to an existing Chrome instance via CDP and scrapes
// Facebook group posts by opening a temporary background tab so the user's
// active tab is never interrupted.
type CDPRuntime struct {
	wsURL string // Chrome DevTools WebSocket URL
}

// NewCDPRuntime probes the CDP endpoint on cdpPort and returns a runtime
// ready to scrape. Returns an error if Chrome is unreachable.
func NewCDPRuntime(cdpPort int) (*CDPRuntime, error) {
	wsURL, err := chromeWSURL(cdpPort)
	if err != nil {
		return nil, Wrap(ErrChromeUnreachable, fmt.Sprintf("probe port %d", cdpPort), err)
	}
	log.Printf("[CDPRuntime] Connected to Chrome at %s", wsURL)
	return &CDPRuntime{wsURL: wsURL}, nil
}

// FetchBatch opens a new background tab in the connected Chrome instance,
// navigates to sourceURL, and extracts posts via JS injection, then closes
// the tab. The user's active tab is never touched.
// Stateless single-batch mode: returns nil when offset > 0.
func (r *CDPRuntime) FetchBatch(ctx context.Context, sourceURL string, offset, batchSize int) ([]RawItem, error) {
	if offset > 0 {
		return nil, nil
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, r.wsURL)
	defer allocCancel()

	// Always open a fresh background tab so the user's active Facebook tab is
	// never navigated away from during the crawl.
	tabCtx, tabCancel := chromedp.NewContext(allocCtx)
	defer tabCancel()

	tabCtx, timeoutCancel := context.WithTimeout(tabCtx, 60*time.Second)
	defer timeoutCancel()

	var rawJSON string
	if strings.Contains(sourceURL, "/search/groups") {
		err := chromedp.Run(tabCtx,
			chromedp.Navigate(sourceURL),
			chromedp.WaitReady(`body`, chromedp.ByQuery),
			chromedp.Sleep(3*time.Second),
			chromedp.Evaluate(extractGroupsJS(batchSize), &rawJSON),
		)
		if err != nil {
			return nil, fmt.Errorf("cdp navigate/extract groups %s: %w", sourceURL, err)
		}
		return parseRawItems(rawJSON)
	}

	if err := chromedp.Run(tabCtx,
		chromedp.Navigate(sourceURL),
		chromedp.WaitVisible(`[role="feed"]`, chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
	); err != nil {
		return nil, fmt.Errorf("cdp navigate %s: %w", sourceURL, err)
	}

	// Context lock: when the source is a group URL, confirm the page is still
	// inside that group before extracting. Facebook can serve home.php instead
	// of the requested group (auth/age/region gates) — those posts would be
	// off-target and pollute the lead set. Drift surfaces as a typed error;
	// the handler maps it to {"status":"aborted","reason":"context_drift"}.
	if expected := parseGroupID(sourceURL); expected != "" {
		if err := AssertInGroup(tabCtx, expected); err != nil {
			return nil, err
		}
	}

	if err := chromedp.Run(tabCtx,
		chromedp.Evaluate(extractPostsJS(batchSize), &rawJSON),
	); err != nil {
		return nil, fmt.Errorf("cdp extract %s: %w", sourceURL, err)
	}

	items, err := parseRawItems(rawJSON)
	if err != nil {
		return nil, err
	}
	log.Printf("[CDPRuntime] Extracted %d posts from %s", len(items), sourceURL)
	return items, nil
}

func parseRawItems(rawJSON string) ([]RawItem, error) {
	var raw []struct {
		ID        string `json:"id"`
		Content   string `json:"content"`
		Author    string `json:"author"`
		AuthorURL string `json:"author_url"`
		PostURL   string `json:"post_url"`
		PostFBID  string `json:"post_fbid"`
		GroupFBID string `json:"group_fbid"`
		Reactions int    `json:"reactions"`
		Comments  int    `json:"comments"`
		Shares    int    `json:"shares"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		return nil, fmt.Errorf("cdp: parse extracted JSON: %w", err)
	}

	items := make([]RawItem, 0, len(raw))
	for _, r := range raw {
		url, repairPath := canonicalSourceURL(r.PostURL, r.PostFBID, r.GroupFBID)
		items = append(items, RawItem{
			ID:               r.ID,
			Content:          r.Content,
			AuthorName:       r.Author,
			AuthorProfileURL: r.AuthorURL,
			SourceURL:        url,
			PostFBID:         strings.TrimSpace(r.PostFBID),
			GroupFBID:        strings.TrimSpace(r.GroupFBID),
			URLRepairPath:    repairPath,
			Timestamp:        time.Now().UTC(),
			Reactions:        r.Reactions,
			Comments:         r.Comments,
			Shares:           r.Shares,
		})
	}
	return items, nil
}

// URL repair telemetry signals. Surfaces in RawItem.URLRepairPath and rides
// through to leadingest.Outcome.Signals as `url:<path>` so Phase 1 telemetry
// can answer "how often did the anchor work?" / "how often did data-ft
// rescue us?" / "how often did we drop a transient feed URL?" without
// log-grepping.
const (
	URLRepairAnchorClean     = "anchor_clean"     // anchor href passed LooksLikePostURL or was a stable non-post URL
	URLRepairSynthFromFBID   = "synth_from_fbid"  // anchor missing or transient → built canonical from post_fbid
	URLRepairDroppedTransient = "dropped_transient" // nothing recoverable → empty URL; ValidateRouting will reject
)

// canonicalSourceURL turns the raw scraper output into the URL that gets
// persisted on the lead. Replaces the prior coalesce-to-page-URL fallback
// that silently leaked home.php / tracking-decorated URLs into the DB.
//
// Order matters:
//  1. A cleaned post URL that already looks like a post permalink wins.
//  2. A cleaned non-post URL that is NOT a transient feed/redirect page
//     (legitimate group shell from search results) is kept as-is.
//  3. Otherwise synthesise from FBIDs when available.
//  4. Otherwise return empty — ValidateRouting in leadingest will drop it
//     with "missing primary (post) URL", visible in classification_log.
//
// Second return value is the repair-path signal for telemetry.
func canonicalSourceURL(postURL, postFBID, groupFBID string) (string, string) {
	u := strings.TrimSpace(postURL)
	if u != "" && !isTransientFacebookURL(u) {
		return u, URLRepairAnchorClean
	}
	if id := strings.TrimSpace(postFBID); id != "" {
		if synth := fburl.CanonicalPostPermalink(strings.TrimSpace(groupFBID), id); synth != "" {
			return synth, URLRepairSynthFromFBID
		}
	}
	return "", URLRepairDroppedTransient
}

// isTransientFacebookURL reports whether a URL points at a feed / redirect
// page that cannot identify a specific post. These are the markers we now
// reject instead of silently persisting (and instead of letting the dashboard
// "Mở bài viết" button land on the home feed).
func isTransientFacebookURL(u string) bool {
	if strings.Contains(u, "/home.php") || strings.Contains(u, "/watch") {
		return true
	}
	// Bare facebook.com with no path or only a querystring.
	for _, bare := range []string{
		"https://www.facebook.com/",
		"https://facebook.com/",
		"https://m.facebook.com/",
	} {
		if u == bare || u == strings.TrimSuffix(bare, "/") {
			return true
		}
		if strings.HasPrefix(u, bare+"?") {
			return true
		}
	}
	return false
}

// extractPostsJS returns a JS snippet that scrapes up to limit posts from the
// Facebook feed that is already visible in the current tab.
//
// Three reliability rules embedded here:
//   - tracking params (__cft__, __tn__, notif_*, ref) are stripped before the
//     URL leaves the page; Facebook decorates every internal href with them.
//   - post_fbid is parsed from the anchor (/posts/, story_fbid=, fbid=) with a
//     data-ft "top_level_post_id" fallback when the permalink anchor is lazy-
//     rendered and missing.
//   - group_fbid is parsed from window.location so a home-feed escape yields
//     "" and the Go side can synthesise a canonical permalink (or reject).
func extractPostsJS(limit int) string {
	return fmt.Sprintf(`
(function() {
  // Strip Facebook tracking params so anchor.href can be persisted as-is.
  function cleanURL(raw) {
    if (!raw) return '';
    try {
      var u = new URL(raw, window.location.origin);
      var toDrop = [];
      u.searchParams.forEach(function(_v, k) {
        if (k.indexOf('__') === 0 || k === 'notif_id' || k === 'notif_t' || k === 'ref') {
          toDrop.push(k);
        }
      });
      toDrop.forEach(function(k) { u.searchParams.delete(k); });
      return u.toString();
    } catch (e) {
      return raw;
    }
  }

  // Pull the numeric post id out of any Facebook permalink form.
  function extractPostID(url) {
    if (!url) return '';
    var patterns = [/\/posts\/(\d+)/, /\/permalink\/(\d+)/, /story_fbid=(\d+)/, /[?&]fbid=(\d+)/];
    for (var i = 0; i < patterns.length; i++) {
      var m = url.match(patterns[i]);
      if (m) return m[1];
    }
    return '';
  }

  // data-ft is a JSON blob Facebook embeds on article wrappers; contains
  // top_level_post_id when the visible permalink anchor isn't rendered yet.
  function postIDFromDataFT(el) {
    var ft = el.getAttribute && el.getAttribute('data-ft');
    if (!ft) return '';
    try {
      var parsed = JSON.parse(ft);
      if (parsed && parsed.top_level_post_id) return String(parsed.top_level_post_id);
      if (parsed && parsed.mf_story_key) return String(parsed.mf_story_key);
    } catch (e) {}
    var m = ft.match(/"top_level_post_id"\s*:\s*"?(\d+)/);
    return m ? m[1] : '';
  }

  // group_fbid comes from the page URL — when the crawler drifts to home.php
  // this returns empty, signalling drift to the Go layer.
  var locMatch = window.location.pathname.match(/\/groups\/(\d+)/);
  var groupFBID = locMatch ? locMatch[1] : '';

  var posts = [];
  var articles = document.querySelectorAll('[role="feed"] > div');
  for (var i = 0; i < articles.length && posts.length < %d; i++) {
    var el = articles[i];

    // Content: largest text block inside the article
    var contentEl = el.querySelector('[data-ad-comet-preview="message"], [data-ad-preview="message"], [dir="auto"]');
    var content = contentEl ? contentEl.innerText.trim() : '';
    if (!content) continue;

    // Author: first link with aria-label or profile-style href
    var authorEl = el.querySelector('a[href*="/groups/"][aria-label], a[href*="facebook.com/"][role="link"]');
    var author = authorEl ? (authorEl.getAttribute('aria-label') || authorEl.innerText.trim()) : '';
    var authorURL = authorEl ? cleanURL(authorEl.href) : '';

    // Post permalink
    var postLinkEl = el.querySelector('a[href*="/posts/"], a[href*="story_fbid"], a[href*="/permalink/"]');
    var postURL = postLinkEl ? cleanURL(postLinkEl.href) : '';
    var postFBID = extractPostID(postURL);
    if (!postFBID) {
      // Fallback: scan article + ancestor for data-ft when anchor lazy-renders.
      var node = el;
      while (node && !postFBID) {
        postFBID = postIDFromDataFT(node);
        node = node.parentElement;
      }
    }

    // Reactions (span near "Like" button)
    var reactEl = el.querySelector('[aria-label*="reaction"], [data-testid="UFI2ReactionsCount"]');
    var reactions = reactEl ? parseInt(reactEl.innerText.replace(/[^0-9]/g, '') || '0') : 0;

    // Comments count
    var cmtEl = el.querySelector('span[aria-describedby]');
    var comments = 0;
    el.querySelectorAll('span').forEach(function(s) {
      if (/^\d+\s*(comment|bình luận)/i.test(s.innerText)) {
        comments = parseInt(s.innerText) || 0;
      }
    });

    posts.push({
      id: postURL || postFBID || ('post_' + i),
      content: content,
      author: author,
      author_url: authorURL,
      post_url: postURL,
      post_fbid: postFBID,
      group_fbid: groupFBID,
      reactions: reactions || 0,
      comments: comments || 0,
      shares: 0
    });
  }
  return JSON.stringify(posts);
})()
`, limit)
}

func extractGroupsJS(limit int) string {
	return fmt.Sprintf(`
(function() {
  var out = [];
  var anchors = Array.from(document.querySelectorAll('a[href*="/groups/"]'));
  var seen = {};
  for (var i = 0; i < anchors.length && out.length < %d; i++) {
    var a = anchors[i];
    var href = a.href || '';
    if (!href || href.indexOf('/groups/') === -1 || seen[href]) continue;
    var label = (a.innerText || a.getAttribute('aria-label') || '').trim();
    if (!label || label.length < 3) continue;
    seen[href] = true;
    var card = a.closest('[role="article"], [role="main"] div') || a.parentElement;
    var text = card ? (card.innerText || '').trim() : label;
    out.push({
      id: href,
      content: text || label,
      author: label,
      author_url: href,
      post_url: href,
      reactions: 0,
      comments: 0,
      shares: 0
    });
  }
  return JSON.stringify(out);
})()
`, limit)
}
