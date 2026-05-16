package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// StatefulSession holds an open Chrome tab across multiple FetchNext calls.
// Used by the handler for long-running scrape jobs to avoid reopening a tab
// per batch. Caller must call Close() when done (defer is safe).
type StatefulSession struct {
	accountID   int64
	cdpPort     int
	allocCtx    context.Context
	allocCancel context.CancelFunc
	tabCtx      context.Context
	tabCancel   context.CancelFunc
	tabTTL      *time.Timer
	scroller    *HumanScroller
	cache       PaginationCache
	seen        map[string]bool
	emptyRuns   int
	fp          Fingerprint
}

const tabTTL = 5 * time.Minute
const maxEmptyRuns = 3

// NewStatefulSession attaches to the visible page in the Chrome instance at
// cdpPort. The context is released when Close() is called or the 5-minute TTL
// fires.
func NewStatefulSession(ctx context.Context, cdpPort int, accountID int64, cache PaginationCache) (*StatefulSession, error) {
	wsURL, err := chromeWSURL(cdpPort)
	if err != nil {
		return nil, Wrap(ErrChromeUnreachable, fmt.Sprintf("get ws url for port %d", cdpPort), err)
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, wsURL)
	targetID, targetErr := visiblePageTargetID(allocCtx)
	var tabCtx context.Context
	var tabCancel context.CancelFunc
	if targetErr == nil {
		tabCtx, tabCancel = chromedp.NewContext(allocCtx, chromedp.WithTargetID(targetID))
	} else {
		tabCtx, tabCancel = chromedp.NewContext(allocCtx)
	}

	s := &StatefulSession{
		accountID:   accountID,
		cdpPort:     cdpPort,
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		tabCtx:      tabCtx,
		tabCancel:   tabCancel,
		scroller:    NewHumanScroller(accountID),
		cache:       cache,
		seen:        make(map[string]bool),
		fp:          DefaultFingerprint(accountID),
	}

	// Hard TTL: cancel the tab context after tabTTL regardless of what's happening.
	s.tabTTL = time.AfterFunc(tabTTL, func() {
		tabCancel()
	})

	return s, nil
}

// Navigate goes to the URL and injects the fingerprint. Must be called before FetchNext.
func (s *StatefulSession) Navigate(url string) error {
	// Inject fingerprint before navigation (runs on every new document)
	script := BuildInjectionScript(s.fp)
	if err := chromedp.Run(s.tabCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(script).Do(ctx)
			return err
		}),
	); err != nil {
		// Non-fatal: fingerprint injection failure doesn't block scraping
		_ = err
	}

	if err := chromedp.Run(s.tabCtx, chromedp.Navigate(url)); err != nil {
		return Wrap(ErrNavigationTimeout, "navigate to "+url, err)
	}

	// Check for ban signals after navigation
	if err := checkBanSignals(s.tabCtx); err != nil {
		return err
	}

	// Wait for the feed
	if err := chromedp.Run(s.tabCtx,
		chromedp.WaitVisible(`[role="feed"]`, chromedp.ByQuery),
	); err != nil {
		return Wrap(ErrNavigationTimeout, "wait for feed", err)
	}

	return nil
}

// identityJS extracts the numeric FB user ID from the logged-in page.
// Tries three extraction points in order of reliability.
const identityJS = "(function(){" +
	"var el=document.querySelector('[data-userid]');" +
	"if(el)return el.getAttribute('data-userid');" +
	"var s=document.querySelectorAll('script'),m,i;" +
	"for(i=0;i<s.length;i++){m=s[i].innerText.match(/\"USER_ID\":\"(\\d+)\"/);if(m)return m[1];}" +
	"m=document.documentElement.innerHTML.match(/\"actorID\":\"(\\d+)\"/);if(m)return m[1];" +
	"return '';})()"

// VerifyIdentity asserts the currently-logged-in Facebook user matches
// expectedFBUserID. Call once after Navigate().
//
// Invariant SESSION_ISOLATION: if the session belongs to a different user,
// return ErrSessionContaminated — do NOT execute the job.
// If expectedFBUserID is empty the check is skipped (account not enrolled yet).
func (s *StatefulSession) VerifyIdentity(expectedFBUserID string) error {
	if expectedFBUserID == "" {
		return nil
	}
	var currentUID string
	if err := chromedp.Run(s.tabCtx, chromedp.Evaluate(identityJS, &currentUID)); err != nil {
		return nil // cannot read — skip rather than false-positive
	}
	if currentUID == "" {
		return CDPError{Code: ErrFacebookLogout, Message: "could not read logged-in user from DOM"}
	}
	if currentUID != expectedFBUserID {
		return CDPError{
			Code:    ErrSessionContaminated,
			Message: fmt.Sprintf("session contamination: want %s got %s", expectedFBUserID, currentUID),
		}
	}
	return nil
}

// FetchNext extracts up to batchSize new posts from the current scroll position.
// Returns nil, nil when the page is exhausted (maxEmptyRuns consecutive empty scrolls).
func (s *StatefulSession) FetchNext(batchSize int) ([]RawItem, error) {
	select {
	case <-s.tabCtx.Done():
		return nil, CDPError{Code: ErrTabTTLExceeded, Message: "tab context cancelled (TTL or manual close)"}
	default:
	}

	var rawJSON string
	if err := chromedp.Run(s.tabCtx,
		chromedp.Evaluate(extractPostsJS(batchSize*2), &rawJSON), // fetch 2× and filter seen
	); err != nil {
		return nil, Wrap(ErrContentExtraction, "evaluate js extractor", err)
	}

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
	}
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		return nil, Wrap(ErrContentExtraction, "parse js output", err)
	}

	var items []RawItem
	for _, r := range raw {
		key := r.ID
		if key == "" {
			key = r.PostURL
		}
		if key == "" || s.seen[key] {
			continue
		}
		if s.cache != nil && s.cache.Seen(r.PostURL, key) {
			continue
		}
		s.seen[key] = true
		if s.cache != nil {
			s.cache.Mark(r.PostURL, key)
		}
		url, repairPath := canonicalSourceURL(r.PostURL, r.PostFBID, r.GroupFBID)
		items = append(items, RawItem{
			ID:               key,
			Content:          r.Content,
			AuthorName:       r.Author,
			AuthorProfileURL: r.AuthorURL,
			SourceURL:        url,
			PostFBID:         r.PostFBID,
			GroupFBID:        r.GroupFBID,
			URLRepairPath:    repairPath,
			Timestamp:        time.Now().UTC(),
			Reactions:        r.Reactions,
			Comments:         r.Comments,
		})
		if len(items) >= batchSize {
			break
		}
	}

	if len(items) == 0 {
		s.emptyRuns++
		if s.emptyRuns >= maxEmptyRuns {
			return nil, nil // exhausted
		}
	} else {
		s.emptyRuns = 0
	}

	// Scroll down for next call
	if err := s.scroller.ScrollOnce(s.tabCtx, 640, 400); err != nil {
		return items, nil // scroll error is non-fatal; return what we have
	}

	return items, nil
}

// Close releases the tab and its allocator context. Safe to call multiple times.
func (s *StatefulSession) Close() {
	if s.tabTTL != nil {
		s.tabTTL.Stop()
	}
	if s.tabCancel != nil {
		s.tabCancel()
	}
	if s.allocCancel != nil {
		s.allocCancel()
	}
}
