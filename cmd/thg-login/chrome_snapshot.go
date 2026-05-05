package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	cdpnetwork "github.com/chromedp/cdproto/network"
	cdppage "github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

func snapshotChrome(bridge *chromeBridge) chromeSnapshot {
	if bridge == nil {
		return chromeSnapshot{Status: streamStatusConnectorOnline}
	}
	if bridge.err != nil || bridge.ctx == nil {
		errMsg := ""
		if bridge.err != nil {
			errMsg = bridge.err.Error()
		}
		return chromeSnapshot{AccountID: bridge.accountID, AccountName: bridge.accountName, Status: streamStatusChromeNotConnected, ChromeError: errMsg}
	}
	snapshotCtx, cancel := context.WithTimeout(bridge.ctx, chromeSnapshotTimeout())
	defer cancel()
	var href string
	var fbUserID string
	var loginIdentifier string
	var loginFormVisible bool
	var identity facebookIdentity
	var screenshot []byte
	err := chromedp.Run(snapshotCtx,
		readFacebookPageState(&href, &fbUserID, &loginIdentifier, &loginFormVisible, &identity),
	)
	if err != nil {
		// Diagnostic: surface the underlying chromedp error once per
		// bridge so the operator can see WHY the probe is failing
		// instead of just "Chrome is not connected" forever.
		if !bridge.probeErrorPrinted {
			fmt.Printf("[Chrome] %s probe error (will retry every heartbeat): %v\n", bridge.accountName, err)
			bridge.probeErrorPrinted = true
		}
		// The chromedp target might have been closed — for example the
		// user closed the Facebook tab, or Chrome consolidated to a
		// single window. Try to re-attach to a still-live page target
		// before falling back to chrome_not_connected.
		if reattachBridgeToFacebookPage(bridge) {
			snapshotCtx2, cancel2 := context.WithTimeout(bridge.ctx, chromeSnapshotTimeout())
			defer cancel2()
			err = chromedp.Run(snapshotCtx2,
				readFacebookPageState(&href, &fbUserID, &loginIdentifier, &loginFormVisible, &identity),
			)
		}
	}
	if err != nil {
		return fallbackChromeSnapshot(bridge, streamStatusChromeNotConnected, err.Error())
	}
	if loginIdentifier != "" {
		setBridgeLoginIdentifier(bridge, loginIdentifier)
	}
	lowerURL := strings.ToLower(href)
	humanRequired := isFacebookHumanRequiredURL(lowerURL)
	if fbUserID != "" && loginFormVisible && !humanRequired && time.Since(bridge.lastLoginRecovery) > 5*time.Second {
		bridge.lastLoginRecovery = time.Now()
		fmt.Printf("[Chrome] %s has Facebook cookies but still shows login form. Reloading Facebook feed for dashboard stream.\n", bridge.accountName)
		_ = chromedp.Run(snapshotCtx,
			navigatePageNoWait("https://www.facebook.com/"),
			chromedp.Sleep(2*time.Second),
			readFacebookPageState(&href, &fbUserID, &loginIdentifier, &loginFormVisible, &identity),
		)
		if loginIdentifier != "" {
			setBridgeLoginIdentifier(bridge, loginIdentifier)
		}
		lowerURL = strings.ToLower(href)
		humanRequired = isFacebookHumanRequiredURL(lowerURL)
	}
	err = chromedp.Run(snapshotCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			data, err := cdppage.CaptureScreenshot().
				WithFormat(cdppage.CaptureScreenshotFormatJpeg).
				WithQuality(40).
				Do(ctx)
			if err == nil && len(data) > 0 {
				screenshot = data
			}
			return nil
		}),
	)
	if err != nil {
		return fallbackChromeSnapshot(bridge, streamStatusChromeNotConnected, err.Error())
	}
	status := streamStatusFacebookLoginRequired
	if humanRequired {
		status = streamStatusFacebookHumanRequired
	}
	if fbUserID != "" && !humanRequired && (!loginFormVisible || facebookSessionURLLooksUsable(href)) {
		status = streamStatusFacebookLoggedIn
	}
	updateChromeWindowPosture(bridge, status)
	loginEmail := ""
	if status == streamStatusFacebookLoggedIn {
		loginEmail = normalizeEmailCandidate(bridgeLoginIdentifier(bridge))
	}
	out := chromeSnapshot{
		AccountID:     bridge.accountID,
		AccountName:   bridge.accountName,
		CurrentURL:    href,
		FBUserID:      fbUserID,
		FBDisplayName: identity.DisplayName,
		FBUsername:    identity.Username,
		FBProfileURL:  identity.ProfileURL,
		LoginEmail:    loginEmail,
		Status:        status,
	}
	if len(screenshot) > 0 {
		out.ScreenshotData = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(screenshot)
	}
	rememberChromeSnapshot(bridge, out)
	return out
}

