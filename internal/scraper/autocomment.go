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

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/thg/scraper/internal/accounts"
	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/browser"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// AutoCommenter handles posting comments on Facebook posts.
type AutoCommenter struct {
	db         *store.Store
	accountMgr *accounts.Manager
	mainPool   browser.Browser
	selectorAI *ai.SelectorAI
}

// NewAutoCommenter creates a new auto-commenter.
func NewAutoCommenter(db *store.Store, accountMgr *accounts.Manager, mainPool browser.Browser) *AutoCommenter {
	return &AutoCommenter{db: db, accountMgr: accountMgr, mainPool: mainPool}
}

// SetSelectorAI injects the AI-driven selector discovery engine.
func (ac *AutoCommenter) SetSelectorAI(s *ai.SelectorAI) {
	ac.selectorAI = s
}

// Execute posts a comment, reply, or group post, attaching an image if msg.ImagePath is set.
func (ac *AutoCommenter) Execute(ctx context.Context, msg *models.OutboundMessage) error {
	switch msg.Type {
	case "comment_reply":
		return ac.PostCommentReply(ctx, msg)
	case "group_post":
		return ac.PostToGroup(ctx, msg)
	default:
		if msg.ImagePath != "" {
			if _, err := os.Stat(msg.ImagePath); err == nil {
				return ac.PostCommentWithImage(ctx, msg)
			}
			log.Printf("[AutoComment] ⚠️ Image path not found, posting text only: %s", msg.ImagePath)
		}
		return ac.PostComment(ctx, msg)
	}
}

