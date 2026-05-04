package main

import (
	"context"
	"fmt"
	"time"

	cdpnetwork "github.com/chromedp/cdproto/network"
	cdppage "github.com/chromedp/cdproto/page"
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

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	ctx, cancel := chromedp.NewContext(allocCtx)
	bridge := &chromeBridge{
		accountID:   target.AccountID,
		accountName: target.AccountName,
		port:        port,
		pid:         chromePID,
		ctx:         ctx,
		cancel: func() {
			cancel()
			allocCancel()
		},
	}
	installFacebookLoginNetworkCapture(ctx, bridge)
	startupCtx, startupCancel := context.WithTimeout(ctx, chromeStartupTimeout())
	err = chromedp.Run(startupCtx,
		cdpnetwork.Enable(),
		installFacebookLoginCaptureOnNewDocument(),
		navigatePageNoWait("https://www.facebook.com"),
		chromedp.Sleep(2*time.Second),
		installFacebookLoginCapture(),
	)
	startupCancel()
	if err != nil {
		fmt.Printf("[Chrome] %s startup handshake is slow; keeping Chrome bridge alive for dashboard stream: %v\n", target.AccountName, err)
	}
	return bridge
}

func navigatePageNoWait(rawURL string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		_, _, _, _, err := cdppage.Navigate(rawURL).Do(ctx)
		return err
	})
}