func rememberChromeSnapshot(bridge *chromeBridge, snap chromeSnapshot) {
	if bridge == nil || snap.Status == "" || snap.Status == streamStatusChromeNotConnected {
		return
	}
	bridge.snapMu.Lock()
	bridge.lastSnap = snap
	bridge.lastSnapAt = time.Now()
	bridge.snapMu.Unlock()
}

func fallbackChromeSnapshot(bridge *chromeBridge, fallbackStatus, chromeError string) chromeSnapshot {
	if bridge == nil {
		return chromeSnapshot{Status: fallbackStatus, ChromeError: chromeError}
	}
	bridge.snapMu.Lock()
	last := bridge.lastSnap
	lastAt := bridge.lastSnapAt
	bridge.snapMu.Unlock()
	if !lastAt.IsZero() && time.Since(lastAt) <= 45*time.Second && last.Status != "" {
		last.ScreenshotData = ""
		last.ChromeError = chromeError
		return last
	}
	return chromeSnapshot{AccountID: bridge.accountID, AccountName: bridge.accountName, Status: fallbackStatus, ChromeError: chromeError}
}

func readFacebookPageState(href, fbUserID, loginIdentifier *string, loginFormVisible *bool, identity *facebookIdentity) chromedp.Action {
	return chromedp.Tasks{
		chromedp.Location(href),
		chromedp.ActionFunc(func(ctx context.Context) error {
			cookies, err := cdpnetwork.GetCookies().WithURLs([]string{
				"https://www.facebook.com",
				"https://facebook.com",
			}).Do(ctx)
			if err != nil {
				return err
			}
			*fbUserID = ""
			for _, ck := range cookies {
				if ck.Name == "c_user" && ck.Value != "" {
					*fbUserID = ck.Value
					break
				}
			}
			return nil
		}),
		installFacebookLoginCapture(),
		chromedp.Evaluate(`(() => {
			const email = document.querySelector('input[name="email"], input#email');
			const pass = document.querySelector('input[name="pass"], input#pass');
			const loginButton = document.querySelector('button[name="login"], input[name="login"]');
			const loginForm = document.querySelector('form[action*="login"], form[action*="/login/"]');
			return Boolean((email && pass) || (loginForm && loginButton));
		})()`, loginFormVisible),
		chromedp.Evaluate(facebookLoginIdentifierScript(), loginIdentifier),
		chromedp.Evaluate(facebookIdentityScript(), identity),
	}
}

func installFacebookLoginCaptureOnNewDocument() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		_, err := cdppage.AddScriptToEvaluateOnNewDocument(facebookLoginCaptureSource()).Do(ctx)
		return err
	})
}

func installFacebookLoginCapture() chromedp.Action {
	return chromedp.Evaluate(facebookLoginCaptureSource(), nil)
}