// PostToGroup navigates to a Facebook group and creates a new post.
// Uses the same www.facebook.com + native chromedp approach as PostCommentWithImage.
// msg.TargetURL = group URL, msg.Content = post text, msg.ImagePath = optional image.
func (ac *AutoCommenter) PostToGroup(ctx context.Context, msg *models.OutboundMessage) error {
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

	log.Printf("[AutoComment] 📝 Posting JD to group: %s", msg.TargetURL)

	// 1. Navigate to group page
	if err := chromedp.Run(scrapeCtx,
		chromedp.Navigate(msg.TargetURL),
		chromedp.Sleep(5*time.Second),
	); err != nil {
		return fmt.Errorf("navigate: %w", err)
	}

	var pageTitle string
	_ = chromedp.Run(scrapeCtx, chromedp.Title(&pageTitle))
	log.Printf("[AutoComment] Page title: %q", pageTitle)
	if strings.Contains(pageTitle, "Log in") || strings.Contains(pageTitle, "log in") {
		_ = ac.db.UpdateAccountStatus(account.ID, models.AccountInactive)
		return fmt.Errorf("account %s not logged in", account.Name)
	}

	// 2. Click the compose trigger to open the post dialog
	// Facebook's compose area: user avatar + "Bạn viết gì đi..." / "Write something..."
	// The text might be in a <span> deeply nested, not in a role="button" directly
	findComposeJS := `(function() {
		window.scrollTo(0, 0);
		var keywords = ["viết", "write", "nghĩ", "mind", "gì đó", "gì đi", "what's on"];
		// Strategy 1: search ALL elements for compose text
		var allEls = document.querySelectorAll('span, div, p');
		for (var i = 0; i < allEls.length; i++) {
			var el = allEls[i];
			// Get DIRECT text (not children's text)
			var directText = "";
			for (var c = 0; c < el.childNodes.length; c++) {
				if (el.childNodes[c].nodeType === 3) directText += el.childNodes[c].textContent;
			}
			directText = directText.toLowerCase().trim();
			if (directText.length < 3 || directText.length > 60) continue;
			var matched = false;
			for (var k = 0; k < keywords.length; k++) {
				if (directText.indexOf(keywords[k]) >= 0) { matched = true; break; }
			}
			if (!matched) continue;
			// Found compose text! Walk up to find clickable parent
			var target = el;
			for (var p = 0; p < 8; p++) {
				if (!target.parentElement) break;
				target = target.parentElement;
				var role = target.getAttribute("role") || "";
				var tabidx = target.getAttribute("tabindex");
				if (role === "button" || tabidx === "0" || tabidx === "-1") break;
			}
			var rect = target.getBoundingClientRect();
			if (rect.height > 0 && rect.width > 50) {
				return JSON.stringify({found: true, x: Math.round(rect.left + rect.width/2), y: Math.round(rect.top + rect.height/2), text: directText.substring(0, 40), tag: target.tagName});
			}
		}
		// Strategy 2: look for the pill-shaped compose input (wide, short, near top)
		var btns = document.querySelectorAll('div[role="button"], div[tabindex="0"]');
		for (var j = 0; j < btns.length; j++) {
			var r = btns[j].getBoundingClientRect();
			if (r.top > 700 || r.height <= 0) continue;
			// Compose trigger: wide (>250px), pill-shaped (height 30-50px)
			if (r.width > 250 && r.height >= 25 && r.height <= 55) {
				return JSON.stringify({found: true, x: Math.round(r.left + r.width/2), y: Math.round(r.top + r.height/2), text: "pill_btn_" + Math.round(r.top), tag: btns[j].tagName});
			}
		}
		// Debug dump: show everything visible
		var dump = [];
		var all2 = document.querySelectorAll('div[role="button"], div[tabindex="0"], span');
		for (var d = 0; d < all2.length && dump.length < 20; d++) {
			var dr = all2[d].getBoundingClientRect();
			if (dr.top > 800 || dr.height <= 0 || dr.width < 30) continue;
			var txt = (all2[d].textContent || "").trim().substring(0, 40);
			if (txt.length === 0) continue;
			dump.push(all2[d].tagName + "[" + Math.round(dr.top) + "," + Math.round(dr.width) + "x" + Math.round(dr.height) + "]:" + txt);
		}
		return JSON.stringify({found: false, dump: dump});
	})()`

	var composeInfo string
	_ = chromedp.Run(scrapeCtx,
		chromedp.Evaluate(`window.scrollTo(0, 0)`, nil),
		chromedp.Sleep(2*time.Second),
		chromedp.Evaluate(findComposeJS, &composeInfo),
	)
	log.Printf("[AutoComment] Compose info: %s", composeInfo)

	// Parse the compose location
	type composeResult struct {
		Found   bool     `json:"found"`
		X       int      `json:"x"`
		Y       int      `json:"y"`
		Text    string   `json:"text"`
		Buttons []string `json:"buttons"`
	}
	var cr composeResult
	if err := json.Unmarshal([]byte(composeInfo), &cr); err != nil || !cr.Found {
		return fmt.Errorf("could not find compose area: %s", composeInfo)
	}

	// Native mouse click at the compose trigger coordinates (triggers React events!)
	log.Printf("[AutoComment] Clicking compose at (%d, %d): %q", cr.X, cr.Y, cr.Text)
	if err := chromedp.Run(scrapeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(input.MousePressed, float64(cr.X), float64(cr.Y)).
				WithButton(input.Left).WithClickCount(1).Do(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(input.MouseReleased, float64(cr.X), float64(cr.Y)).
				WithButton(input.Left).WithClickCount(1).Do(ctx)
		}),
		chromedp.Sleep(3*time.Second),
	); err != nil {
		return fmt.Errorf("native click compose: %w", err)
	}

	// 3. Wait for dialog contenteditable to appear
	// Facebook may NOT use role="dialog" — search broadly for the post composer input
	var dialogFound string
	checkDialogJS := `(function() {
		// Strategy 1: find contenteditable inside role="dialog"
		var dialog = document.querySelector('div[role="dialog"]');
		if (dialog) {
			var ce = dialog.querySelector('div[contenteditable="true"]');
			if (ce) { ce.focus(); ce.click(); return "ready_dialog_ce"; }
			var tb = dialog.querySelector('div[role="textbox"]');
			if (tb) { tb.focus(); tb.click(); return "ready_dialog_tb"; }
		}
		// Strategy 2: find any visible contenteditable that appeared after click
		var allCE = document.querySelectorAll('div[contenteditable="true"]');
		for (var i = 0; i < allCE.length; i++) {
			var r = allCE[i].getBoundingClientRect();
			if (r.height > 30 && r.width > 200 && r.top > 50 && r.top < 600) {
				allCE[i].focus(); allCE[i].click();
				return "ready_global_ce_" + Math.round(r.top);
			}
		}
		// Strategy 3: find role="textbox" anywhere
		var allTB = document.querySelectorAll('div[role="textbox"]');
		for (var j = 0; j < allTB.length; j++) {
			var rt = allTB[j].getBoundingClientRect();
			if (rt.height > 20 && rt.width > 200 && rt.top > 50 && rt.top < 600) {
				allTB[j].focus(); allTB[j].click();
				return "ready_global_tb_" + Math.round(rt.top);
			}
		}
		// Strategy 4: find by aria-label or placeholder containing "bài viết"
		var labeled = document.querySelectorAll('[aria-label*="bài viết"], [aria-label*="public post"], [aria-placeholder*="bài viết"], [data-placeholder*="bài viết"]');
		for (var k = 0; k < labeled.length; k++) {
			var rl = labeled[k].getBoundingClientRect();
			if (rl.height > 10 && rl.width > 100) {
				labeled[k].focus(); labeled[k].click();
				return "ready_labeled:" + (labeled[k].getAttribute("aria-label") || labeled[k].getAttribute("aria-placeholder") || "").substring(0, 30);
			}
		}
		// Debug: dump what's on the page
		var info = "dialog=" + (dialog ? "yes" : "no");
		var ces = document.querySelectorAll('div[contenteditable="true"]');
		info += ",ce_count=" + ces.length;
		for (var d = 0; d < ces.length; d++) {
			var dr = ces[d].getBoundingClientRect();
			info += ",ce[" + d + "]=" + Math.round(dr.top) + "," + Math.round(dr.width) + "x" + Math.round(dr.height);
		}
		var tbs = document.querySelectorAll('div[role="textbox"]');
		info += ",tb_count=" + tbs.length;
		return "not_found|" + info;
	})()`
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(checkDialogJS, &dialogFound))
	log.Printf("[AutoComment] Dialog check: %s", dialogFound)

	if !strings.HasPrefix(dialogFound, "ready") {
		// Retry — dialog animation may need more time
		_ = chromedp.Run(scrapeCtx, chromedp.Sleep(3*time.Second))
		_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(checkDialogJS, &dialogFound))
		log.Printf("[AutoComment] Dialog check retry: %s", dialogFound)
		if !strings.HasPrefix(dialogFound, "ready") {
			return fmt.Errorf("dialog not ready after compose click: %s", dialogFound)
		}
	}

	// 4. Type content using ClipboardEvent paste (same as comment flow — React compatible)
	// Find the input broadly — same approach as dialog detection
	typeJS := fmt.Sprintf(`(function() {
		// Find the active post composer input
		var ce = null;
		// Try dialog first
		var dialog = document.querySelector('div[role="dialog"]');
		if (dialog) {
			ce = dialog.querySelector('div[contenteditable="true"]') || dialog.querySelector('div[role="textbox"]');
		}
		// Fallback: any visible contenteditable/textbox on page
		if (!ce) {
			var all = document.querySelectorAll('div[contenteditable="true"], div[role="textbox"]');
			for (var i = 0; i < all.length; i++) {
				var r = all[i].getBoundingClientRect();
				if (r.height > 20 && r.width > 200 && r.top > 50 && r.top < 600) { ce = all[i]; break; }
			}
		}
		if (!ce) return "no_input";
		ce.focus();
		try {
			var dt = new DataTransfer();
			dt.setData('text/plain', %s);
			ce.dispatchEvent(new ClipboardEvent('paste', {clipboardData: dt, bubbles: true, cancelable: true}));
		} catch(e) { return "error:" + e.message; }
		var txt = (ce.textContent || ce.innerText || "").trim();
		return txt.length > 0 ? "typed:" + txt.length : "empty_after_paste";
	})()`, jsonQuote(msg.Content))

	var typeResult string
	_ = chromedp.Run(scrapeCtx,
		chromedp.Evaluate(typeJS, &typeResult),
		chromedp.Sleep(1*time.Second),
	)
	log.Printf("[AutoComment] Type result: %s", typeResult)

	// Fallback: CDP InsertText if paste didn't work
	if !strings.HasPrefix(typeResult, "typed:") {
		log.Printf("[AutoComment] Paste failed, using CDP InsertText...")
		_ = chromedp.Run(scrapeCtx,
			chromedp.Evaluate(`(function(){
				var ce = null;
				var dialog = document.querySelector('div[role="dialog"]');
				if (dialog) ce = dialog.querySelector('div[contenteditable="true"]') || dialog.querySelector('div[role="textbox"]');
				if (!ce) {
					var all = document.querySelectorAll('div[contenteditable="true"], div[role="textbox"]');
					for (var i = 0; i < all.length; i++) {
						var r = all[i].getBoundingClientRect();
						if (r.height > 20 && r.width > 200 && r.top > 50 && r.top < 600) { ce = all[i]; break; }
					}
				}
				if(ce){ce.focus();ce.click();document.execCommand('selectAll');document.execCommand('delete');}
				return ce?"focused":"no_input";
			})()`, nil),
			chromedp.Sleep(300*time.Millisecond),
			chromedp.ActionFunc(func(ctx context.Context) error {
				return input.InsertText(msg.Content).Do(ctx)
			}),
			chromedp.Sleep(500*time.Millisecond),
		)
	}

	// 5. Attach image if available
	if msg.ImagePath != "" {
		if _, statErr := os.Stat(msg.ImagePath); statErr == nil {
			absPath, _ := filepath.Abs(msg.ImagePath)
			// Click photo/video button in dialog to reveal file input
			_ = chromedp.Run(scrapeCtx,
				chromedp.Evaluate(`(function() {
					var dialog = document.querySelector('div[role="dialog"]');
					if (!dialog) return "no_dialog";
					var btns = dialog.querySelectorAll('div[role="button"], i, svg, img');
					var triggers = ["photo", "ảnh", "video", "hình"];
					for (var i = 0; i < btns.length; i++) {
						var lbl = (btns[i].getAttribute("aria-label") || btns[i].textContent || "").toLowerCase();
						for (var t = 0; t < triggers.length; t++) {
							if (lbl.indexOf(triggers[t]) >= 0) {
								btns[i].click();
								return "clicked:" + lbl.substring(0, 20);
							}
						}
					}
					return "no_photo_btn";
				})()`, nil),
				chromedp.Sleep(2*time.Second),
			)
			// Upload file
			if err := chromedp.Run(scrapeCtx,
				chromedp.SetUploadFiles(`div[role="dialog"] input[type="file"]`, []string{absPath}),
				chromedp.Sleep(3*time.Second),
			); err != nil {
				log.Printf("[AutoComment] Image upload failed (continuing without): %v", err)
			} else {
				log.Printf("[AutoComment] 🖼️ Attached image: %s", msg.ImagePath)
			}
		}
	}

	// 6. Scroll dialog to bottom and click Post/Đăng button
	// First scroll the dialog container so the submit button is visible
	_ = chromedp.Run(scrapeCtx,
		chromedp.Evaluate(`(function(){
			var dialog = document.querySelector('div[role="dialog"]');
			if(dialog) dialog.scrollTop = dialog.scrollHeight;
			// Also scroll any scrollable child
			var scrollables = document.querySelectorAll('div[style*="overflow"]');
			for(var i=0;i<scrollables.length;i++){scrollables[i].scrollTop=scrollables[i].scrollHeight;}
		})()`, nil),
		chromedp.Sleep(1*time.Second),
	)

	// Search for the submit button — Facebook may use ANY element type
	submitJS := `(function() {
		var triggers = ["đăng", "post"];
		// Search ALL visible elements whose trimmed text is exactly a trigger word
		var all = document.querySelectorAll('*');
		var candidates = [];
		for (var i = 0; i < all.length; i++) {
			var el = all[i];
			// Get direct text only (skip containers with lots of child text)
			var directText = "";
			for (var c = 0; c < el.childNodes.length; c++) {
				if (el.childNodes[c].nodeType === 3) directText += el.childNodes[c].textContent;
			}
			directText = directText.toLowerCase().trim();
			if (directText.length === 0) continue;
			var matched = false;
			for (var t = 0; t < triggers.length; t++) {
				if (directText === triggers[t]) { matched = true; break; }
			}
			if (!matched) continue;
			var rect = el.getBoundingClientRect();
			if (rect.height <= 0 || rect.width < 30) continue;
			candidates.push({el: el, rect: rect, text: directText, tag: el.tagName});
		}
		if (candidates.length === 0) {
			// Debug: search aria-label
			var labeled = document.querySelectorAll('[aria-label*="Đăng"], [aria-label*="Post"], [aria-label*="đăng"]');
			for (var j = 0; j < labeled.length; j++) {
				var lr = labeled[j].getBoundingClientRect();
				if (lr.height > 0 && lr.width > 30) {
					return JSON.stringify({found: true, x: Math.round(lr.left + lr.width/2), y: Math.round(lr.top + lr.height/2), text: "labeled:" + labeled[j].getAttribute("aria-label")});
				}
			}
			return JSON.stringify({found: false, candidateCount: 0});
		}
		// Pick the largest/most prominent candidate (likely the blue button)
		candidates.sort(function(a,b) { return (b.rect.width * b.rect.height) - (a.rect.width * a.rect.height); });
		var best = candidates[0];
		return JSON.stringify({found: true, x: Math.round(best.rect.left + best.rect.width/2), y: Math.round(best.rect.top + best.rect.height/2), text: best.text, tag: best.tag, count: candidates.length});
	})()`

	var submitInfo string
	if err := chromedp.Run(scrapeCtx,
		chromedp.Evaluate(submitJS, &submitInfo),
	); err != nil {
		return fmt.Errorf("submit post: %w", err)
	}
	log.Printf("[AutoComment] Submit info: %s", submitInfo)

	type submitResult struct {
		Found bool   `json:"found"`
		X     int    `json:"x"`
		Y     int    `json:"y"`
		Text  string `json:"text"`
	}
	var sr submitResult
	if err := json.Unmarshal([]byte(submitInfo), &sr); err != nil || !sr.Found {
		return fmt.Errorf("could not find submit button: %s", submitInfo)
	}

	// Native mouse click on submit button (triggers React)
	log.Printf("[AutoComment] Clicking submit at (%d, %d): %q", sr.X, sr.Y, sr.Text)
	if err := chromedp.Run(scrapeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(input.MousePressed, float64(sr.X), float64(sr.Y)).
				WithButton(input.Left).WithClickCount(1).Do(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(input.MouseReleased, float64(sr.X), float64(sr.Y)).
				WithButton(input.Left).WithClickCount(1).Do(ctx)
		}),
		chromedp.Sleep(8*time.Second),
	); err != nil {
		return fmt.Errorf("click submit: %w", err)
	}

	log.Printf("[AutoComment] ✅ Posted JD to group: %s | Content: %s", msg.TargetURL, truncate(msg.Content, 80))
	return nil
}

