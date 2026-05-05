package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	cdpnetwork "github.com/chromedp/cdproto/network"
	cdppage "github.com/chromedp/cdproto/page"
	cdptarget "github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

func startChromeBridge(port int) *chromeBridge {
	return startChromeBridgeForTarget(browserTarget{AccountID: 0, AccountName: "Default Facebook"}, port)
}

func startChromeBridgeForTarget(target browserTarget, port int) *chromeBridge {
	if port <= 0 {
		port = 9222
	}
	devtoolsURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	chromePID := findLocalChromeProcessID(port)
	wsURL, err := chromeWebSocketURL(devtoolsURL)
	if err != nil {
		pid, launchErr := launchChrome(port, chromeUserDataDir(target.AccountID))
		if launchErr != nil {
			return &chromeBridge{accountID: target.AccountID, accountName: target.AccountName, port: port, err: fmt.Errorf("%v; launch chrome: %w", err, launchErr)}
		}
		chromePID = pid
		wsURL, err = waitChromeWebSocketURL(devtoolsURL, 15*time.Second)
		if err != nil {
			return &chromeBridge{accountID: target.AccountID, accountName: target.AccountName, port: port, err: err}
		}
	}
	if pid := findLocalChromeProcessID(port); pid > 0 {
		chromePID = pid
	}
	if chromePID == 0 {
		chromePID = findLocalChromeProcessID(port)
	}

	// Pin chromedp to the Facebook page Chrome already opened on launch
	// (port arg `https://www.facebook.com`). Without this, NewContext
	// auto-creates a fresh about:blank target — the user logs into
	// Tab 1 while the probe runs on Tab 2; status reads can return
	// stale state and, if Chrome ever consolidates tabs, the probe's
	// target dies and every heartbeat reports "Chrome is not connected".
	targetID := pickFacebookPageTarget(devtoolsURL)
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	var ctx context.Context
	var cancel context.CancelFunc
	if targetID != "" {
		ctx, cancel = chromedp.NewContext(allocCtx, chromedp.WithTargetID(cdptarget.ID(targetID)))
	} else {
		ctx, cancel = chromedp.NewContext(allocCtx)
	}
	bridge := &chromeBridge{
		accountID:   target.AccountID,
		accountName: target.AccountName,
		port:        port,
		pid:         chromePID,
		ctx:         ctx,
		targetID:    targetID,
		cancel: func() {
			cancel()
			allocCancel()
		},
	}
	installFacebookLoginNetworkCapture(ctx, bridge)
	startupCtx, startupCancel := context.WithTimeout(ctx, chromeStartupTimeout())
	startupActions := []chromedp.Action{
		cdpnetwork.Enable(),
		installFacebookLoginCaptureOnNewDocument(),
	}
	if targetID == "" {
		// We had to create a brand-new tab — bring it to facebook.com
		// so the probe sees Facebook state. When we attached to an
		// existing target, the user already navigated it; do not force
		// a re-navigation that would clobber a half-completed login.
		startupActions = append(startupActions,
			navigatePageNoWait("https://www.facebook.com"),
			chromedp.Sleep(2*time.Second),
		)
	}
	startupActions = append(startupActions, installFacebookLoginCapture())
	err = chromedp.Run(startupCtx, startupActions...)
	startupCancel()
	if err != nil {
		fmt.Printf("[Chrome] %s startup handshake is slow; keeping Chrome bridge alive for dashboard stream: %v\n", target.AccountName, err)
	}
	return bridge
}

// reattachBridgeToFacebookPage attempts to re-pin a bridge whose
// chromedp target died (user closed the Facebook tab, Chrome merged
// windows, etc.) onto a still-live page target. Returns true when a
// new context was successfully created. Rate-limits itself to once
// every 10 s so a permanently-dead Chrome doesn't burn CPU spinning
// up failed contexts every heartbeat.
func reattachBridgeToFacebookPage(bridge *chromeBridge) bool {
	if bridge == nil || bridge.port <= 0 {
		return false
	}
	if !bridge.lastReattemptAt.IsZero() && time.Since(bridge.lastReattemptAt) < 10*time.Second {
		return false
	}
	bridge.lastReattemptAt = time.Now()

	devtoolsURL := fmt.Sprintf("http://127.0.0.1:%d", bridge.port)
	wsURL, err := chromeWebSocketURL(devtoolsURL)
	if err != nil {
		// Chrome process is down — no point re-attaching, the snapshot
		// fallback will surface the error to the dashboard.
		return false
	}
	targetID := pickFacebookPageTarget(devtoolsURL)
	if targetID == "" {
		// No usable page target. Don't auto-create one here; that would
		// race with whatever the user is doing in Chrome. Wait for the
		// next heartbeat.
		return false
	}
	// Tear down the dead context first so we don't leak.
	if bridge.cancel != nil {
		bridge.cancel()
	}
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(cdptarget.ID(targetID)))
	bridge.ctx = ctx
	bridge.targetID = targetID
	bridge.cancel = func() {
		cancel()
		allocCancel()
	}
	if !bridge.reattemptWarned {
		fmt.Printf("[Chrome] %s re-attached to live Facebook tab (target %s)\n", bridge.accountName, shortTargetID(targetID))
		bridge.reattemptWarned = true
	}
	// Re-install Network.Enable on the new target so cookie reads work.
	startupCtx, startupCancel := context.WithTimeout(ctx, chromeStartupTimeout())
	defer startupCancel()
	_ = chromedp.Run(startupCtx, cdpnetwork.Enable(), installFacebookLoginCapture())
	return true
}

func shortTargetID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// pickFacebookPageTarget asks Chrome's /json/list for the best page
// target to attach to. It prefers a tab already on facebook.com; falls
// back to the first non-blank, non-devtools page; returns "" when only
// the about:blank chromedp tab exists (caller should then create one).
func pickFacebookPageTarget(devtoolsURL string) string {
	targets, err := chromeTargets(devtoolsURL)
	if err != nil {
		return ""
	}
	var fallback string
	for _, t := range targets {
		if t.Type != "page" {
			continue
		}
		id := chromeTargetID(devtoolsURL, t)
		if id == "" {
			continue
		}
		lower := strings.ToLower(t.URL)
		if strings.Contains(lower, "facebook.com") {
			return id
		}
		if fallback == "" && t.URL != "about:blank" && !strings.HasPrefix(t.URL, "devtools://") {
			fallback = id
		}
	}
	return fallback
}

func navigatePageNoWait(rawURL string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		_, _, _, _, err := cdppage.Navigate(rawURL).Do(ctx)
		return err
	})
}