func facebookLoginCaptureSource() string {
	return `(() => {
		try {
			const key = "__thg_last_facebook_login_identifier";
			const prop = "__thgLastFacebookLoginIdentifier";
			const selectors = [
				'input[name="email"]',
				'input#email',
				'input[autocomplete="username"]',
				'input[type="email"]',
				'input[type="text"][name*="email" i]',
				'input[type="text"][autocomplete="username"]'
			];
			const readField = () => selectors.map((selector) => document.querySelector(selector)).find(Boolean);
			const save = (value) => {
				value = String(value || "").trim();
				if (!value) return "";
				value = value.slice(0, 320);
				window[prop] = value;
				try { window.localStorage.setItem(key, value); } catch (_) {}
				try { window.sessionStorage.setItem(key, value); } catch (_) {}
				return value;
			};
			const remember = () => {
				const field = readField();
				if (!field) return "";
				return save(field.value || field.getAttribute("value") || "");
			};
			const bindField = () => {
				const field = readField();
				if (!field || field.dataset.thgLoginCaptureBound) return Boolean(field);
				field.dataset.thgLoginCaptureBound = "1";
				["input", "change", "keyup", "keydown", "blur", "focusout"].forEach((eventName) => {
					field.addEventListener(eventName, remember, true);
				});
				remember();
				return true;
			};
			const bindDocument = () => {
				if (window.__thgFacebookLoginDocumentBound) return;
				window.__thgFacebookLoginDocumentBound = true;
				["submit", "click", "keydown", "beforeunload", "pagehide"].forEach((eventName) => {
					document.addEventListener(eventName, remember, true);
				});
				const observer = new MutationObserver(() => bindField());
				observer.observe(document.documentElement || document, { childList: true, subtree: true, attributes: true, attributeFilter: ["value"] });
			};
			bindDocument();
			bindField();
			return Boolean(window[prop] || (() => {
				try {
					return window.localStorage.getItem(key) || window.sessionStorage.getItem(key) || "";
				} catch (_) {
					return "";
				}
			})());
		} catch (_) {
			return false;
		}
	})()`
}

func facebookLoginIdentifierScript() string {
	return `(() => {
		try {
			const key = "__thg_last_facebook_login_identifier";
			const prop = "__thgLastFacebookLoginIdentifier";
			const fromWindow = String(window[prop] || "").trim();
			if (fromWindow) return fromWindow.slice(0, 320);
			let stored = "";
			try { stored = String(window.localStorage.getItem(key) || "").trim(); } catch (_) {}
			if (!stored) {
				try { stored = String(window.sessionStorage.getItem(key) || "").trim(); } catch (_) {}
			}
			if (stored) return stored.slice(0, 320);
			const selectors = [
				'input[name="email"]',
				'input#email',
				'input[autocomplete="username"]',
				'input[type="email"]',
				'input[type="text"][name*="email" i]',
				'input[type="text"][autocomplete="username"]'
			];
			const field = selectors.map((selector) => document.querySelector(selector)).find(Boolean);
			if (!field) return "";
			const value = String(field.value || field.getAttribute("value") || "").trim();
			if (value) {
				window[prop] = value.slice(0, 320);
				try { window.localStorage.setItem(key, value.slice(0, 320)); } catch (_) {}
				try { window.sessionStorage.setItem(key, value.slice(0, 320)); } catch (_) {}
			}
			return value.slice(0, 320);
		} catch (_) {
			return "";
		}
	})()`
}

