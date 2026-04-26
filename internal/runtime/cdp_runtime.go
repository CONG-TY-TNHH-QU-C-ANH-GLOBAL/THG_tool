package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// Ensure json is used for unmarshaling extracted post data.

// CDPRuntime connects to an existing Chrome instance via CDP and scrapes
// Facebook group posts without disturbing the user's active session.
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

// FetchBatch opens a new background tab in the attached Chrome, navigates to
// sourceURL, and extracts posts from the Facebook group feed via JS injection.
// Stateless single-batch mode: returns nil when offset > 0.
func (r *CDPRuntime) FetchBatch(ctx context.Context, sourceURL string, offset, batchSize int) ([]RawItem, error) {
	if offset > 0 {
		return nil, nil
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, r.wsURL)
	defer allocCancel()

	tabCtx, tabCancel := chromedp.NewContext(allocCtx)
	defer tabCancel()

	tabCtx, timeoutCancel := context.WithTimeout(tabCtx, 60*time.Second)
	defer timeoutCancel()

	var rawJSON string
	err := chromedp.Run(tabCtx,
		chromedp.Navigate(sourceURL),
		chromedp.WaitVisible(`[role="feed"]`, chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
		chromedp.Evaluate(extractPostsJS(batchSize), &rawJSON),
	)
	if err != nil {
		return nil, fmt.Errorf("cdp navigate/extract %s: %w", sourceURL, err)
	}

	var raw []struct {
		ID        string `json:"id"`
		Content   string `json:"content"`
		Author    string `json:"author"`
		AuthorURL string `json:"author_url"`
		PostURL   string `json:"post_url"`
		Reactions int    `json:"reactions"`
		Comments  int    `json:"comments"`
		Shares    int    `json:"shares"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		return nil, fmt.Errorf("cdp: parse extracted JSON: %w", err)
	}

	items := make([]RawItem, 0, len(raw))
	for _, r := range raw {
		items = append(items, RawItem{
			ID:               r.ID,
			Content:          r.Content,
			AuthorName:       r.Author,
			AuthorProfileURL: r.AuthorURL,
			SourceURL:        coalesce(r.PostURL, sourceURL),
			Timestamp:        time.Now().UTC(),
			Reactions:        r.Reactions,
			Comments:         r.Comments,
			Shares:           r.Shares,
		})
	}
	log.Printf("[CDPRuntime] Extracted %d posts from %s", len(items), sourceURL)
	return items, nil
}

func coalesce(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// extractPostsJS returns a JS snippet that scrapes up to limit posts from the
// Facebook feed that is already visible in the current tab.
func extractPostsJS(limit int) string {
	return fmt.Sprintf(`
(function() {
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
    var authorURL = authorEl ? authorEl.href : '';

    // Post permalink
    var postLinkEl = el.querySelector('a[href*="/posts/"], a[href*="story_fbid"], a[href*="/permalink/"]');
    var postURL = postLinkEl ? postLinkEl.href : '';

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
      id: postURL || ('post_' + i),
      content: content,
      author: author,
      author_url: authorURL,
      post_url: postURL,
      reactions: reactions || 0,
      comments: comments || 0,
      shares: 0
    });
  }
  return JSON.stringify(posts);
})()
`, limit)
}
