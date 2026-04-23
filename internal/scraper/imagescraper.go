package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/thg/scraper/internal/browser"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// ImageScraper crawls images from a website URL and saves them to the database.
type ImageScraper struct {
	pool   *browser.Pool
	db     *store.Store
	client *http.Client
}

// NewImageScraper creates a new image scraper.
func NewImageScraper(pool *browser.Pool, db *store.Store) *ImageScraper {
	return &ImageScraper{
		pool: pool,
		db:   db,
		client: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}
}

// catalogProduct holds rich metadata extracted from the catalog page.
type catalogProduct struct {
	Name     string `json:"name"`
	Price    string `json:"price"`
	SKU      string `json:"sku"`
	Origin   string `json:"origin"`   // "VN", "US", "CN"
	ImageURL string `json:"imageUrl"` // empty if placeholder
}

// CrawlCatalog visits a catalog URL, paginates through ALL pages,
// extracts product metadata (name, price, SKU, origin), downloads ONLY real images
// (skips placeholder/SVG icons), and saves them with rich descriptions for AI matching.
// Returns the number of images saved.
func (is *ImageScraper) CrawlCatalog(ctx context.Context, pageURL string) (int, error) {
	if is.pool == nil {
		return 0, fmt.Errorf("browser pool not initialized")
	}

	bCtx, err := is.pool.Acquire(30 * time.Second)
	if err != nil {
		return 0, fmt.Errorf("acquire browser: %w", err)
	}
	defer is.pool.Release(bCtx)

	// Longer timeout for multi-page crawl (303 products ÷ ~20 per page = ~16 pages)
	scrapeCtx, cancel := context.WithTimeout(bCtx.Ctx, 5*time.Minute)
	defer cancel()

	log.Printf("[ImageScraper] Navigating to catalog: %s", pageURL)

	if err := chromedp.Run(scrapeCtx,
		chromedp.Navigate(pageURL),
		chromedp.Sleep(5*time.Second),
	); err != nil {
		return 0, fmt.Errorf("navigate: %w", err)
	}

	// Prepare images directory
	imgDir := filepath.Join("data", "images")
	if err := os.MkdirAll(imgDir, 0755); err != nil {
		return 0, fmt.Errorf("create images dir: %w", err)
	}

	totalSaved := 0
	pageNum := 0
	maxPages := 30 // safety cap

	for pageNum < maxPages {
		pageNum++

		// Scroll to trigger lazy-loaded images
		scrollJS := `
		(async () => {
			const total = document.body.scrollHeight;
			const steps = 6;
			for (let i = 1; i <= steps; i++) {
				window.scrollTo(0, (total / steps) * i);
				await new Promise(r => setTimeout(r, 400));
			}
			window.scrollTo(0, 0);
			await new Promise(r => setTimeout(r, 300));
		})()`
		_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(scrollJS, nil))
		_ = chromedp.Run(scrapeCtx, chromedp.Sleep(2*time.Second))

		// Extract products from current page with rich metadata
		// Strategy: find all product card containers, for each extract:
		// - img src (skip if no <img> = placeholder)
		// - h3 text (product name)
		// - price text
		// - SKU text
		// - origin emoji (🇻🇳=VN, 🇺🇸=US, 🇨🇳=CN)
		extractJS := `
		JSON.stringify((() => {
			// Find product cards — look for repeating card-like containers with images/titles
			const cards = document.querySelectorAll('[class*="product-card"], [class*="productCard"], [class*="card"]');
			if (cards.length === 0) return [];
			
			return Array.from(cards).map(card => {
				// Image: only real <img> tags, skip SVG placeholders
				const img = card.querySelector('img[src]:not([src*="data:"]):not([src*="svg"])');
				const imageUrl = img ? (img.src || img.getAttribute('data-src') || '') : '';
				
				// Skip if image looks like a placeholder or logo
				if (imageUrl && (imageUrl.includes('placeholder') || imageUrl.includes('logo'))) {
					return {name:'', price:'', sku:'', origin:'', imageUrl:''};
				}
				
				// Product name from h3 or first heading
				const h3 = card.querySelector('h3, h4, [class*="title"], [class*="name"]');
				const name = h3 ? h3.textContent.trim() : '';
				
				// Price
				const priceEl = card.querySelector('[class*="price"], [class*="Price"]');
				const price = priceEl ? priceEl.textContent.trim() : '';
				
				// SKU
				const skuEl = card.querySelector('[class*="sku"], [class*="SKU"], [class*="Sku"]');
				let sku = skuEl ? skuEl.textContent.trim() : '';
				if (sku.startsWith('SKU:')) sku = sku.substring(4).trim();
				
				// Origin from emoji flags or text
				let origin = '';
				const text = card.textContent;
				if (text.includes('🇻🇳') || text.includes('VN')) origin = 'VN';
				else if (text.includes('🇺🇸') || text.includes('US')) origin = 'US';
				else if (text.includes('🇨🇳') || text.includes('CN')) origin = 'CN';
				
				return {name, price, sku, origin, imageUrl};
			}).filter(p => p.name && p.imageUrl); // Only products with name AND real image
		})())`

		var rawJSON string
		if err := chromedp.Run(scrapeCtx, chromedp.Evaluate(extractJS, &rawJSON)); err != nil {
			log.Printf("[ImageScraper] Page %d extract error: %v", pageNum, err)
			break
		}

		var products []catalogProduct
		if err := json.Unmarshal([]byte(rawJSON), &products); err != nil {
			log.Printf("[ImageScraper] Page %d parse error: %v", pageNum, err)
			break
		}

		log.Printf("[ImageScraper] Page %d: found %d products with real images", pageNum, len(products))

		// Download and save each product image with rich metadata
		for i, prod := range products {
			if prod.ImageURL == "" || prod.Name == "" {
				continue
			}

			absURL := resolveURL(pageURL, prod.ImageURL)
			if absURL == "" {
				continue
			}

			localPath, err := is.downloadImage(absURL, imgDir, totalSaved+i)
			if err != nil {
				log.Printf("[ImageScraper] Skip %s: %v", prod.Name, err)
				continue
			}

			// Build rich description for AI matching
			// Format: "ProductName | $Price | Origin: VN/US/CN"
			desc := prod.Name
			if prod.Price != "" {
				desc += " | " + prod.Price
			}
			if prod.Origin != "" {
				desc += " | Origin: " + prod.Origin
			}
			if prod.SKU != "" {
				desc += " | SKU: " + prod.SKU
			}

			// Category = origin for easy filtering
			category := "catalog"
			if prod.Origin != "" {
				category = "catalog-" + strings.ToLower(prod.Origin)
			}

			compImg := &models.CompanyImage{
				TelegramFileID: fmt.Sprintf("catalog_%d_%d", time.Now().Unix(), totalSaved+i),
				LocalPath:      localPath,
				Description:    desc,
				Category:       category,
				SourceURL:      pageURL,
			}
			if _, err := is.db.InsertCompanyImage(compImg); err != nil {
				log.Printf("[ImageScraper] DB save error: %v", err)
				continue
			}
			totalSaved++
			log.Printf("[ImageScraper] ✅ [%s] %s → %s", prod.Origin, prod.Name, filepath.Base(localPath))
		}

		// Try to click "Next" page button
		nextPageJS := `
		(() => {
			// Find next/chevron button
			const btns = document.querySelectorAll('button, a, [role="button"]');
			for (const btn of btns) {
				const text = btn.textContent.trim();
				const ariaLabel = (btn.getAttribute('aria-label') || '').toLowerCase();
				// Look for next arrow (›, >, chevron) or "Next" text
				if (text === '›' || text === '>' || text === '→' || text === '»' ||
				    ariaLabel.includes('next') || ariaLabel.includes('sau') ||
				    btn.querySelector('svg[class*="chevron"], svg[class*="arrow"], svg[class*="next"]')) {
					// Check it's not disabled
					if (btn.disabled || btn.classList.contains('disabled') || btn.getAttribute('aria-disabled') === 'true') {
						return "LAST_PAGE";
					}
					btn.click();
					return "CLICKED";
				}
			}
			return "NOT_FOUND";
		})()`

		var nextResult string
		_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(nextPageJS, &nextResult))
		log.Printf("[ImageScraper] Pagination: %s (after page %d)", nextResult, pageNum)

		if nextResult != "CLICKED" {
			log.Printf("[ImageScraper] ✅ Reached last page (%d)", pageNum)
			break
		}

		// Wait for next page to load
		_ = chromedp.Run(scrapeCtx, chromedp.Sleep(3*time.Second))
	}

	log.Printf("[ImageScraper] 🏁 Catalog crawl complete: %d images saved across %d pages", totalSaved, pageNum)
	return totalSaved, nil
}

func (is *ImageScraper) downloadImage(imgURL, dir string, idx int) (string, error) {
	req, err := http.NewRequest("GET", imgURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", imgURL)

	resp, err := is.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Check content type
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "image/") {
		return "", fmt.Errorf("not an image: %s", ct)
	}

	// Determine extension
	ext := ".jpg"
	if strings.Contains(ct, "png") {
		ext = ".png"
	} else if strings.Contains(ct, "webp") {
		ext = ".webp"
	} else if strings.Contains(ct, "gif") {
		ext = ".gif"
	}

	filename := fmt.Sprintf("catalog_%d_%d%s", time.Now().UnixMilli(), idx, ext)
	localPath := filepath.Join(dir, filename)

	f, err := os.Create(localPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	n, err := io.Copy(f, resp.Body)
	if err != nil {
		os.Remove(localPath)
		return "", err
	}

	// Skip files too small to be real product images (< 1KB)
	if n < 1000 {
		os.Remove(localPath)
		return "", fmt.Errorf("too small (%d bytes)", n)
	}

	return localPath, nil
}

func resolveURL(base, ref string) string {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	b, err := url.Parse(base)
	if err != nil {
		return ""
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	return b.ResolveReference(r).String()
}