// PostCommentReply navigates to a post, finds the target commenter's comment,
// and posts a reply to it. msg.TargetURL is the post URL, msg.TargetName is the
// commenter's display name used to locate the correct comment node.
func (ac *AutoCommenter) PostCommentReply(ctx context.Context, msg *models.OutboundMessage) error {
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

	scrapeCtx, cancel := context.WithTimeout(bCtx.Ctx, 90*time.Second)
	defer cancel()

	postID := extractPostID(msg.TargetURL)
	log.Printf("[AutoComment] 💬 Reply to comment by %q on post %s", msg.TargetName, msg.TargetURL)

	if err := chromedp.Run(scrapeCtx,
		chromedp.Navigate(msg.TargetURL),
		chromedp.Sleep(5*time.Second),
	); err != nil {
		return fmt.Errorf("navigate: %w", err)
	}

	var pageTitle string
	_ = chromedp.Run(scrapeCtx, chromedp.Title(&pageTitle))
	if pageTitle == "Log in to Facebook" || pageTitle == "Facebook – log in or sign up" {
		_ = ac.db.UpdateAccountStatus(account.ID, models.AccountInactive)
		return fmt.Errorf("account %s not logged in", account.Name)
	}

	// Expand "View more comments" up to 3 times so we can find the target commenter
	expandMoreJS := `
	(() => {
		let clicked = 0;
		const sels = [
			'div[role="button"][aria-expanded="false"]',
			'span[role="button"]',
		];
		for (const el of document.querySelectorAll('div[role="button"], span[role="button"]')) {
			const t = el.textContent.toLowerCase();
			if ((t.includes('more comment') || t.includes('xem thêm bình luận') || t.includes('view') && t.includes('comment')) && !el.closest('[role="dialog"]')) {
				el.click(); clicked++;
				if (clicked >= 2) break;
			}
		}
		return "expanded:" + clicked;
	})()`
	for range 3 {
		var expRes string
		_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(expandMoreJS, &expRes))
		log.Printf("[AutoComment] 📖 %s", expRes)
		_ = chromedp.Run(scrapeCtx, chromedp.Sleep(2*time.Second))
	}

	// Find the target commenter's comment and click its Reply button
	replyClickJS := fmt.Sprintf(`
	(() => {
		const targetName = %q;
		// Walk all comment articles/items looking for author match
		const articles = Array.from(document.querySelectorAll('[role="article"], [data-visualcompletion="ignore-dynamic"]'));
		for (const art of articles) {
			const links = Array.from(art.querySelectorAll('a[href*="facebook.com"]'));
			const nameMatch = links.some(a => a.textContent.trim() === targetName);
			if (!nameMatch) continue;
			// Found the right comment — click its Reply button
			const replyBtns = Array.from(art.querySelectorAll('div[role="button"], span[role="button"]'));
			for (const btn of replyBtns) {
				const t = btn.textContent.toLowerCase().trim();
				if (t === 'reply' || t === 'trả lời' || t === 'phản hồi') {
					btn.click();
					return "REPLY_CLICKED:" + targetName;
				}
			}
			return "NO_REPLY_BTN:" + targetName;
		}
		return "COMMENT_NOT_FOUND:" + targetName;
	})()`, msg.TargetName)

	var replyResult string
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(replyClickJS, &replyResult))
	log.Printf("[AutoComment] 🖱️ Reply click: %s", replyResult)
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(2*time.Second))

	if strings.HasPrefix(replyResult, "COMMENT_NOT_FOUND") || strings.HasPrefix(replyResult, "NO_REPLY_BTN") {
		// Fallback: post as a top-level comment on the post instead
		log.Printf("[AutoComment] ⚠️ Could not find comment by %q — posting as top-level comment", msg.TargetName)
		return ac.PostComment(ctx, msg)
	}

	// Type the reply text (the reply input box is now active)
	if err := ac.typeCommentText(scrapeCtx, postID, msg.Content); err != nil {
		return fmt.Errorf("type reply: %w", err)
	}

	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(1*time.Second))
	if err := ac.submitComment(scrapeCtx, postID); err != nil {
		return err
	}

	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(7*time.Second))
	if err := ac.verifySubmitted(scrapeCtx, postID, msg.ID); err != nil {
		return err
	}

	log.Printf("[AutoComment] ✅ Reply posted to %s on %s", msg.TargetName, msg.TargetURL)
	_ = ac.db.UpdateOutboundStatus(msg.ID, models.OutboundSent)
	if account != nil && account.ID > 0 {
		_ = ac.db.UpdateAccountLastUsed(account.ID)
	}
	return nil
}

