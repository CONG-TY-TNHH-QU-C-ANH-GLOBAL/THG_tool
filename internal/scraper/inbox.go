package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/thg/scraper/internal/accounts"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// InboxScraper handles scraping Facebook Messenger inbox.
type InboxScraper struct {
	db         *store.Store
	accountMgr *accounts.Manager
}

// NewInboxScraper creates a new inbox scraper.
func NewInboxScraper(db *store.Store, accountMgr *accounts.Manager) *InboxScraper {
	return &InboxScraper{db: db, accountMgr: accountMgr}
}

// ScrapeInbox checks the inbox for new messages using the specified account's Chrome profile.
func (s *InboxScraper) ScrapeInbox(ctx context.Context, account *models.Account) ([]models.InboxMessage, error) {
	if account == nil {
		return nil, fmt.Errorf("account required for inbox scraping")
	}

	// Get a browser pool using the account's persistent Chrome profile
	pool, err := s.accountMgr.GetPoolForAccount(account)
	if err != nil {
		return nil, fmt.Errorf("get pool for account: %w", err)
	}
	defer pool.Shutdown()

	bCtx, err := pool.Acquire(30 * time.Second)
	if err != nil {
		return nil, fmt.Errorf("acquire browser: %w", err)
	}
	defer pool.Release(bCtx)

	scrapeCtx, cancel := context.WithTimeout(bCtx.Ctx, 60*time.Second)
	defer cancel()

	log.Printf("[Inbox] Navigating to Messenger for account: %s", account.Name)

	// Navigate to Facebook Messenger (session loaded from Chrome profile)
	if err := chromedp.Run(scrapeCtx,
		chromedp.Navigate("https://www.facebook.com/messages/t/"),
		chromedp.Sleep(5*time.Second),
	); err != nil {
		return nil, fmt.Errorf("navigate to inbox: %w", err)
	}

	// Check if we're actually logged in
	var pageTitle string
	_ = chromedp.Run(scrapeCtx, chromedp.Title(&pageTitle))
	if pageTitle == "Log in to Facebook" || pageTitle == "Facebook" || pageTitle == "Facebook – log in or sign up" {
		log.Printf("[Inbox] Account %s not logged in, marking inactive", account.Name)
		_ = s.db.UpdateAccountStatus(account.ID, models.AccountInactive)
		return nil, fmt.Errorf("account %s not authenticated — run login first", account.Name)
	}

	// Wait for conversations to load
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(3*time.Second))

	// Extract conversations
	extractJS := `
	(() => {
		const messages = [];
		const convItems = document.querySelectorAll('a[href*="/messages/t/"]');
		for (const item of convItems) {
			try {
				const nameEl = item.querySelector('span[dir="auto"]');
				if (!nameEl) continue;
				const sender = nameEl.textContent.trim();
				if (!sender) continue;
				const spans = item.querySelectorAll('span');
				let preview = '';
				for (const span of spans) {
					const text = span.textContent.trim();
					if (text.length > 10 && text !== sender) {
						preview = text;
						break;
					}
				}
				const url = item.href || '';
				const isUnread = item.querySelector('[data-visualcompletion="ignore"]') !== null;
				messages.push({ sender, senderURL: url, content: preview.substring(0, 500), isUnread });
			} catch(e) { continue; }
		}
		return JSON.stringify(messages.slice(0, 20));
	})()
	`

	var resultJSON string
	if err := chromedp.Run(scrapeCtx, chromedp.Evaluate(extractJS, &resultJSON)); err != nil {
		return nil, fmt.Errorf("extract inbox: %w", err)
	}

	type rawMessage struct {
		Sender    string `json:"sender"`
		SenderURL string `json:"senderURL"`
		Content   string `json:"content"`
		IsUnread  bool   `json:"isUnread"`
	}

	var rawMessages []rawMessage
	if err := json.Unmarshal([]byte(resultJSON), &rawMessages); err != nil {
		return nil, fmt.Errorf("parse inbox JSON: %w", err)
	}

	now := time.Now()
	var messages []models.InboxMessage
	for _, rm := range rawMessages {
		if rm.Content == "" {
			continue
		}
		msg := models.InboxMessage{
			Platform:   models.PlatformFacebook,
			Sender:     rm.Sender,
			SenderURL:  rm.SenderURL,
			Content:    rm.Content,
			IsRead:     !rm.IsUnread,
			ReceivedAt: now,
		}
		if id, err := s.db.InsertInboxMessage(&msg); err == nil {
			msg.ID = id
			messages = append(messages, msg)
		}
	}

	log.Printf("[Inbox] Extracted %d messages for account %s", len(messages), account.Name)
	return messages, nil
}
