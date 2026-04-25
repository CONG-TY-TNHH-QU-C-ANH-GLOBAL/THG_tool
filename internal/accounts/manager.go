package accounts

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/thg/scraper/internal/browser"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// Manager handles multi-account Facebook management using persistent Chrome profiles.
// Each account gets its own Chrome user-data-dir, so login sessions survive restarts.
type Manager struct {
	mu             sync.RWMutex
	db             *store.Store
	chromePath     string
	baseProfileDir string // base directory for all account profiles
	current        int    // current account index for rotation
}

// NewManager creates a new account manager.
// baseProfileDir is where Chrome profiles are stored (e.g. "./data/profiles").
func NewManager(db *store.Store, chromePath, baseProfileDir string) *Manager {
	if baseProfileDir == "" {
		baseProfileDir = filepath.Join(".", "data", "profiles")
	}
	// Ensure base dir exists
	_ = os.MkdirAll(baseProfileDir, 0755)

	return &Manager{
		db:             db,
		chromePath:     chromePath,
		baseProfileDir: baseProfileDir,
	}
}

// GetProfileDir returns the Chrome profile directory for an account.
func (m *Manager) GetProfileDir(accountID int64) string {
	if accountID == 0 {
		// Use the root baseProfileDir for the default scraper session
		return m.baseProfileDir
	}
	return filepath.Join(m.baseProfileDir, fmt.Sprintf("account_%d", accountID))
}

// GetNextAccount returns the next active account in round-robin rotation.
func (m *Manager) GetNextAccount(platform models.Platform) (*models.Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	accounts, err := m.db.GetActiveAccounts(platform)
	if err != nil || len(accounts) == 0 {
		// FALLBACK: Return the default scraper session
		return &models.Account{
			ID:       0,
			Name:     "Scraper Default",
			Platform: platform,
			Status:   models.AccountActive,
		}, nil
	}

	// Round-robin
	idx := m.current % len(accounts)
	m.current++
	account := &accounts[idx]

	// Update last used
	_ = m.db.UpdateAccountLastUsed(account.ID)

	return account, nil
}

// GetPoolForAccount creates a browser pool using the account's persistent Chrome profile.
// The Chrome profile stores the login session, so no cookies needed.
func (m *Manager) GetPoolForAccount(account *models.Account) (browser.Browser, error) {
	profileDir := m.GetProfileDir(account.ID)
	proxyURL := account.ProxyURL

	return browser.NewPool(1, m.chromePath, proxyURL, profileDir)
}

// LoginInteractive opens a VISIBLE Chrome window for manual login.
// The user logs in manually, and the session is saved to the profile directory.
// After this, all headless scraping sessions will reuse the login.
func (m *Manager) LoginInteractive(account *models.Account) error {
	profileDir := m.GetProfileDir(account.ID)
	_ = os.MkdirAll(profileDir, 0755)

	log.Printf("[Accounts] Opening Chrome for manual login: %s (profile: %s)", account.Name, profileDir)

	// Launch Chrome in VISIBLE mode (headless=false) so user can log in
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false), // VISIBLE browser!
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("no-first-run", true),
		chromedp.UserDataDir(profileDir),
	)

	if m.chromePath != "" {
		opts = append(opts, chromedp.ExecPath(m.chromePath))
	}
	if account.ProxyURL != "" {
		opts = append(opts, chromedp.ProxyServer(account.ProxyURL))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// Navigate to Facebook login
	if err := chromedp.Run(ctx,
		chromedp.Navigate("https://www.facebook.com/"),
	); err != nil {
		return fmt.Errorf("navigate: %w", err)
	}

	log.Printf("[Accounts] ⚠️ Chrome đã mở! Hãy đăng nhập Facebook vào tài khoản: %s", account.Name)
	log.Printf("[Accounts] ⚠️ Sau khi đăng nhập xong, đóng cửa sổ Chrome.")

	// Wait for user to log in and close the browser (up to 5 minutes)
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer waitCancel()

	// Poll until browser closes or timeout
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			log.Printf("[Accounts] Login timeout for %s", account.Name)
			return nil // Profile is saved even on timeout
		case <-ticker.C:
			// Check if context is still valid (browser still open)
			err := chromedp.Run(ctx, chromedp.Evaluate("1", nil))
			if err != nil {
				// Browser closed — login complete!
				log.Printf("[Accounts] ✅ Login saved for %s in profile: %s", account.Name, profileDir)
				_ = m.db.UpdateAccountStatus(account.ID, models.AccountActive)
				return nil
			}
		}
	}
}

// CheckHealth verifies an account's login is still valid.
func (m *Manager) CheckHealth(account *models.Account) (models.AccountStatus, error) {
	profileDir := m.GetProfileDir(account.ID)

	// Check if profile dir exists at all
	if _, err := os.Stat(profileDir); os.IsNotExist(err) {
		return models.AccountInactive, fmt.Errorf("no profile dir for account %s", account.Name)
	}

	log.Printf("[Accounts] Health check: %s", account.Name)

	// Open headless Chrome with the persistent profile
	pool, err := browser.NewPool(1, m.chromePath, account.ProxyURL, profileDir)
	if err != nil {
		return models.AccountBanned, err
	}
	defer pool.Shutdown()

	bCtx, err := pool.Acquire(15 * time.Second)
	if err != nil {
		return models.AccountBanned, err
	}
	defer pool.Release(bCtx)

	scrapeCtx, cancel := context.WithTimeout(bCtx.Ctx, 20*time.Second)
	defer cancel()

	var pageTitle string
	if err := chromedp.Run(scrapeCtx,
		chromedp.Navigate("https://www.facebook.com/me"),
		chromedp.Sleep(3*time.Second),
		chromedp.Title(&pageTitle),
	); err != nil {
		_ = m.db.UpdateAccountStatus(account.ID, models.AccountBanned)
		return models.AccountBanned, err
	}

	// Check if actually logged in
	if pageTitle == "" || pageTitle == "Facebook" || pageTitle == "Log in to Facebook" || pageTitle == "Facebook – log in or sign up" {
		log.Printf("[Accounts] %s appears logged out", account.Name)
		_ = m.db.UpdateAccountStatus(account.ID, models.AccountInactive)
		return models.AccountInactive, nil
	}

	log.Printf("[Accounts] ✅ %s is healthy: %s", account.Name, pageTitle)
	_ = m.db.UpdateAccountStatus(account.ID, models.AccountActive)
	return models.AccountActive, nil
}

// GetAccountByID returns a specific account.
func (m *Manager) GetAccountByID(id int64) (*models.Account, error) {
	if id == 0 {
		return &models.Account{
			ID:       0,
			Name:     "Scraper Default",
			Platform: models.PlatformFacebook,
			Status:   models.AccountActive,
		}, nil
	}
	return m.db.GetAccount(id)
}