// acquirePool returns the browser to use and whether it is temporary (must be shut down after use).
func (ac *AutoCommenter) acquirePool(account *models.Account) (browser.Browser, bool, error) {
	if account.ID == 0 && ac.mainPool != nil {
		return ac.mainPool, false, nil
	}
	pool, err := ac.accountMgr.GetPoolForAccount(account)
	return pool, true, err
}

// articleScopeJS returns a JS snippet that defines `scope` as the [role="article"]
// element containing a link to postID. Falls back to `document` if not found.
// Must be embedded at the top of each (() => { ... })() block.
func articleScopeJS(postID string) string {
	return fmt.Sprintf(`
    const _postID = %q;
    let scope = document;
    if (_postID) {
        for (const a of document.querySelectorAll('[role="article"]')) {
            if (a.querySelector('a[href*="' + _postID + '"]') || a.innerHTML.includes(_postID)) {
                scope = a; break;
            }
        }
    }`, postID)
}

// findInputJS returns a JS snippet that resolves to the comment textbox within scope.
// If nothing is found in scope, falls back to the topmost visible textbox on the page.
const findInputJS = `
    const _sels = [
        'div[aria-label="Write a comment"]',
        'div[aria-label="Viết bình luận"]',
        'div[aria-label="Write a public comment..."]',
        'div[aria-label="Viết bình luận công khai..."]',
        'div[aria-label*="bình luận"][role="textbox"]',
        'div[aria-label*="comment"][role="textbox"]',
        'div[contenteditable="true"][role="textbox"]',
    ];
    let _inp = null;
    for (const sel of _sels) {
        _inp = scope.querySelector(sel);
        if (_inp) break;
    }
    if (!_inp) {
        // Fallback: topmost visible textbox on entire page
        const all = Array.from(document.querySelectorAll('div[contenteditable="true"][role="textbox"]'));
        all.sort((a, b) => a.getBoundingClientRect().top - b.getBoundingClientRect().top);
        _inp = all.find(e => e.getBoundingClientRect().height > 0) || all[0] || null;
    }`

