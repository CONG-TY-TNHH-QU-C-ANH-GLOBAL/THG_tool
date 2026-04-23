package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/thg/scraper/internal/models"
)

// PostInbox navigates to the lead's Facebook profile and sends a Messenger DM.
// Safety: if the profile has pre-existing messages that we didn't initiate, it's a friend/contact — abort.
// Memory: creates/updates a ConversationThread so AI has full context for follow-ups.
func (ac *AutoCommenter) PostInbox(ctx context.Context, msg *models.OutboundMessage) error {
	account, err := ac.accountMgr.GetAccountByID(msg.AccountID)
	if err != nil {
		return fmt.Errorf("get account: %w", err)
	}

	pool, isTempPool, err := ac.acquirePool(account)
	if err != nil {
		return fmt.Errorf("get pool: %w", err)
	}
	if isTempPool {
		defer pool.Shutdown()
	}

	bCtx, err := pool.Acquire(30 * time.Second)
	if err != nil {
		return fmt.Errorf("acquire browser: %w", err)
	}
	defer pool.Release(bCtx)

	scrapeCtx, cancel := context.WithTimeout(bCtx.Ctx, 120*time.Second)
	defer cancel()

	log.Printf("[AutoInbox] 🔗 Navigating to profile: %s", msg.TargetURL)
	if err := chromedp.Run(scrapeCtx,
		chromedp.Navigate(msg.TargetURL),
		chromedp.Sleep(5*time.Second),
	); err != nil {
		return fmt.Errorf("navigate profile: %w", err)
	}

	// Kiểm tra đăng nhập
	var pageTitle string
	_ = chromedp.Run(scrapeCtx, chromedp.Title(&pageTitle))
	if strings.Contains(pageTitle, "Log in") || strings.Contains(pageTitle, "log in or sign up") {
		_ = ac.db.UpdateAccountStatus(account.ID, models.AccountInactive)
		return fmt.Errorf("account %s not logged in", account.Name)
	}

	// Bước 1: Click nút "Nhắn tin" / "Message" trên trang profile
	clickMsgJS := `
	(() => {
		const labels = ['Nhắn tin', 'Message', 'Send Message', 'Gửi tin nhắn'];
		// Tìm trong button/div có aria-label hoặc text khớp
		const candidates = [
			...document.querySelectorAll('div[role="button"], a[role="button"], button'),
		];
		for (const el of candidates) {
			const lbl = (el.getAttribute('aria-label') || '').trim();
			const txt = (el.textContent || '').trim();
			for (const label of labels) {
				if (lbl === label || txt === label) {
					el.click();
					return "CLICKED:" + label;
				}
			}
		}
		return "NOT_FOUND";
	})()`

	var clickResult string
	if err := chromedp.Run(scrapeCtx, chromedp.Evaluate(clickMsgJS, &clickResult)); err != nil {
		return fmt.Errorf("click message button: %w", err)
	}
	log.Printf("[AutoInbox] 💬 Message button: %s", clickResult)
	if !strings.HasPrefix(clickResult, "CLICKED") {
		return fmt.Errorf("message button not found on profile page")
	}
	// Chờ đủ lâu để Messenger redirect hoặc chat popup render xong
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(4*time.Second))

	// Bước 2: Log URL + SAFETY CHECK
	var currentURL string
	_ = chromedp.Run(scrapeCtx, chromedp.Location(&currentURL))
	log.Printf("[AutoInbox] 📍 URL after click: %s", currentURL)

	// SAFETY FILTER: Nếu đây KHÔNG phải conversation do hệ thống khởi tạo
	// và đã có tin nhắn cũ → đây là bạn bè / người quen → ABORT
	isOurThread := ac.db.ThreadExistsForProfile(msg.TargetURL)
	if !isOurThread {
		var existingMsgCount string
		_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(`
		(() => {
			// Đếm message bubbles có text thực sự trong conversation
			const rows = document.querySelectorAll('[role="row"]');
			let count = 0;
			for (const row of rows) {
				if (row.querySelector('div[dir="auto"]')) count++;
			}
			return String(count);
		})()`, &existingMsgCount))
		log.Printf("[AutoInbox] 🔍 Existing messages in conversation: %s", existingMsgCount)
		if existingMsgCount != "" && existingMsgCount != "0" {
			log.Printf("[AutoInbox] 🚫 SAFETY: Skipping %s — pre-existing conversation detected (%s msgs). Likely friend/contact.", msg.TargetName, existingMsgCount)
			// Mark as rejected so we never try again
			_ = ac.db.UpdateOutboundStatus(msg.ID, models.OutboundRejected)
			return fmt.Errorf("safety: pre-existing conversation with %s (%s msgs) — not a lead", msg.TargetName, existingMsgCount)
		}
	}

	// Bước 3: Poll tìm chat input — đợi tối đa ~10s (7 lần × 1.5s)
	// Messenger có thể load chậm hoặc mở trong tab mới / popup — cần poll
	const focusChatJS = `
	(() => {
		const chatSels = [
			'div[aria-label="Aa"][contenteditable="true"]',
			'div[aria-label="Message"][contenteditable="true"]',
			'div[aria-label="Tin nhắn"][contenteditable="true"]',
			'div[role="textbox"][contenteditable="true"]',
			'div[data-lexical-editor="true"]',
		];
		for (const sel of chatSels) {
			const el = document.querySelector(sel);
			if (el && el.getBoundingClientRect().height > 0) {
				el.focus(); el.click();
				return "FOCUSED:" + sel;
			}
		}
		return "NOT_FOUND";
	})()`

	var focusResult string
	for attempt := 0; attempt < 7; attempt++ {
		_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(focusChatJS, &focusResult))
		if strings.HasPrefix(focusResult, "FOCUSED") {
			break
		}
		log.Printf("[AutoInbox] ⏳ Chat input not ready (attempt %d/7)", attempt+1)
		_ = chromedp.Run(scrapeCtx, chromedp.Sleep(1500*time.Millisecond))
	}
	log.Printf("[AutoInbox] 🎯 Chat input: %s", focusResult)
	if !strings.HasPrefix(focusResult, "FOCUSED") {
		return fmt.Errorf("messenger chat input not found after 10s (URL: %s)", currentURL)
	}
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(400*time.Millisecond))

	// Bước 4: Type tin nhắn — CDP InsertText (reliable với Lexical editor của Messenger)
	log.Printf("[AutoInbox] ⌨️ Typing inbox message (%d chars)", len(msg.Content))
	if err := chromedp.Run(scrapeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.InsertText(msg.Content).Do(ctx)
		}),
		chromedp.Sleep(600*time.Millisecond),
	); err != nil {
		return fmt.Errorf("type inbox message: %w", err)
	}

	// Bước 5: Gửi — click Send button hoặc Enter
	sendJS := `
	(() => {
		const sendLabels = ['Gửi', 'Send', 'Press Enter to send'];
		for (const el of document.querySelectorAll('div[role="button"], button')) {
			const lbl = (el.getAttribute('aria-label') || el.textContent || '').trim();
			if (sendLabels.includes(lbl)) { el.click(); return "BTN_SENT"; }
		}
		return "USE_ENTER";
	})()`

	var sendResult string
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(sendJS, &sendResult))
	log.Printf("[AutoInbox] 📤 Send: %s", sendResult)

	if sendResult != "BTN_SENT" {
		if err := chromedp.Run(scrapeCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				return input.DispatchKeyEvent(input.KeyDown).
					WithKey("Enter").WithCode("Enter").
					WithNativeVirtualKeyCode(13).WithWindowsVirtualKeyCode(13).Do(ctx)
			}),
			chromedp.Sleep(100*time.Millisecond),
			chromedp.ActionFunc(func(ctx context.Context) error {
				return input.DispatchKeyEvent(input.KeyUp).
					WithKey("Enter").WithCode("Enter").
					WithNativeVirtualKeyCode(13).WithWindowsVirtualKeyCode(13).Do(ctx)
			}),
		); err != nil {
			return fmt.Errorf("send Enter: %w", err)
		}
	}

	// Bước 6: Verify — kiểm tra input đã được clear sau khi gửi
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(3*time.Second))
	var verifyResult string
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(`
	(() => {
		const sels = [
			'div[aria-label="Aa"][contenteditable="true"]',
			'div[aria-label="Message"][contenteditable="true"]',
			'div[aria-label="Tin nhắn"][contenteditable="true"]',
			'div[role="textbox"][contenteditable="true"]',
			'div[data-lexical-editor="true"]',
		];
		for (const sel of sels) {
			const el = document.querySelector(sel);
			if (el) return (el.textContent || '').trim().length === 0 ? "CLEARED" : "NOT_CLEARED";
		}
		return "NO_INPUT"; // input gone = message sent OK
	})()`, &verifyResult))
	log.Printf("[AutoInbox] 🔍 Verify: %s (msg #%d)", verifyResult, msg.ID)

	if verifyResult == "NOT_CLEARED" {
		return fmt.Errorf("inbox send verification failed: input not cleared (message may not have been sent)")
	}

	log.Printf("[AutoInbox] ✅ Inbox sent to %s: %s", msg.TargetName, truncate(msg.Content, 60))
	_ = ac.db.UpdateOutboundStatus(msg.ID, models.OutboundSent)
	if account != nil && account.ID > 0 {
		_ = ac.db.UpdateAccountLastUsed(account.ID)
	}

	// Ghi nhớ: tạo/cập nhật ConversationThread để AI có context cho lần sau
	// Thread niche is stored as "general" — AI uses BusinessProfile for domain context
	threadID, err := ac.db.CreateThread(0, string(msg.Platform), msg.TargetURL, msg.TargetName, "general")
	if err == nil && threadID > 0 {
		_ = ac.db.AddThreadMessage(threadID, "outbound", msg.Content, true)
		log.Printf("[AutoInbox] 📝 Thread #%d recorded for %s", threadID, msg.TargetName)
	}

	return nil
}

