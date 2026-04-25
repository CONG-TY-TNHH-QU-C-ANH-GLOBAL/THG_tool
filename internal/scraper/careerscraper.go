package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/thg/scraper/internal/browser"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// CareersScraper handles scraping job postings from the company careers page.
type CareersScraper struct {
	pool browser.Browser
	db   *store.Store
}

// NewCareersScraper creates a new careers scraper.
func NewCareersScraper(pool browser.Browser, db *store.Store) *CareersScraper {
	return &CareersScraper{pool: pool, db: db}
}

// CrawlCareers scrapes job postings from a careers page.
// Strategy: find actual job cards (ones with a clickable arrow), click each to open modal, extract.
func (s *CareersScraper) CrawlCareers(ctx context.Context, targetURL string) error {
	bCtx, err := s.pool.Acquire(30 * time.Second)
	if err != nil {
		return fmt.Errorf("acquire browser: %w", err)
	}
	defer s.pool.Release(bCtx)

	crawlCtx, cancel := context.WithTimeout(bCtx.Ctx, 180*time.Second)
	defer cancel()

	log.Printf("[Careers] Navigating to %s", targetURL)

	if err := chromedp.Run(crawlCtx,
		chromedp.Evaluate("Object.defineProperty(navigator, 'webdriver', { get: () => undefined });", nil),
		chromedp.Navigate(targetURL),
		chromedp.Sleep(8*time.Second),
	); err != nil {
		return fmt.Errorf("navigate: %w", err)
	}

	// Scroll to make sure all content is loaded
	_ = chromedp.Run(crawlCtx, chromedp.Evaluate("window.scrollTo(0, document.body.scrollHeight)", nil))
	_ = chromedp.Run(crawlCtx, chromedp.Sleep(2*time.Second))
	_ = chromedp.Run(crawlCtx, chromedp.Evaluate("window.scrollTo(0, 0)", nil))
	_ = chromedp.Run(crawlCtx, chromedp.Sleep(1*time.Second))

	// Step 1: Find ONLY real job cards.
	// Real job cards are identifiable because they sit inside the "Current Openings" or
	// job-listing section, and typically have a detail-trigger (arrow icon, "view" button, etc).
	// We use a two-pass approach:
	//   Pass A: Find cards that contain both an H3 AND a clickable arrow/button sibling.
	//   Pass B: If Pass A fails, find all H3s that appear AFTER a heading containing "Opening" or "Position".
	findJobsJS := `(function() {
	var results = [];
	var seen = {};

	// Pass A: Look for cards containing an H3 and a clickable arrow element.
	// These are the real job listing cards on the THG careers page.
	var allCards = document.querySelectorAll('div, article, li, section');
	for (var i = 0; i < allCards.length; i++) {
		var card = allCards[i];
		var h3 = card.querySelector('h3');
		if (!h3) continue;
		var title = (h3.innerText || '').trim();
		if (title.length < 3 || title.length > 120) continue;
		if (seen[title]) continue;

		// Check if this card has an arrow or detail trigger
		var hasArrow = false;
		var spans = card.querySelectorAll('span, div, button, a');
		for (var j = 0; j < spans.length; j++) {
			var txt = (spans[j].innerText || '').trim();
			if (txt === '\u2192' || txt === 'View' || txt === 'Details' || txt === '\u203A') {
				hasArrow = true;
				break;
			}
		}
		// Also check: card should have salary/location meta (short text lines)
		var cardText = (card.innerText || '');
		var hasMeta = (cardText.indexOf('VND') >= 0 || cardText.indexOf('Salary') >= 0 ||
			cardText.indexOf('Experience') >= 0 || cardText.indexOf('Location') >= 0 ||
			cardText.indexOf('Intern') >= 0 || cardText.indexOf('Full') >= 0);

		if (hasArrow || hasMeta) {
			// Verify it's not a nested duplicate (only pick the tightest container)
			var dominated = false;
			for (var k = 0; k < results.length; k++) {
				if (results[k] === title) { dominated = true; break; }
			}
			if (!dominated) {
				results.push(title);
				seen[title] = true;
			}
		}
	}

	// Pass B: If Pass A found nothing, find H3s appearing after "Current Openings" heading
	if (results.length === 0) {
		var allH = document.querySelectorAll('h2, h3');
		var foundSection = false;
		for (var m = 0; m < allH.length; m++) {
			var text = (allH[m].innerText || '').trim().toLowerCase();
			if (!foundSection) {
				if (text.indexOf('opening') >= 0 || text.indexOf('position') >= 0 || text.indexOf('job') >= 0) {
					foundSection = true;
				}
				continue;
			}
			// Once we're past the section header, collect H3s until a new H2 section
			if (allH[m].tagName === 'H2') break;
			var t = (allH[m].innerText || '').trim();
			if (t.length >= 5 && t.length <= 120 && !seen[t]) {
				results.push(t);
				seen[t] = true;
			}
		}
	}

	return JSON.stringify(results);
})()`

	var titlesRaw string
	if err := chromedp.Run(crawlCtx, chromedp.Evaluate(findJobsJS, &titlesRaw)); err != nil {
		return fmt.Errorf("find job cards: %w", err)
	}

	var jobTitles []string
	if err := json.Unmarshal([]byte(titlesRaw), &jobTitles); err != nil {
		return fmt.Errorf("parse job titles: %w", err)
	}

	log.Printf("[Careers] Found %d actual job cards: %v", len(jobTitles), jobTitles)

	if len(jobTitles) == 0 {
		// Last resort fallback
		var fullText string
		_ = chromedp.Run(crawlCtx, chromedp.Evaluate("(document.body.innerText || '').substring(0, 8000)", &fullText))
		if fullText == "" {
			return fmt.Errorf("không tìm thấy tin tuyển dụng nào tại: %s", targetURL)
		}
		_ = s.db.DeactivateAllCareerJobs()
		_, _ = s.db.InsertCareerJob(&models.CareerJob{
			Title: "THG Open Positions", Description: fullText,
			Location: extractLocation(fullText), Email: "careers@thgfulfill.com",
			URL: targetURL, IsActive: true,
		})
		return nil
	}

	// Step 2: Click each job title to open its modal and extract details.
	type rawJob struct {
		Title     string
		ModalText string
		Location  string
		Email     string
	}

	var extracted []rawJob
	var lastModalText string

	for i, title := range jobTitles {
		if crawlCtx.Err() != nil {
			break
		}
		log.Printf("[Careers] Clicking job %d/%d: %q", i+1, len(jobTitles), title)

		// Click the card containing this title
		clickJS := fmt.Sprintf(`(function() {
	var target = %s;
	var headings = document.querySelectorAll('h3');
	for (var i = 0; i < headings.length; i++) {
		var h = headings[i];
		if ((h.innerText || '').trim() === target) {
			var card = h.closest('div[class]');
			if (card) { card.click(); return 'CLICKED_CARD'; }
			h.click();
			return 'CLICKED_H3';
		}
	}
	return 'NOT_FOUND';
})()`, jsonQuote(title))

		var clickResult string
		if err := chromedp.Run(crawlCtx, chromedp.Evaluate(clickJS, &clickResult)); err != nil {
			log.Printf("[Careers] Click failed for %q: %v", title, err)
			continue
		}
		if clickResult == "NOT_FOUND" {
			log.Printf("[Careers] Element not found for %q", title)
			continue
		}

		// Wait for modal to appear
		_ = chromedp.Run(crawlCtx, chromedp.Sleep(2*time.Second))

		// Extract modal content — try multiple selectors
		modalExtractJS := `(function() {
	var selectors = [
		'div[class*="fixed"][class*="inset"]',
		'[role="dialog"]',
		'[aria-modal="true"]',
		'div[class*="modal"]',
		'div[class*="overlay"]'
	];
	for (var s = 0; s < selectors.length; s++) {
		var els = document.querySelectorAll(selectors[s]);
		for (var e = 0; e < els.length; e++) {
			var el = els[e];
			if (el.offsetHeight > 200 && el.offsetWidth > 200) {
				var text = (el.innerText || '').trim();
				if (text.length > 100) return text.substring(0, 5000);
			}
		}
	}
	return '';
})()`

		var modalText string
		if err := chromedp.Run(crawlCtx, chromedp.Evaluate(modalExtractJS, &modalText)); err != nil {
			log.Printf("[Careers] Modal extract failed for %q: %v", title, err)
		}

		// Check: is modal actually new content, or is it stale from the previous job?
		if len(modalText) > 100 && modalText != lastModalText {
			extracted = append(extracted, rawJob{
				Title:     title,
				ModalText: modalText,
				Location:  extractLocation(modalText),
				Email:     extractEmail(modalText),
			})
			lastModalText = modalText
			log.Printf("[Careers] Extracted %d chars for %q", len(modalText), title)
		} else if modalText == lastModalText && len(modalText) > 100 {
			log.Printf("[Careers] Skipping %q — same modal content as previous (stale)", title)
		} else {
			log.Printf("[Careers] Modal was empty/short for %q (got %d chars)", title, len(modalText))
		}

		// Close modal: press Escape, then click outside
		_ = chromedp.Run(crawlCtx,
			chromedp.KeyEvent("\x1b"),
			chromedp.Sleep(500*time.Millisecond),
		)
		// Also try clicking overlay/backdrop to definitively close
		closeJS := `(function() {
	var backdrops = document.querySelectorAll('div[class*="fixed"][class*="inset"]');
	for (var i = 0; i < backdrops.length; i++) {
		var bd = backdrops[i];
		if (bd.offsetHeight > 0 && bd.children.length <= 2) {
			bd.click();
			return 'CLOSED';
		}
	}
	return 'NO_BACKDROP';
})()`
		_ = chromedp.Run(crawlCtx, chromedp.Evaluate(closeJS, nil))
		_ = chromedp.Run(crawlCtx, chromedp.Sleep(800*time.Millisecond))
	}

	if len(extracted) == 0 {
		return fmt.Errorf("không trích xuất được tin tuyển dụng nào (đã thử %d jobs)", len(jobTitles))
	}

	// Step 3: Save to database
	_ = s.db.DeactivateAllCareerJobs()

	saved := 0
	for _, j := range extracted {
		if j.Email == "" {
			j.Email = "careers@thgfulfill.com"
		}
		dbJob := &models.CareerJob{
			Title:       j.Title,
			Description: j.ModalText,
			Location:    j.Location,
			Email:       j.Email,
			URL:         targetURL,
			IsActive:    true,
		}
		if _, err := s.db.InsertCareerJob(dbJob); err == nil {
			saved++
		}
	}

	log.Printf("[Careers] Saved %d actual job positions to database", saved)
	return nil
}