// PostCommentWithImage posts a comment with an image attachment on a Facebook post.
func (ac *AutoCommenter) PostCommentWithImage(ctx context.Context, msg *models.OutboundMessage) error {
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

	scrapeCtx, cancel := context.WithTimeout(bCtx.Ctx, 90*time.Second)
	defer cancel()

	postID := extractPostID(msg.TargetURL)
	log.Printf("[AutoComment] 🔗 Navigating (with image) to post %s: %s", postID, msg.TargetURL)

	if err := chromedp.Run(scrapeCtx,
		chromedp.Navigate(msg.TargetURL),
		chromedp.Sleep(5*time.Second),
	); err != nil {
		return fmt.Errorf("navigate: %w", err)
	}

	var pageTitle string
	_ = chromedp.Run(scrapeCtx, chromedp.Title(&pageTitle))
	if pageTitle == "Log in to Facebook" || pageTitle == "Facebook – log in or sign up" {
		_ = ac.db.UpdateAccountStatus(account.ID, models.AccountInactive)
		return fmt.Errorf("account %s not logged in", account.Name)
	}

	var currentURL string
	_ = chromedp.Run(scrapeCtx, chromedp.Location(&currentURL))
	if postID != "" && !strings.Contains(currentURL, postID) {
		return fmt.Errorf("redirected to wrong page (expected post %s, got %s)", postID, currentURL)
	}
	log.Printf("[AutoComment] ✅ URL verified: %s", currentURL)

	// Step 1: Expand the comment area of the TARGET post
	expandJS := fmt.Sprintf(`
	(() => {
		%s
		for (const sel of ['div[aria-label="Leave a comment"]', 'div[aria-label="Bình luận"]', 'span[data-sigil="addcomment"]']) {
			const el = scope.querySelector(sel);
			if (el) { el.click(); return "EXPANDED"; }
		}
		%s
		if (!_inp) return "NOT_FOUND";
		_inp.click(); _inp.focus(); return "FOUND";
	})()`, articleScopeJS(postID), findInputJS)

	var expandResult string
	if err := chromedp.Run(scrapeCtx, chromedp.Evaluate(expandJS, &expandResult)); err != nil {
		return fmt.Errorf("expand comment: %w", err)
	}
	log.Printf("[AutoComment] 📝 Expand: %s", expandResult)
	if expandResult == "NOT_FOUND" {
		return fmt.Errorf("could not find comment input for post %s", postID)
	}
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(2*time.Second))

	// Step 2: Attach image TRƯỚC khi type text (photo click sẽ làm mất React state nếu type trước)
	photoAttached := ac.attachImageToComment(scrapeCtx, postID, msg.ImagePath)
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(2*time.Second))

	// Step 3: Type text SAU attach image — dùng typeCommentText với CDP InsertText fallback
	if err := ac.typeCommentText(scrapeCtx, postID, msg.Content); err != nil {
		return fmt.Errorf("type comment: %w", err)
	}
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(1*time.Second))

	// Step 4: Submit — click the Post/Đăng button scoped to the target article
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(500*time.Millisecond))
	if err := ac.submitComment(scrapeCtx, postID); err != nil {
		return err
	}

	// Step 5: Verify input cleared
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(7*time.Second))
	if err := ac.verifySubmitted(scrapeCtx, postID, msg.ID); err != nil {
		return err
	}

	if photoAttached {
		log.Printf("[AutoComment] ✅ Comment+image posted on %s", msg.TargetURL)
	} else {
		log.Printf("[AutoComment] ✅ Comment posted (text only, no image) on %s", msg.TargetURL)
	}
	_ = ac.db.UpdateOutboundStatus(msg.ID, models.OutboundSent)
	if account != nil && account.ID > 0 {
		_ = ac.db.UpdateAccountLastUsed(account.ID)
	}
	return nil
}

