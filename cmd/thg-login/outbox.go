package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

func executeApprovedOutbox(serverURL, token string, bridges map[int64]*chromeBridge) bool {
	messages, err := fetchApprovedOutbox(serverURL, token)
	if err != nil {
		if isDeviceTokenRejected(err) {
			printDeviceTokenRejected(err)
			return false
		}
		fmt.Println("[warn] outbox sync failed:", err)
		return true
	}
	if len(messages) > 0 {
		fmt.Printf("[Outbox] received %d approved automation action(s)\n", len(messages))
	}
	for _, msg := range messages {
		bridge := bridges[msg.AccountID]
		errText := ""
		result, err := executeOutboundMessage(msg, bridge)
		if err != nil {
			errText = err.Error()
			fmt.Printf("[warn] outbox %d (%s) failed: %s\n", msg.ID, msg.Type, errText)
		} else {
			fmt.Printf("[Outbox] action %d (%s) sent with account %d -> %s\n", msg.ID, msg.Type, msg.AccountID, result)
		}
		if err := completeOutboxMessage(serverURL, token, msg.ID, err == nil, errText); err != nil {
			if isDeviceTokenRejected(err) {
				printDeviceTokenRejected(err)
				return false
			}
			fmt.Printf("[warn] outbox %d completion failed: %v\n", msg.ID, err)
		}
	}
	return true
}

func executeOutboundMessage(msg outboundMessage, bridge *chromeBridge) (string, error) {
	if bridge == nil || bridge.ctx == nil || bridge.err != nil {
		return "", fmt.Errorf("Chrome profile for account %d is not ready", msg.AccountID)
	}
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return "", fmt.Errorf("outbox content is empty")
	}
	if len([]rune(content)) > 3000 {
		return "", fmt.Errorf("outbox content is too long")
	}
	ctx, cancel := context.WithTimeout(bridge.ctx, outboundActionTimeout())
	defer cancel()
	switch strings.ToLower(strings.TrimSpace(msg.Type)) {
	case "comment":
		return executeFacebookCommentAction(ctx, msg.TargetURL, content)
	case "inbox":
		return executeFacebookInboxAction(ctx, msg.TargetURL, content)
	case "group_post":
		return executeFacebookPostAction(ctx, msg.TargetURL, content)
	default:
		return "", fmt.Errorf("unsupported outbox type %q", msg.Type)
	}
}

func executeFacebookCommentAction(ctx context.Context, targetURL, content string) (string, error) {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return "", fmt.Errorf("comment target URL is empty")
	}
	if err := chromedp.Run(ctx,
		chromedp.Navigate(targetURL),
		chromedp.WaitReady(`body`, chromedp.ByQuery),
		chromedp.Sleep(2500*time.Millisecond),
		chromedp.Evaluate(dismissFacebookBlockingOverlaysJS(), nil),
		chromedp.Sleep(800*time.Millisecond),
	); err != nil {
		return "", fmt.Errorf("open comment target: %w", err)
	}
	if err := ensureFacebookSessionReady(ctx); err != nil {
		return "", err
	}
	var result string
	if err := chromedp.Run(ctx, chromedp.Evaluate(facebookCommentActionJS(content), &result)); err != nil {
		return "", fmt.Errorf("comment action failed: %w", err)
	}
	if !strings.HasPrefix(result, "sent_") {
		return "", fmt.Errorf("comment not sent: %s", result)
	}
	return result, nil
}

func executeFacebookInboxAction(ctx context.Context, targetURL, content string) (string, error) {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return "", fmt.Errorf("inbox target URL is empty")
	}
	if err := chromedp.Run(ctx,
		chromedp.Navigate(targetURL),
		chromedp.WaitReady(`body`, chromedp.ByQuery),
		chromedp.Sleep(2500*time.Millisecond),
		chromedp.Evaluate(dismissFacebookBlockingOverlaysJS(), nil),
		chromedp.Sleep(800*time.Millisecond),
	); err != nil {
		return "", fmt.Errorf("open inbox target: %w", err)
	}
	if err := ensureFacebookSessionReady(ctx); err != nil {
		return "", err
	}
	var result string
	if err := chromedp.Run(ctx, chromedp.Evaluate(facebookInboxActionJS(content), &result)); err != nil {
		return "", fmt.Errorf("inbox action failed: %w", err)
	}
	if !strings.HasPrefix(result, "sent_") {
		return "", fmt.Errorf("inbox not sent: %s", result)
	}
	return result, nil
}

func executeFacebookPostAction(ctx context.Context, targetURL, content string) (string, error) {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return "", fmt.Errorf("post target URL is empty")
	}
	if err := chromedp.Run(ctx,
		chromedp.Navigate(targetURL),
		chromedp.WaitReady(`body`, chromedp.ByQuery),
		chromedp.Sleep(3*time.Second),
		chromedp.Evaluate(dismissFacebookBlockingOverlaysJS(), nil),
		chromedp.Sleep(800*time.Millisecond),
	); err != nil {
		return "", fmt.Errorf("open post target: %w", err)
	}
	if err := ensureFacebookSessionReady(ctx); err != nil {
		return "", err
	}
	var result string
	if err := chromedp.Run(ctx, chromedp.Evaluate(facebookPostActionJS(content), &result)); err != nil {
		return "", fmt.Errorf("post action failed: %w", err)
	}
	if !strings.HasPrefix(result, "sent_") {
		return "", fmt.Errorf("post not sent: %s", result)
	}
	return result, nil
}

func ensureFacebookSessionReady(ctx context.Context) error {
	var href, fbUserID, loginIdentifier string
	var loginFormVisible bool
	var identity facebookIdentity
	if err := chromedp.Run(ctx, readFacebookPageState(&href, &fbUserID, &loginIdentifier, &loginFormVisible, &identity)); err != nil {
		return fmt.Errorf("read Facebook session: %w", err)
	}
	if isFacebookHumanRequiredURL(href) {
		return fmt.Errorf("human verification required in Facebook")
	}
	if loginFormVisible || fbUserID == "" {
		return fmt.Errorf("Facebook session is not logged in")
	}
	return nil
}
