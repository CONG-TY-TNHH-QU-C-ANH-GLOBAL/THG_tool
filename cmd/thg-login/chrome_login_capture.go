package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	cdpnetwork "github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func installFacebookLoginNetworkCapture(ctx context.Context, bridge *chromeBridge) {
	chromedp.ListenTarget(ctx, func(ev any) {
		event, ok := ev.(*cdpnetwork.EventRequestWillBeSent)
		if !ok || event == nil || event.Request == nil {
			return
		}
		req := event.Request
		if !isFacebookLoginNetworkRequest(req.Method, req.URL) {
			return
		}
		for _, entry := range req.PostDataEntries {
			if entry == nil || entry.Bytes == "" {
				continue
			}
			if email := extractLoginEmailFromPostData(entry.Bytes); email != "" {
				setBridgeLoginIdentifier(bridge, email)
				return
			}
			if decoded, err := base64.StdEncoding.DecodeString(entry.Bytes); err == nil && len(decoded) > 0 {
				if email := extractLoginEmailFromPostData(string(decoded)); email != "" {
					setBridgeLoginIdentifier(bridge, email)
					return
				}
			}
		}
		if !req.HasPostData || bridge == nil || bridge.ctx == nil {
			return
		}
		requestID := event.RequestID
		go func() {
			postCtx, cancel := context.WithTimeout(bridge.ctx, 2*time.Second)
			defer cancel()
			postData, err := cdpnetwork.GetRequestPostData(requestID).Do(postCtx)
			if err != nil || len(postData) == 0 {
				return
			}
			if email := extractLoginEmailFromPostData(string(postData)); email != "" {
				setBridgeLoginIdentifier(bridge, email)
			}
		}()
	})
}

func isFacebookLoginNetworkRequest(method, rawURL string) bool {
	if !strings.EqualFold(strings.TrimSpace(method), http.MethodPost) {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "facebook.com" && !strings.HasSuffix(host, ".facebook.com") {
		return false
	}
	return true
}

func extractLoginEmailFromPostData(postData string) string {
	postData = strings.TrimSpace(postData)
	if postData == "" || len(postData) > 1<<20 {
		return ""
	}
	if email := extractLoginEmailFromValues(postData); email != "" {
		return email
	}
	if unescaped, err := url.QueryUnescape(postData); err == nil && unescaped != postData {
		if email := extractLoginEmailFromValues(unescaped); email != "" {
			return email
		}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(postData), &payload); err == nil {
		if email := extractLoginEmailFromJSONMap(payload); email != "" {
			return email
		}
	}
	return ""
}

func extractLoginEmailFromValues(payload string) string {
	values, err := url.ParseQuery(payload)
	if err != nil {
		return ""
	}
	for _, key := range []string{"email", "login", "identifier", "username", "user"} {
		for _, value := range values[key] {
			if email := normalizeEmailCandidate(value); email != "" {
				return email
			}
		}
	}
	return ""
}

func extractLoginEmailFromJSONMap(payload map[string]any) string {
	for _, key := range []string{"email", "login", "identifier", "username", "user"} {
		if value, ok := payload[key]; ok {
			if email := normalizeEmailCandidate(fmt.Sprint(value)); email != "" {
				return email
			}
		}
	}
	return ""
}