func facebookIdentityScript() string {
	return `(() => {
		try {
			const out = {display_name: "", username: "", profile_url: ""};
			const cleanText = (value) => String(value || "")
				.replace(/\s+/g, " ")
				.replace(/^(profile|your profile|trang cÃ¡ nhÃ¢n|xem trang cÃ¡ nhÃ¢n)\s*/i, "")
				.trim()
				.slice(0, 120);
			const badPath = new Set([
				"groups", "friends", "watch", "marketplace", "messages", "notifications",
				"settings", "help", "privacy", "gaming", "reel", "reels", "stories",
				"pages", "events", "memories", "saved", "bookmarks", "ads", "business"
			]);
			const normalizeURL = (href) => {
				try {
					const u = new URL(href, location.href);
					if (!/(^|\.)facebook\.com$/i.test(u.hostname)) return "";
					u.hash = "";
					return u.toString();
				} catch (_) {
					return "";
				}
			};
			const cUser = (() => {
				const m = String(document.cookie || "").match(/(?:^|;\s*)c_user=([^;]+)/);
				return m ? decodeURIComponent(m[1]) : "";
			})();
			const anchors = Array.from(document.querySelectorAll("a[href]"));
			const candidates = [];
			for (const a of anchors) {
				const href = normalizeURL(a.getAttribute("href") || a.href || "");
				if (!href) continue;
				const u = new URL(href);
				const path = u.pathname.replace(/^\/+|\/+$/g, "");
				const first = path.split("/")[0] || "";
				const text = cleanText(a.innerText || a.getAttribute("aria-label") || a.getAttribute("title") || "");
				let score = 0;
				if (cUser && u.searchParams.get("id") === cUser) score += 120;
				if (cUser && href.includes("profile.php") && href.includes(cUser)) score += 110;
				if (text && !badPath.has(first.toLowerCase())) score += 20;
				if (first && !badPath.has(first.toLowerCase()) && !first.includes(".php")) score += 15;
				if (/profile/i.test(a.getAttribute("aria-label") || "")) score += 10;
				if (score > 0) candidates.push({href, text, first, score});
			}
			candidates.sort((a, b) => b.score - a.score);
			const best = candidates[0];
			if (best) {
				out.profile_url = best.href;
				out.display_name = best.text;
				if (best.first && !best.first.includes(".php") && !badPath.has(best.first.toLowerCase())) {
					out.username = best.first.replace(/^@+/, "").slice(0, 80);
				}
			}
			if (!out.display_name) {
				const rawLabel = Array.from(document.querySelectorAll('[aria-label]'))
					.map(el => String(el.getAttribute("aria-label") || ""))
					.find(text => /profile|trang cÃ¡ nhÃ¢n|trang ca nhan/i.test(text));
				const label = cleanText(rawLabel || "");
				if (label && !/^(profile|your profile|trang cÃ¡ nhÃ¢n|xem trang cÃ¡ nhÃ¢n)$/i.test(label)) out.display_name = label;
			}
			return out;
		} catch (_) {
			return {display_name: "", username: "", profile_url: ""};
		}
	})()`
}

func setBridgeLoginIdentifier(bridge *chromeBridge, value string) {
	if bridge == nil {
		return
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if len(value) > 320 {
		value = value[:320]
	}
	email := normalizeEmailCandidate(value)
	var masked string
	bridge.loginMu.Lock()
	bridge.loginIdentifier = value
	if email != "" && bridge.loginCaptureLog != email {
		bridge.loginCaptureLog = email
		masked = maskEmail(email)
	}
	accountName := bridge.accountName
	bridge.loginMu.Unlock()
	if masked != "" {
		fmt.Printf("[Chrome] Captured Facebook login email for %s: %s\n", accountName, masked)
	}
}

func bridgeLoginIdentifier(bridge *chromeBridge) string {
	if bridge == nil {
		return ""
	}
	bridge.loginMu.Lock()
	defer bridge.loginMu.Unlock()
	return bridge.loginIdentifier
}

func maskEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.Index(email, "@")
	if at <= 0 {
		return "***"
	}
	name := email[:at]
	domain := email[at+1:]
	if len(name) <= 2 {
		return name[:1] + "***@" + domain
	}
	return name[:2] + "***@" + domain
}

func normalizeEmailCandidate(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || len(value) > 320 {
		return ""
	}
	if strings.ContainsAny(value, " \t\r\n") || !strings.Contains(value, "@") {
		return ""
	}
	return value
}

