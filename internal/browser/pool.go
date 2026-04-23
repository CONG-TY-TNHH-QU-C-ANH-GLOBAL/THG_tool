package browser

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/chromedp"
)

// Pool manages a pool of Chrome browser contexts for concurrent scraping.
type Pool struct {
	mu           sync.Mutex
	allocCtx     context.Context
	allocCancel  context.CancelFunc
	parentCtx    context.Context // persistent anchor context (keeps Chrome alive)
	parentCancel context.CancelFunc
	contexts     chan *BrowserCtx
	maxSize      int
	chromePath   string
	proxyURL     string
	profileDir   string   // persistent profile directory
	closed       int32    // atomic: 1 = shut down
	shutdownOnce sync.Once
}

// BrowserCtx wraps a chromedp context with metadata.
type BrowserCtx struct {
	Ctx    context.Context
	Cancel context.CancelFunc
	ID     int
	InUse  bool
}

// NewPool creates a browser pool with the specified number of Chrome instances.
// If profileDir is set, Chrome uses a persistent user data directory (login sessions survive restarts).
func NewPool(size int, chromePath, proxyURL, profileDir string) (*Pool, error) {
	p := &Pool{
		contexts:   make(chan *BrowserCtx, size),
		maxSize:    size,
		chromePath: chromePath,
		proxyURL:   proxyURL,
		profileDir: profileDir,
	}

	// Determine headless mode: default=false (visible), set HEADLESS=true for headless
	headless := strings.ToLower(os.Getenv("HEADLESS")) == "true"
	if headless {
		log.Println("[BrowserPool] Running in HEADLESS mode")
	} else {
		log.Println("[BrowserPool] Running in VISIBLE mode (anti-detection)")
	}

	// Create the allocator (shared Chrome process)
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-extensions", false),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-translate", true),
		chromedp.Flag("disable-features", "Translate,TranslateUI,AutoTranslate"),
		chromedp.Flag("lang", "vi-VN"),
		chromedp.Flag("mute-audio", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("safebrowsing-disable-auto-update", true),
		// Anti-detection stealth flags
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("enable-automation", false), // CRITICAL: prevents separate session context
		chromedp.Flag("enable-features", "NetworkService,NetworkServiceInProcess"),
		chromedp.Flag("disable-infobars", true),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"),
		chromedp.WindowSize(1920, 1080),
	)

	if chromePath != "" {
		opts = append(opts, chromedp.ExecPath(chromePath))
	}

	if proxyURL != "" {
		opts = append(opts, chromedp.ProxyServer(proxyURL))
	}

	// Use persistent profile directory if specified
	if profileDir != "" {
		// Ensure directory exists
		if err := os.MkdirAll(profileDir, 0755); err != nil {
			return nil, fmt.Errorf("create profile dir: %w", err)
		}
		opts = append(opts, chromedp.UserDataDir(profileDir))
		log.Printf("[BrowserPool] Using persistent profile: %s", profileDir)
	}

	p.allocCtx, p.allocCancel = chromedp.NewExecAllocator(context.Background(), opts...)

	// Create the PARENT context (launches Chrome, keeps it alive, holds cookies)
	// This context is NOT in the pool — it's the persistent anchor
	p.parentCtx, p.parentCancel = chromedp.NewContext(p.allocCtx, chromedp.WithLogf(log.Printf))

	// Navigate to blank page to ensure Chrome starts and profile loads
	if err := chromedp.Run(p.parentCtx, chromedp.Navigate("about:blank")); err != nil {
		p.parentCancel()
		return nil, fmt.Errorf("start chrome: %w", err)
	}

	// Create CHILD contexts from parent — they SHARE the same browser context & cookies
	// Parent stays alive as anchor, children go into the pool for workers
	for i := 0; i < size; i++ {
		ctx, cancel := chromedp.NewContext(p.parentCtx)
		bCtx := &BrowserCtx{
			Ctx:    ctx,
			Cancel: cancel,
			ID:     i,
		}
		p.contexts <- bCtx
	}

	log.Printf("[BrowserPool] Initialized with %d contexts (shared session)", size)
	return p, nil
}

// ParentCtx returns the persistent parent browser context.
// Use this for login checks — it shares cookies with all pool contexts.
func (p *Pool) ParentCtx() context.Context {
	return p.parentCtx
}

// NewPoolForProfile creates a pool that uses a specific account profile.
// Each account gets its own Chrome user data directory to persist login sessions.
func NewPoolForProfile(chromePath, proxyURL string, accountID int64, baseProfileDir string) (*Pool, error) {
	profileDir := filepath.Join(baseProfileDir, fmt.Sprintf("account_%d", accountID))
	return NewPool(1, chromePath, proxyURL, profileDir)
}

// Acquire gets a browser context from the pool (blocks if none available).
// Creates a FRESH tab context each time to prevent stale contexts.
func (p *Pool) Acquire(timeout time.Duration) (*BrowserCtx, error) {
	if atomic.LoadInt32(&p.closed) == 1 {
		return nil, fmt.Errorf("browser pool is shut down")
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case ctx, ok := <-p.contexts:
		if !ok || ctx == nil {
			return nil, fmt.Errorf("browser pool is shut down")
		}
		// Cancel old context and create a fresh tab
		if ctx.Cancel != nil {
			ctx.Cancel()
		}
		newCtx, newCancel := chromedp.NewContext(p.parentCtx)
		ctx.Ctx = newCtx
		ctx.Cancel = newCancel
		ctx.InUse = true
		return ctx, nil
	case <-timer.C:
		return nil, fmt.Errorf("browser pool: acquire timeout after %v", timeout)
	}
}

// Release returns a browser context to the pool.
// Safe to call after Shutdown — just cancels the context instead of returning it.
func (p *Pool) Release(bCtx *BrowserCtx) {
	if atomic.LoadInt32(&p.closed) == 1 {
		if bCtx.Cancel != nil {
			bCtx.Cancel()
		}
		return
	}
	bCtx.InUse = false
	select {
	case p.contexts <- bCtx:
	default:
		// pool unexpectedly full — cancel instead of blocking
		if bCtx.Cancel != nil {
			bCtx.Cancel()
		}
	}
}

// Shutdown closes all browser contexts and the allocator. Safe to call multiple times.
func (p *Pool) Shutdown() {
	p.shutdownOnce.Do(func() {
		atomic.StoreInt32(&p.closed, 1)
		// Drain all available (idle) contexts
		for {
			select {
			case bCtx := <-p.contexts:
				if bCtx != nil && bCtx.Cancel != nil {
					bCtx.Cancel()
				}
			default:
				p.parentCancel()
				p.allocCancel()
				log.Println("[BrowserPool] Shut down")
				return
			}
		}
	})
}

// Size returns the current pool capacity.
func (p *Pool) Size() int {
	return p.maxSize
}

// Available returns the number of available contexts.
func (p *Pool) Available() int {
	return len(p.contexts)
}

// ProfileDir returns the profile directory being used.
func (p *Pool) ProfileDir() string {
	return p.profileDir
}
