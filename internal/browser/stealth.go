package browser

import (
	"context"
	"log"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// Stealth injects anti-detection scripts and blocks heavy resources.
func Stealth(ctx context.Context) error {
	// Override navigator.webdriver to false
	script := `
		Object.defineProperty(navigator, 'webdriver', { get: () => false });
		Object.defineProperty(navigator, 'plugins', { get: () => [1, 2, 3, 4, 5] });
		Object.defineProperty(navigator, 'languages', { get: () => ['vi-VN', 'vi', 'en-US', 'en'] });
		window.chrome = { runtime: {} };
		const originalQuery = window.navigator.permissions.query;
		window.navigator.permissions.query = (parameters) =>
			parameters.name === 'notifications'
				? Promise.resolve({ state: Notification.permission })
				: originalQuery(parameters);
	`

	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(script).Do(ctx)
			return err
		}),
	)
}

// BlockResources sets up network interception to block heavy resources (images, CSS, fonts).
// This dramatically speeds up page loads for scraping.
func BlockResources(ctx context.Context) error {
	blockedTypes := map[network.ResourceType]bool{
		network.ResourceTypeImage:      true,
		network.ResourceTypeMedia:      true,
		network.ResourceTypeFont:       true,
		network.ResourceTypeStylesheet: true,
	}

	// Listen for RequestPaused events from the Fetch domain
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		if req, ok := ev.(*fetch.EventRequestPaused); ok {
			go func() {
				if blockedTypes[req.ResourceType] {
					_ = chromedp.Run(ctx,
						chromedp.ActionFunc(func(ctx context.Context) error {
							return fetch.FailRequest(req.RequestID, network.ErrorReasonBlockedByClient).Do(ctx)
						}),
					)
				} else {
					_ = chromedp.Run(ctx,
						chromedp.ActionFunc(func(ctx context.Context) error {
							return fetch.ContinueRequest(req.RequestID).Do(ctx)
						}),
					)
				}
			}()
		}
	})

	// Enable Fetch domain interception
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return fetch.Enable().Do(ctx)
		}),
	)
}

// InjectCookies loads saved cookies for session persistence.
func InjectCookies(ctx context.Context, cookies []*network.CookieParam) error {
	if len(cookies) == 0 {
		return nil
	}
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return network.SetCookies(cookies).Do(ctx)
		}),
	)
}

// LogNavigation logs page navigation events for debugging.
func LogNavigation(ctx context.Context) {
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		if nav, ok := ev.(*page.EventFrameNavigated); ok {
			log.Printf("[Nav] %s", nav.Frame.URL)
		}
	})
}