// ScrapeConversation reads the current Messenger thread and returns all visible messages.
// Used by the follow-up system to detect replies and build conversation context.
func (ac *AutoCommenter) ScrapeConversation(ctx context.Context, thread *models.ConversationThread, accountID int64) ([]models.ConversationMessage, error) {
	account, err := ac.accountMgr.GetAccountByID(accountID)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}

	pool, isTempPool, err := ac.acquirePool(account)
	if err != nil {
		return nil, fmt.Errorf("get pool: %w", err)
	}
	if isTempPool {
		defer pool.Shutdown()
	}

	bCtx, err := pool.Acquire(30 * time.Second)
	if err != nil {
		return nil, fmt.Errorf("acquire browser: %w", err)
	}
	defer pool.Release(bCtx)

	scrapeCtx, cancel := context.WithTimeout(bCtx.Ctx, 90*time.Second)
	defer cancel()

	// Navigate to profile and open chat
	if err := chromedp.Run(scrapeCtx,
		chromedp.Navigate(thread.ProfileURL),
		chromedp.Sleep(4*time.Second),
	); err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}

	// Click "Nhắn tin"
	clickJS := `
	(() => {
		const labels = ['Nhắn tin', 'Message', 'Send Message', 'Gửi tin nhắn'];
		for (const el of document.querySelectorAll('div[role="button"], a[role="button"], button')) {
			const lbl = (el.getAttribute('aria-label') || '').trim();
			const txt = (el.textContent || '').trim();
			for (const label of labels) {
				if (lbl === label || txt === label) { el.click(); return "CLICKED"; }
			}
		}
		return "NOT_FOUND";
	})()`
	var clickRes string
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(clickJS, &clickRes))
	if clickRes != "CLICKED" {
		return nil, fmt.Errorf("message button not found")
	}

	// Poll until chat loads (max 12s)
	chatLoaded := false
	for i := 0; i < 8; i++ {
		_ = chromedp.Run(scrapeCtx, chromedp.Sleep(1500*time.Millisecond))
		var check string
		_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(`
		(() => {
			return document.querySelector('[role="row"]') ? "LOADED" : "WAIT";
		})()`, &check))
		if check == "LOADED" {
			chatLoaded = true
			break
		}
	}
	if !chatLoaded {
		return nil, fmt.Errorf("conversation did not load")
	}

	// Extract messages with direction detection (position-based: left=inbound, right=outbound)
	var rawJSON string
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(`
	(() => {
		const msgs = [];
		const rows = document.querySelectorAll('[role="row"]');
		const pageCenter = window.innerWidth / 2;
		for (const row of rows) {
			const textEls = row.querySelectorAll('div[dir="auto"]');
			if (!textEls.length) continue;
			const content = Array.from(textEls)
				.map(e => e.textContent.trim())
				.filter(t => t.length > 0)
				.join(' ');
			if (!content || content.length < 2) continue;
			const rect = row.getBoundingClientRect();
			const rowCenter = rect.left + rect.width / 2;
			msgs.push({
				content: content.substring(0, 1000),
				direction: rowCenter < pageCenter ? 'inbound' : 'outbound'
			});
		}
		return JSON.stringify(msgs.slice(-30)); // most recent 30
	})()`, &rawJSON))

	type rawMsg struct {
		Content   string `json:"content"`
		Direction string `json:"direction"`
	}
	var raw []rawMsg
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil || len(raw) == 0 {
		return nil, fmt.Errorf("no messages found in conversation")
	}

	now := time.Now()
	var result []models.ConversationMessage
	for _, r := range raw {
		result = append(result, models.ConversationMessage{
			ThreadID:  thread.ID,
			Direction: r.Direction,
			Content:   r.Content,
			CreatedAt: now,
		})
	}
	return result, nil
}
