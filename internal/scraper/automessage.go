package scraper

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/thg/scraper/internal/accounts"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// AutoMessenger handles sending inbox messages via Facebook Messenger.
type AutoMessenger struct {
	db         *store.Store
	accountMgr *accounts.Manager
}

// NewAutoMessenger creates a new auto-messenger.
func NewAutoMessenger(db *store.Store, accountMgr *accounts.Manager) *AutoMessenger {
	return &AutoMessenger{db: db, accountMgr: accountMgr}
}

// SendMessage navigates to a Messenger conversation and sends a message.
func (am *AutoMessenger) SendMessage(ctx context.Context, msg *models.OutboundMessage) error {
	if msg.AccountID == 0 {
		return fmt.Errorf("account_id required for auto-inbox")
	}

	account, err := am.accountMgr.GetAccountByID(msg.AccountID)
	if err != nil {
		return fmt.Errorf("get account: %w", err)
	}

	// Get browser pool using account's persistent Chrome profile
	pool, err := am.accountMgr.GetPoolForAccount(account)
	if err != nil {
		return fmt.Errorf("get pool: %w", err)
	}
	defer pool.Shutdown()

	bCtx, err := pool.Acquire(30 * time.Second)
	if err != nil {
		return fmt.Errorf("acquire browser: %w", err)
	}
	defer pool.Release(bCtx)

	scrapeCtx, cancel := context.WithTimeout(bCtx.Ctx, 60*time.Second)
	defer cancel()

	log.Printf("[AutoInbox] Opening Messenger for: %s", msg.TargetName)

	// Navigate to the target's Messenger conversation
	targetURL := msg.TargetURL
	if targetURL == "" {
		return fmt.Errorf("target_url required for Messenger")
	}

	// If it's a profile URL, convert to Messenger URL
	// facebook.com/profile → messenger.com/t/profile
	if err := chromedp.Run(scrapeCtx,
		chromedp.Navigate(targetURL),
		chromedp.Sleep(4*time.Second),
	); err != nil {
		return fmt.Errorf("navigate: %w", err)
	}

	// Check login
	var pageTitle string
	_ = chromedp.Run(scrapeCtx, chromedp.Title(&pageTitle))
	if pageTitle == "Log in to Facebook" || pageTitle == "Facebook – log in or sign up" {
		_ = am.db.UpdateAccountStatus(account.ID, models.AccountInactive)
		return fmt.Errorf("account %s not logged in", account.Name)
	}

	// Find message input and type
	messageJS := fmt.Sprintf(`
	(() => {
		const selectors = [
			'div[aria-label="Message"]',
			'div[aria-label="Tin nhắn"]',
			'div[aria-label="Aa"]', 
			'div[role="textbox"][contenteditable="true"]',
			'p[class*="xat24cr"]',
		];
		
		let input = null;
		for (const sel of selectors) {
			const els = document.querySelectorAll(sel);
			if (els.length > 0) { input = els[0]; break; }
		}
		
		if (!input) return "NO_INPUT";
		
		input.focus();
		input.click();
		input.textContent = %q;
		input.dispatchEvent(new Event('input', { bubbles: true }));
		
		return "READY";
	})()
	`, msg.Content)

	var result string
	if err := chromedp.Run(scrapeCtx, chromedp.Evaluate(messageJS, &result)); err != nil {
		return fmt.Errorf("find message input: %w", err)
	}

	if result == "NO_INPUT" {
		return fmt.Errorf("could not find message input box")
	}

	// Wait a moment, then press Enter to send
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(1*time.Second))

	submitJS := `
	(() => {
		const selectors = [
			'div[aria-label="Message"]',
			'div[aria-label="Tin nhắn"]',
			'div[role="textbox"][contenteditable="true"]',
		];
		let input = null;
		for (const sel of selectors) {
			const els = document.querySelectorAll(sel);
			if (els.length > 0) { input = els[0]; break; }
		}
		if (input) {
			const enterEvent = new KeyboardEvent('keydown', {
				key: 'Enter', code: 'Enter', keyCode: 13, which: 13, bubbles: true
			});
			input.dispatchEvent(enterEvent);
			return "SENT";
		}
		return "FAILED";
	})()
	`

	var submitResult string
	if err := chromedp.Run(scrapeCtx, chromedp.Evaluate(submitJS, &submitResult)); err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	// Wait for send
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(3*time.Second))

	log.Printf("[AutoInbox] ✅ Message sent to %s: %s", msg.TargetName, truncate(msg.Content, 50))

	// Update message status
	_ = am.db.UpdateOutboundStatus(msg.ID, models.OutboundSent)
	_ = am.db.UpdateAccountLastUsed(account.ID)

	return nil
}