// PostComment navigates to a Facebook post and posts a comment using the account's Chrome profile.
func (ac *AutoCommenter) PostComment(ctx context.Context, msg *models.OutboundMessage) error {
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

	scrapeCtx, cancel := context.WithTimeout(bCtx.Ctx, 90*time.Second)
	defer cancel()

	postID := extractPostID(msg.TargetURL)
	log.Printf("[AutoComment] 🔗 Navigating to post %s: %s", postID, msg.TargetURL)

	if err := chromedp.Run(scrapeCtx,
		chromedp.Navigate(msg.TargetURL),
		chromedp.Sleep(5*time.Second),
	); err != nil {
		return fmt.Errorf("navigate: %w", err)
	}

	var pageTitle string
	_ = chromedp.Run(scrapeCtx, chromedp.Title(&pageTitle))
	if pageTitle == "Log in to Facebook" || pageTitle == "Facebook – log in or sign up" {
		_ = ac.db.UpdateAccountStatus(account.ID, models.AccountInactive)
		return fmt.Errorf("account %s not logged in", account.Name)
	}

	var currentURL string
	_ = chromedp.Run(scrapeCtx, chromedp.Location(&currentURL))
	if !strings.Contains(currentURL, "facebook.com") {
		return fmt.Errorf("redirected away from Facebook: %s", currentURL)
	}
	if postID != "" && !strings.Contains(currentURL, postID) {
		return fmt.Errorf("redirected to wrong page (expected post %s, got %s)", postID, currentURL)
	}
	log.Printf("[AutoComment] ✅ URL verified: %s", currentURL)

	// Step 1: Expand the comment area of the TARGET post only
	expandJS := fmt.Sprintf(`
	(() => {
		%s
		for (const sel of ['div[aria-label="Leave a comment"]', 'div[aria-label="Bình luận"]', 'span[data-sigil="addcomment"]']) {
			const el = scope.querySelector(sel);
			if (el) { el.click(); return "EXPANDED"; }
		}
		%s
		if (!_inp) return "NOT_FOUND";
		_inp.click(); _inp.focus(); return "FOUND";
	})()`, articleScopeJS(postID), findInputJS)

	var expandResult string
	if err := chromedp.Run(scrapeCtx, chromedp.Evaluate(expandJS, &expandResult)); err != nil {
		return fmt.Errorf("expand comment: %w", err)
	}
	log.Printf("[AutoComment] 📝 Expand: %s", expandResult)
	if expandResult == "NOT_FOUND" {
		return fmt.Errorf("could not find comment input for post %s", postID)
	}
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(2*time.Second))

	// Step 2: Type text — với CDP InsertText fallback
	if err := ac.typeCommentText(scrapeCtx, postID, msg.Content); err != nil {
		return fmt.Errorf("type comment: %w", err)
	}

	// Step 3: Submit scoped to target article
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(1*time.Second))
	if err := ac.submitComment(scrapeCtx, postID); err != nil {
		return err
	}

	// Step 4: Verify input cleared
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(7*time.Second))
	if err := ac.verifySubmitted(scrapeCtx, postID, msg.ID); err != nil {
		return err
	}

	log.Printf("[AutoComment] ✅ Comment posted on %s: %s", msg.TargetURL, truncate(msg.Content, 50))
	_ = ac.db.UpdateOutboundStatus(msg.ID, models.OutboundSent)
	if account != nil && account.ID > 0 {
		_ = ac.db.UpdateAccountLastUsed(account.ID)
	}
	return nil
}

// typeCommentText focuses the comment input và type nội dung.
// Thử theo thứ tự: ClipboardEvent paste → execCommand → CDP InsertText (bypass React hoàn toàn).
// Mỗi method chạy RIÊNG BIỆT, clear input trước khi thử method tiếp theo — tránh text bị lặp.
func (ac *AutoCommenter) typeCommentText(scrapeCtx context.Context, postID, content string) error {
	// Helper JS: clear + focus input
	clearFocusJS := fmt.Sprintf(`
	(() => {
		%s
		%s
		if (!_inp) return "NO_INPUT";
		_inp.focus(); _inp.click();
		_inp.scrollIntoView({block:"center"});
		document.execCommand('selectAll', false, null);
		document.execCommand('delete', false, null);
		return "READY";
	})()`, articleScopeJS(postID), findInputJS)

	// Helper JS: check if input has text
	checkTextJS := fmt.Sprintf(`
	(() => {
		%s
		%s
		if (!_inp) return "EMPTY";
		const t = (_inp.textContent || _inp.innerText || "").trim();
		return t.length > 0 ? "OK:" + t.length : "EMPTY";
	})()`, articleScopeJS(postID), findInputJS)

	// ── Bước 1: Focus + clear input ──
	var focusResult string
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(clearFocusJS, &focusResult))
	log.Printf("[AutoComment] 🎯 Focus for type: %s", focusResult)
	if focusResult == "NO_INPUT" {
		return fmt.Errorf("comment input not found for typing")
	}
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(300*time.Millisecond))

	// ── Bước 2: Thử ClipboardEvent paste ──
	pasteJS := fmt.Sprintf(`
	(() => {
		%s
		%s
		if (!_inp) return "NO_INPUT";
		try {
			const dt = new DataTransfer();
			dt.setData('text/plain', %q);
			_inp.dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }));
		} catch(e) { return "ERROR:" + e.message; }
		return "DISPATCHED";
	})()`, articleScopeJS(postID), findInputJS, content)

	var pasteResult string
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(pasteJS, &pasteResult))
	log.Printf("[AutoComment] ⌨️ Paste dispatched: %s", pasteResult)

	// Wait for React to process the paste event asynchronously
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(500*time.Millisecond))

	// Check if paste worked
	var checkResult string
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(checkTextJS, &checkResult))
	log.Printf("[AutoComment] ⌨️ Post-paste check: %s", checkResult)
	if strings.HasPrefix(checkResult, "OK:") {
		return nil
	}

	// ── Bước 3: Clear + thử execCommand insertText ──
	log.Printf("[AutoComment] ⌨️ Fallback 1: execCommand insertText")
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(clearFocusJS, &focusResult))
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(200*time.Millisecond))

	execJS := fmt.Sprintf(`
	(() => {
		%s
		%s
		if (!_inp) return "NO_INPUT";
		_inp.focus();
		document.execCommand('insertText', false, %q);
		const t = (_inp.textContent || _inp.innerText || "").trim();
		return t.length > 0 ? "TYPED:" + t.length : "EMPTY";
	})()`, articleScopeJS(postID), findInputJS, content)

	var execResult string
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(execJS, &execResult))
	log.Printf("[AutoComment] ⌨️ execCommand result: %s", execResult)
	if strings.HasPrefix(execResult, "TYPED:") {
		return nil
	}

	// ── Bước 4: Clear + CDP InsertText — bypass mọi JS event handler ──
	log.Printf("[AutoComment] ⌨️ Fallback 2: CDP InsertText")
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(clearFocusJS, &focusResult))
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(200*time.Millisecond))

	if err := chromedp.Run(scrapeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.InsertText(content).Do(ctx)
		}),
	); err != nil {
		return fmt.Errorf("CDP InsertText failed: %w", err)
	}

	// Verify
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(300*time.Millisecond))
	var verifyResult string
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(checkTextJS, &verifyResult))
	log.Printf("[AutoComment] ⌨️ Type verify (CDP): %s", verifyResult)

	if verifyResult == "EMPTY" {
		return fmt.Errorf("all typing methods failed — comment input may be unresponsive")
	}
	return nil
}