// CrawlCareersImages opens each job modal on the careers page, screenshots it,
// and saves the image as a CompanyImage with category="career_job".
// This allows the HR agent to attach a visual JD card when commenting on candidates.
func (s *CareersScraper) CrawlCareersImages(ctx context.Context, targetURL string) (int, error) {
	bCtx, err := s.pool.Acquire(30 * time.Second)
	if err != nil {
		return 0, fmt.Errorf("acquire browser: %w", err)
	}
	defer s.pool.Release(bCtx)

	crawlCtx, cancel := context.WithTimeout(bCtx.Ctx, 300*time.Second)
	defer cancel()

	imgDir := filepath.Join("data", "images", "careers")
	if err := os.MkdirAll(imgDir, 0755); err != nil {
		return 0, fmt.Errorf("create careers image dir: %w", err)
	}

	log.Printf("[CareersImg] Navigating to %s", targetURL)
	if err := chromedp.Run(crawlCtx,
		chromedp.Evaluate("Object.defineProperty(navigator, 'webdriver', { get: () => undefined });", nil),
		chromedp.Navigate(targetURL),
		chromedp.Sleep(8*time.Second),
		chromedp.Evaluate("window.scrollTo(0, document.body.scrollHeight)", nil),
		chromedp.Sleep(2*time.Second),
		chromedp.Evaluate("window.scrollTo(0, 0)", nil),
		chromedp.Sleep(1*time.Second),
	); err != nil {
		return 0, fmt.Errorf("navigate: %w", err)
	}

	// Reuse same JS to discover job titles
	findJobsJS := `(function() {
	var results = [];
	var seen = {};
	var allCards = document.querySelectorAll('div, article, li, section');
	for (var i = 0; i < allCards.length; i++) {
		var card = allCards[i];
		var h3 = card.querySelector('h3');
		if (!h3) continue;
		var title = (h3.innerText || '').trim();
		if (title.length < 3 || title.length > 120 || seen[title]) continue;
		var hasArrow = false;
		for (var j = 0, spans = card.querySelectorAll('span,div,button,a'); j < spans.length; j++) {
			var txt = (spans[j].innerText || '').trim();
			if (txt === '\u2192' || txt === 'View' || txt === 'Details' || txt === '\u203A') { hasArrow = true; break; }
		}
		var cardText = card.innerText || '';
		var hasMeta = cardText.indexOf('VND') >= 0 || cardText.indexOf('Salary') >= 0 ||
			cardText.indexOf('Experience') >= 0 || cardText.indexOf('Location') >= 0 ||
			cardText.indexOf('Intern') >= 0 || cardText.indexOf('Full') >= 0;
		if (hasArrow || hasMeta) { results.push(title); seen[title] = true; }
	}
	if (results.length === 0) {
		var allH = document.querySelectorAll('h2, h3'), foundSection = false;
		for (var m = 0; m < allH.length; m++) {
			var text = (allH[m].innerText || '').trim().toLowerCase();
			if (!foundSection) {
				if (text.indexOf('opening') >= 0 || text.indexOf('position') >= 0 || text.indexOf('job') >= 0) foundSection = true;
				continue;
			}
			if (allH[m].tagName === 'H2') break;
			var t = (allH[m].innerText || '').trim();
			if (t.length >= 5 && t.length <= 120 && !seen[t]) { results.push(t); seen[t] = true; }
		}
	}
	return JSON.stringify(results);
})()`

	var titlesRaw string
	if err := chromedp.Run(crawlCtx, chromedp.Evaluate(findJobsJS, &titlesRaw)); err != nil {
		return 0, fmt.Errorf("find job cards: %w", err)
	}
	var jobTitles []string
	if err := json.Unmarshal([]byte(titlesRaw), &jobTitles); err != nil {
		return 0, fmt.Errorf("parse job titles: %w", err)
	}
	log.Printf("[CareersImg] Found %d job cards to screenshot: %v", len(jobTitles), jobTitles)

	saved := 0
	for i, title := range jobTitles {
		if crawlCtx.Err() != nil {
			break
		}

		// Click to open modal
		clickJS := fmt.Sprintf(`(function() {
	var target = %s;
	var headings = document.querySelectorAll('h3');
	for (var i = 0; i < headings.length; i++) {
		if ((headings[i].innerText || '').trim() === target) {
			var card = headings[i].closest('div[class]');
			if (card) { card.click(); return 'CARD'; }
			headings[i].click(); return 'H3';
		}
	}
	return 'NOT_FOUND';
})()`, jsonQuote(title))

		var clickRes string
		if err := chromedp.Run(crawlCtx, chromedp.Evaluate(clickJS, &clickRes)); err != nil || clickRes == "NOT_FOUND" {
			log.Printf("[CareersImg] Skip %q — not found", title)
			continue
		}
		_ = chromedp.Run(crawlCtx, chromedp.Sleep(2500*time.Millisecond))

		// Find modal element and screenshot it
		var modalClip []byte
		screenshotJS := `(function() {
	var sels = ['div[class*="fixed"][class*="inset"]','[role="dialog"]','[aria-modal="true"]','div[class*="modal"]'];
	for (var s = 0; s < sels.length; s++) {
		var els = document.querySelectorAll(sels[s]);
		for (var e = 0; e < els.length; e++) {
			if (els[e].offsetHeight > 200 && els[e].offsetWidth > 200) {
				var r = els[e].getBoundingClientRect();
				return JSON.stringify({x:Math.round(r.x), y:Math.round(r.y), w:Math.round(r.width), h:Math.round(r.height)});
			}
		}
	}
	return '';
})()`
		var clipRaw string
		_ = chromedp.Run(crawlCtx, chromedp.Evaluate(screenshotJS, &clipRaw))

		if clipRaw != "" {
			var clip struct{ X, Y, W, H int }
			if json.Unmarshal([]byte(clipRaw), &clip) == nil && clip.W > 100 && clip.H > 100 {
				_ = chromedp.Run(crawlCtx, chromedp.CaptureScreenshot(&modalClip))
			}
		}
		if len(modalClip) == 0 {
			// Full page screenshot fallback
			_ = chromedp.Run(crawlCtx, chromedp.CaptureScreenshot(&modalClip))
		}

		if len(modalClip) == 0 {
			log.Printf("[CareersImg] Screenshot empty for %q — skip", title)
			_ = chromedp.Run(crawlCtx, chromedp.KeyEvent("\x1b"), chromedp.Sleep(500*time.Millisecond))
			continue
		}

		// Save screenshot to disk
		safeTitle := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
				return r
			}
			if r == ' ' {
				return '_'
			}
			return -1
		}, title)
		if safeTitle == "" {
			safeTitle = fmt.Sprintf("job_%d", i+1)
		}
		imgPath := filepath.Join(imgDir, fmt.Sprintf("%s_%d.png", safeTitle, time.Now().Unix()))
		if err := os.WriteFile(imgPath, modalClip, 0644); err != nil {
			log.Printf("[CareersImg] WriteFile failed for %q: %v", title, err)
		} else {
			compImg := &models.CompanyImage{
				TelegramFileID: fmt.Sprintf("career_%d_%d", time.Now().Unix(), i),
				LocalPath:      imgPath,
				Description:    title,
				Category:       "career_job",
				SourceURL:      targetURL,
			}
			if _, err := s.db.InsertCompanyImage(compImg); err == nil {
				saved++
				log.Printf("[CareersImg] ✅ Saved screenshot for %q → %s", title, filepath.Base(imgPath))
			}
		}

		// Close modal
		_ = chromedp.Run(crawlCtx,
			chromedp.KeyEvent("\x1b"),
			chromedp.Sleep(800*time.Millisecond),
		)
	}

	return saved, nil
}

// jsonQuote returns a JSON-quoted string safe for embedding in JavaScript.
func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// extractLocation infers job location from text keywords.
func extractLocation(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "hồ chí minh") || strings.Contains(lower, "hcm") || strings.Contains(lower, "tp.hcm"):
		return "Hồ Chí Minh"
	case strings.Contains(lower, "hà nội") || strings.Contains(lower, " hn "):
		return "Hà Nội"
	case strings.Contains(lower, "california") || strings.Contains(lower, "usa") || strings.Contains(lower, " us "):
		return "US"
	case strings.Contains(lower, "remote") || strings.Contains(lower, "từ xa"):
		return "Remote"
	default:
		return ""
	}
}

// extractEmail finds the first email address in text.
func extractEmail(text string) string {
	for i, c := range text {
		if c == '@' {
			start := i
			for start > 0 && isEmailChar(rune(text[start-1])) {
				start--
			}
			end := i + 1
			for end < len(text) && isEmailChar(rune(text[end])) {
				end++
			}
			if start < i && end > i+1 {
				return text[start:end]
			}
		}
	}
	return "careers@thgfulfill.com"
}

func isEmailChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
}