func isFacebookHumanRequiredURL(rawURL string) bool {
	rawURL = strings.TrimSpace(strings.ToLower(rawURL))
	if rawURL == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return strings.Contains(rawURL, "/checkpoint") || strings.Contains(rawURL, "/two_step")
	}
	path := strings.ToLower(parsed.EscapedPath())
	return strings.Contains(path, "/checkpoint") ||
		strings.Contains(path, "/two_step") ||
		strings.Contains(path, "/two_step_verification")
}

func facebookSessionURLLooksUsable(rawURL string) bool {
	rawURL = strings.TrimSpace(strings.ToLower(rawURL))
	if rawURL == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return !strings.Contains(rawURL, "/login") && !strings.Contains(rawURL, "login.php")
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "facebook.com" && !strings.HasSuffix(host, ".facebook.com") {
		return false
	}
	path := strings.ToLower(parsed.EscapedPath())
	if strings.Contains(path, "/login") || strings.Contains(path, "login.php") {
		return false
	}
	return true
}

func chromeSnapshotTimeout() time.Duration {
	seconds, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("THG_SNAPSHOT_TIMEOUT_SECONDS")))
	if seconds <= 0 {
		seconds = 8
	}
	if seconds < 3 {
		seconds = 3
	}
	if seconds > 30 {
		seconds = 30
	}
	return time.Duration(seconds) * time.Second
}

func chromeStartupTimeout() time.Duration {
	seconds, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("THG_CHROME_STARTUP_TIMEOUT_SECONDS")))
	if seconds <= 0 {
		seconds = 8
	}
	if seconds < 3 {
		seconds = 3
	}
	if seconds > 30 {
		seconds = 30
	}
	return time.Duration(seconds) * time.Second
}

func updateChromeWindowPosture(bridge *chromeBridge, status string) {
	if bridge == nil || bridge.ctx == nil || keepLocalChromeVisibleAfterLogin() {
		return
	}
	postureCtx, cancel := context.WithTimeout(bridge.ctx, 3*time.Second)
	defer cancel()
	if status == streamStatusFacebookLoggedIn {
		// Spec from operators: Chrome only hides AFTER the server has
		// accepted the Facebook identity for this account slot
		// (applyConnectorIdentity bound fb_user_id without a 409
		// mismatch). Until that confirmation arrives, the window stays
		// visible — a stale c_user cookie from a prior run can produce
		// a client-side "logged in" reading on the first heartbeat,
		// and we MUST NOT minimize the window before the operator has
		// completed and persisted a real login.
		if !bridge.identityConfirmed {
			return
		}
		if bridge.windowHidden && time.Since(bridge.lastWindowPosture) < 5*time.Second {
			return
		}
		bridge.lastWindowPosture = time.Now()
		if err := hideChromeWindowAfterLogin(postureCtx, bridge.pid); err != nil {
			if !bridge.windowWarned {
				fmt.Printf("[Chrome] Could not move %s to dashboard-only mode: %v\n", bridge.accountName, err)
				bridge.windowWarned = true
			}
			return
		}
		bridge.windowHidden = true
		bridge.windowWarned = false
		fmt.Printf("[Chrome] %s logged in. Local Chrome is locked to dashboard-only mode; continue in the Browser dashboard.\n", bridge.accountName)
		return
	}
	if !bridge.windowHidden && time.Since(bridge.lastWindowPosture) < 5*time.Second {
		return
	}
	bridge.lastWindowPosture = time.Now()
	if err := showChromeWindowForLogin(postureCtx, bridge.pid); err != nil {
		if !bridge.windowWarned {
			fmt.Printf("[Chrome] Could not show %s for local login/checkpoint: %v\n", bridge.accountName, err)
			bridge.windowWarned = true
		}
		return
	}
	bridge.windowHidden = false
	bridge.windowWarned = false
	fmt.Printf("[Chrome] %s needs local login/checkpoint. Chrome is visible on this device.\n", bridge.accountName)
}