// submitComment clicks the Post/Đăng button scoped to the target article,
// falling back to CDP Enter if the button is not found.
func (ac *AutoCommenter) submitComment(scrapeCtx context.Context, postID string) error {
	submitJS := fmt.Sprintf(`
	(() => {
		%s
		for (const sel of [
			'div[aria-label="Post"][role="button"]',
			'div[aria-label="Đăng"][role="button"]',
			'button[aria-label="Post"]',
			'button[aria-label="Đăng"]',
		]) {
			const el = scope.querySelector(sel);
			if (el) { el.click(); return "BTN_CLICKED"; }
		}
		// Re-focus correct input so CDP Enter lands on the right element
		%s
		if (_inp) { _inp.focus(); _inp.click(); }
		return "USE_ENTER";
	})()`, articleScopeJS(postID), findInputJS)

	var submitResult string
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(submitJS, &submitResult))
	log.Printf("[AutoComment] 🖱️ Submit: %s", submitResult)

	if submitResult != "BTN_CLICKED" {
		err := chromedp.Run(scrapeCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				return input.DispatchKeyEvent(input.KeyDown).
					WithKey("Enter").WithCode("Enter").
					WithNativeVirtualKeyCode(13).WithWindowsVirtualKeyCode(13).
					Do(ctx)
			}),
			chromedp.Sleep(100*time.Millisecond),
			chromedp.ActionFunc(func(ctx context.Context) error {
				return input.DispatchKeyEvent(input.KeyUp).
					WithKey("Enter").WithCode("Enter").
					WithNativeVirtualKeyCode(13).WithWindowsVirtualKeyCode(13).
					Do(ctx)
			}),
		)
		if err != nil {
			return fmt.Errorf("dispatch Enter key: %w", err)
		}
		log.Printf("[AutoComment] 🚀 Enter dispatched via CDP")
	}
	return nil
}

// verifySubmitted checks that the comment input was cleared (submitted successfully).
func (ac *AutoCommenter) verifySubmitted(scrapeCtx context.Context, postID string, msgID int64) error {
	verifyJS := fmt.Sprintf(`
	(() => {
		%s
		%s
		const inputText = _inp ? (_inp.textContent || "") : "";
		return JSON.stringify({ inputEmpty: inputText.trim().length === 0 });
	})()`, articleScopeJS(postID), findInputJS)

	var verifyResult string
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(verifyJS, &verifyResult))
	log.Printf("[AutoComment] 🔍 Verify: %s (msg #%d)", verifyResult, msgID)

	if strings.Contains(verifyResult, `"inputEmpty":false`) {
		return fmt.Errorf("comment submission failed: input not cleared (verify=%s)", verifyResult)
	}
	return nil
}

// attachImageToComment đính kèm ảnh vào comment bằng cách tìm nút/file input
// GẦN NHẤT với comment textbox theo khoảng cách visual (getBoundingClientRect).
// Facebook dùng React Portal nên toolbar KHÔNG nằm trong article scope — không thể dùng scope.querySelector.
// Khoảng cách visual luôn chọn đúng nút comment, vì Create Post ở đầu trang (xa hơn).
func (ac *AutoCommenter) attachImageToComment(scrapeCtx context.Context, postID, imagePath string) bool {
	// Bước 1: Refocus comment input để toolbar ảnh xuất hiện trong DOM
	refocusJS := fmt.Sprintf(`
	(() => {
		%s
		%s
		if (!_inp) return "NO_INPUT";
		_inp.focus(); _inp.click();
		return "FOCUSED";
	})()`, articleScopeJS(postID), findInputJS)
	var refocusResult string
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(refocusJS, &refocusResult))
	log.Printf("[AutoComment] 🎯 Refocus: %s", refocusResult)
	if refocusResult == "NO_INPUT" {
		log.Printf("[AutoComment] ⚠️ Cannot find comment input")
		return false
	}
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(800*time.Millisecond))

	// Bước 2: Click nút ảnh GẦN NHẤT với comment input (không dùng scope — React Portal)
	clickClosestJS := fmt.Sprintf(`
	(() => {
		%s
		%s
		if (!_inp) return "NO_INPUT";
		const inpR = _inp.getBoundingClientRect();
		const photoSels = [
			'span[aria-label="Đính kèm một ảnh hoặc video"]',
			'span[aria-label="Attach a photo or video"]',
			'div[aria-label="Đính kèm ảnh hoặc video"]',
			'div[aria-label="Đính kèm một bức ảnh hoặc video"]',
			'div[aria-label="Comment with an image or video"]',
			'div[aria-label="Viết bình luận có ảnh hoặc video"]',
			'span[aria-label*="ảnh"]',
			'span[aria-label*="photo"]',
			'div[aria-label*="ảnh"][role="button"]',
			'div[aria-label*="photo"][role="button"]',
		];
		let closest = null, minDist = Infinity;
		for (const sel of photoSels) {
			for (const btn of document.querySelectorAll(sel)) {
				const r = btn.getBoundingClientRect();
				if (r.width === 0 && r.height === 0) continue; // ẩn, bỏ qua
				const d = Math.abs(r.top - inpR.top) + Math.abs(r.left - inpR.left);
				if (d < minDist) { minDist = d; closest = btn; }
			}
		}
		if (!closest) return "NOT_FOUND";
		closest.click();
		return "CLICKED:dist" + Math.round(minDist);
	})()`, articleScopeJS(postID), findInputJS)

	var clickResult string
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(clickClosestJS, &clickResult))
	log.Printf("[AutoComment] 📷 Closest photo button: %s", clickResult)

	if !strings.HasPrefix(clickResult, "CLICKED") {
		log.Printf("[AutoComment] ⚠️ No photo button found on page, posting text only")
		return false
	}
	_ = chromedp.Run(scrapeCtx, chromedp.Sleep(1500*time.Millisecond))

	// Bước 3: Kiểm tra dialog sai (Create Post mở inline) → Escape nếu cần
	var dialogCheck string
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(`
	(() => {
		const dlg = document.querySelector('[role="dialog"]');
		if (dlg) {
			const markers = ['Tạo bài viết', 'Create post', 'Create Post', "What's on your mind?", 'Bạn đang nghĩ gì'];
			for (const m of markers) {
				if (dlg.innerHTML.includes(m)) return "WRONG_DIALOG";
			}
		}
		return "OK";
	})()`, &dialogCheck))
	log.Printf("[AutoComment] 🔎 Dialog check: %s", dialogCheck)

	if dialogCheck == "WRONG_DIALOG" {
		log.Printf("[AutoComment] ⚠️ Create Post dialog opened — pressing Escape, skipping image")
		_ = chromedp.Run(scrapeCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				return input.DispatchKeyEvent(input.KeyDown).
					WithKey("Escape").WithCode("Escape").
					WithNativeVirtualKeyCode(27).WithWindowsVirtualKeyCode(27).Do(ctx)
			}),
			chromedp.Sleep(300*time.Millisecond),
			chromedp.ActionFunc(func(ctx context.Context) error {
				return input.DispatchKeyEvent(input.KeyUp).
					WithKey("Escape").WithCode("Escape").
					WithNativeVirtualKeyCode(27).WithWindowsVirtualKeyCode(27).Do(ctx)
			}),
			chromedp.Sleep(700*time.Millisecond),
		)
		return false
	}

	// Bước 4: Tag file input GẦN NHẤT với comment input để SetUploadFiles dùng đúng chỗ
	tagFileInputJS := fmt.Sprintf(`
	(() => {
		%s
		%s
		if (!_inp) return "NO_INPUT";
		const inpR = _inp.getBoundingClientRect();
		// Xóa tag cũ
		for (const el of document.querySelectorAll('[data-thg-comment]')) {
			el.removeAttribute('data-thg-comment');
		}
		const fis = Array.from(document.querySelectorAll('input[type="file"]'));
		if (fis.length === 0) return "NOT_FOUND";
		let closest = null, minDist = Infinity;
		for (const fi of fis) {
			// file input ẩn (0x0) → lấy vị trí từ parent
			let el = fi.parentElement || fi;
			let r = el.getBoundingClientRect();
			for (let t = 0; t < 8 && r.width === 0 && r.height === 0 && el.parentElement; t++) {
				el = el.parentElement;
				r = el.getBoundingClientRect();
			}
			const d = Math.abs(r.top - inpR.top) + Math.abs(r.left - inpR.left);
			if (d < minDist) { minDist = d; closest = fi; }
		}
		if (!closest) { fis[0].setAttribute('data-thg-comment', '1'); return "TAGGED:fallback"; }
		closest.setAttribute('data-thg-comment', '1');
		return "TAGGED:dist" + Math.round(minDist);
	})()`, articleScopeJS(postID), findInputJS)

	var tagResult string
	_ = chromedp.Run(scrapeCtx, chromedp.Evaluate(tagFileInputJS, &tagResult))
	log.Printf("[AutoComment] 📁 File input: %s", tagResult)

	if tagResult == "NOT_FOUND" || tagResult == "NO_INPUT" {
		log.Printf("[AutoComment] ⚠️ No file input found after photo click, posting text only")
		return false
	}

	// Bước 5: Upload ảnh vào đúng file input đã tag
	absPath, absErr := filepath.Abs(imagePath)
	if absErr != nil {
		absPath = imagePath
	}
	uploadErr := chromedp.Run(scrapeCtx,
		chromedp.SetUploadFiles(`input[type="file"][data-thg-comment="1"]`, []string{absPath}),
		chromedp.Sleep(3*time.Second),
	)
	if uploadErr != nil {
		log.Printf("[AutoComment] ⚠️ Image upload failed: %v", uploadErr)
		return false
	}
	log.Printf("[AutoComment] 🖼️ Image attached: %s", filepath.Base(imagePath))
	return true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// extractPostID extracts the post ID from a Facebook post URL.
// e.g. https://www.facebook.com/groups/123/posts/pfbid02XYZ/ → "pfbid02XYZ"
func extractPostID(url string) string {
	url = strings.TrimRight(url, "/")
	parts := strings.Split(url, "/")
	for i, p := range parts {
		if (p == "posts" || p == "permalink") && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	// pfbid or story_fbid in query string
	for _, p := range parts {
		if strings.HasPrefix(p, "pfbid") {
			return p
		}
	}
	return ""
}
