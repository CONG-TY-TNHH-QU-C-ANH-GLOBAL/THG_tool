package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

func executeLocalCrawlCommand(serverURL, token string, cmd connectorCommand, bridge *chromeBridge) (string, error) {
	var task localCrawlTask
	if err := json.Unmarshal([]byte(defaultString(cmd.PayloadJSON, "{}")), &task); err != nil {
		return "", err
	}
	if task.TaskID == "" || task.AccountID <= 0 {
		err := fmt.Errorf("crawl command missing task_id/account_id")
		_ = sendCrawlResult(serverURL, token, localCrawlResult{TaskID: task.TaskID, Intent: task.Intent, AccountID: cmd.AccountID, Status: "failed", Error: err.Error(), Keywords: task.Keywords})
		return "", err
	}
	if task.AccountID != cmd.AccountID {
		err := fmt.Errorf("crawl command account mismatch: command=%d task=%d", cmd.AccountID, task.AccountID)
		_ = sendCrawlResult(serverURL, token, localCrawlResult{TaskID: task.TaskID, Intent: task.Intent, AccountID: cmd.AccountID, Status: "failed", Error: err.Error(), Keywords: task.Keywords})
		return "", err
	}
	fmt.Printf("[Crawl] starting command %d task=%s account=%d sources=%d\n", cmd.ID, task.TaskID, task.AccountID, len(task.CrawlPlan.Sources))
	ctx, cancel := context.WithTimeout(bridge.ctx, 3*time.Minute)
	defer cancel()

	var href, fbUserID, loginIdentifier string
	var loginFormVisible bool
	var identity facebookIdentity
	if err := chromedp.Run(ctx, readFacebookPageState(&href, &fbUserID, &loginIdentifier, &loginFormVisible, &identity)); err != nil {
		_ = sendCrawlResult(serverURL, token, localCrawlResult{TaskID: task.TaskID, Intent: task.Intent, AccountID: task.AccountID, Status: "failed", Error: err.Error(), Keywords: task.Keywords})
		return "", err
	}
	if fbUserID == "" || loginFormVisible || isFacebookHumanRequiredURL(href) {
		err := fmt.Errorf("facebook session is not ready for crawl")
		_ = sendCrawlResult(serverURL, token, localCrawlResult{TaskID: task.TaskID, Intent: task.Intent, AccountID: task.AccountID, Status: "failed", Error: err.Error(), Keywords: task.Keywords})
		return "", err
	}

	maxItems := task.CrawlPlan.MaxItems
	if maxItems <= 0 {
		maxItems = 50
	}
	batchSize := task.CrawlPlan.BatchSize
	if batchSize <= 0 {
		batchSize = 20
	}
	if batchSize > maxItems {
		batchSize = maxItems
	}
	items := make([]localCrawlItem, 0, maxItems)
	seen := map[string]bool{}
	for _, source := range task.CrawlPlan.Sources {
		if len(items) >= maxItems {
			break
		}
		fmt.Printf("[Crawl] opening %s\n", source.URL)
		sourceItems, err := crawlSourceWithChrome(ctx, source, maxItems-len(items), batchSize)
		if err != nil {
			_ = sendCrawlResult(serverURL, token, localCrawlResult{TaskID: task.TaskID, Intent: task.Intent, AccountID: task.AccountID, Status: "failed", Error: err.Error(), Keywords: task.Keywords, Items: items})
			return "", err
		}
		for _, item := range sourceItems {
			key := item.ID
			if key == "" {
				key = item.SourceURL + "|" + item.AuthorName + "|" + item.Content
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			items = append(items, item)
			if len(items) >= maxItems {
				break
			}
		}
	}
	result := localCrawlResult{
		TaskID:    task.TaskID,
		Intent:    task.Intent,
		AccountID: task.AccountID,
		Status:    "completed",
		Keywords:  firstNonEmptyStringSlice(task.Keywords, task.Filters.Keywords),
		Items:     items,
	}
	if err := sendCrawlResult(serverURL, token, result); err != nil {
		return "", err
	}
	fmt.Printf("[Crawl] completed task=%s items=%d\n", task.TaskID, len(items))
	return fmt.Sprintf("crawl_completed items=%d", len(items)), nil
}

func crawlSourceWithChrome(ctx context.Context, source localCrawlSource, maxItems, batchSize int) ([]localCrawlItem, error) {
	source.URL = strings.TrimSpace(source.URL)
	if source.URL == "" {
		return nil, fmt.Errorf("crawl source URL is empty")
	}
	if maxItems <= 0 {
		return nil, nil
	}
	if batchSize <= 0 || batchSize > maxItems {
		batchSize = maxItems
	}
	var rawJSON string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(source.URL),
		chromedp.WaitReady(`body`, chromedp.ByQuery),
		chromedp.Sleep(4*time.Second),
		chromedp.Evaluate(dismissFacebookBlockingOverlaysJS(), nil),
		chromedp.Sleep(1200*time.Millisecond),
	); err != nil {
		return nil, fmt.Errorf("navigate %s: %w", source.URL, err)
	}
	items := make([]localCrawlItem, 0, maxItems)
	seen := map[string]bool{}
	for attempt := 0; attempt < 4 && len(items) < maxItems; attempt++ {
		script := localExtractPostsJS(batchSize)
		if strings.Contains(source.URL, "/search/groups") || source.Type == "facebook_search" {
			script = localExtractGroupsJS(batchSize)
		}
		if err := chromedp.Run(ctx, chromedp.Evaluate(script, &rawJSON)); err != nil {
			return nil, fmt.Errorf("extract %s: %w", source.URL, err)
		}
		var batch []localCrawlItem
		if err := json.Unmarshal([]byte(rawJSON), &batch); err != nil {
			return nil, fmt.Errorf("parse extracted crawl JSON: %w", err)
		}
		for _, item := range batch {
			if item.SourceURL == "" {
				item.SourceURL = source.URL
			}
			key := item.ID
			if key == "" {
				key = item.SourceURL + "|" + item.AuthorName + "|" + item.Content
			}
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			items = append(items, item)
			if len(items) >= maxItems {
				break
			}
		}
		if len(items) >= maxItems {
			break
		}
		_ = chromedp.Run(ctx,
			chromedp.Evaluate(`window.scrollBy(0, Math.max(900, window.innerHeight || 900)); "scrolled";`, nil),
			chromedp.Sleep(2*time.Second),
		)
	}
	return items, nil
}

func sendCrawlResult(serverURL, token string, result localCrawlResult) error {
	return postAgentJSON(serverURL+"/api/connectors/crawl-result", token, result, 30*time.Second, nil)
}

func dismissFacebookBlockingOverlaysJS() string {
	return `(() => {
  const labels = new Set(["not now", "ok", "close", "Ä‘á»ƒ sau", "lÃºc khÃ¡c", "khÃ´ng pháº£i bÃ¢y giá»"]);
  const candidates = Array.from(document.querySelectorAll('div[role="button"], button, [aria-label]'));
  for (const el of candidates) {
    const text = String(el.innerText || el.getAttribute('aria-label') || '').trim().toLowerCase();
    if (!text) continue;
    if (labels.has(text) || text.includes("not now") || text.includes("remember password")) {
      try { el.click(); return "clicked"; } catch (_) {}
    }
  }
  const close = document.querySelector('[aria-label="Close"], [aria-label="ÄÃ³ng"], [aria-label="ÄÃ³ng cá»­a sá»•"]');
  if (close) {
    try { close.click(); return "closed"; } catch (_) {}
  }
  return "none";
})()`
}

func facebookCommentActionJS(content string) string {
	return fmt.Sprintf(`(async () => {
  const text = %s;
  const wait = (ms) => new Promise(resolve => setTimeout(resolve, ms));
  const norm = (v) => String(v || '').trim().toLowerCase();
  const visible = (el) => {
    if (!el) return false;
    const r = el.getBoundingClientRect();
    const style = getComputedStyle(el);
    return r.width > 8 && r.height > 8 && style.visibility !== 'hidden' && style.display !== 'none';
  };
  const labelOf = (el) => norm(el.innerText || el.getAttribute('aria-label') || el.getAttribute('placeholder') || el.title);
  const hasAny = (value, keys) => keys.some(k => value.includes(k));
  const commentKeys = ['comment', 'write a comment', 'b\u00ecnh lu\u1eadn', 'vi\u1ebft b\u00ecnh lu\u1eadn'];
  const submitKeys = ['comment', 'post', 'send', 'b\u00ecnh lu\u1eadn', '\u0111\u0103ng', 'g\u1eedi'];
  const clickLikeUser = (el) => {
    if (!el) return false;
    try { el.scrollIntoView({block:'center', inline:'center'}); } catch (_) {}
    try { el.click(); return true; } catch (_) { return false; }
  };
  const buttons = Array.from(document.querySelectorAll('div[role="button"], button, a[role="button"], span[role="button"]')).filter(visible);
  const commentButton = buttons.find(el => {
    const label = labelOf(el);
    return hasAny(label, commentKeys) && !label.includes('share') && !label.includes('like');
  });
  if (commentButton) {
    clickLikeUser(commentButton);
    await wait(900);
  }
  const editors = Array.from(document.querySelectorAll('[contenteditable="true"], textarea, input[type="text"]')).filter(visible);
  let editor = editors.find(el => hasAny(labelOf(el), commentKeys));
  if (!editor) editor = editors.find(el => (el.getAttribute('role') || '').toLowerCase() === 'textbox');
  if (!editor) editor = editors[0];
  if (!editor) return 'comment_box_not_found';
  try { editor.focus({preventScroll:true}); } catch (_) { try { editor.focus(); } catch (_) {} }
  if (editor.isContentEditable) {
    document.execCommand('insertText', false, text);
  } else if ('value' in editor) {
    const proto = editor instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    const setter = Object.getOwnPropertyDescriptor(proto, 'value') && Object.getOwnPropertyDescriptor(proto, 'value').set;
    if (setter) setter.call(editor, text); else editor.value = text;
  }
  try { editor.dispatchEvent(new InputEvent('input', {bubbles:true, inputType:'insertText', data:text})); } catch (_) { editor.dispatchEvent(new Event('input', {bubbles:true})); }
  await wait(700);
  const scope = editor.closest('[role="dialog"], form, [role="article"]') || document;
  const submit = Array.from(scope.querySelectorAll('div[role="button"], button, [aria-label]')).filter(visible).find(el => {
    const label = labelOf(el);
    if (!hasAny(label, submitKeys)) return false;
    if (label.includes('share') || label.includes('like') || label.includes('cancel')) return false;
    return el.getAttribute('aria-disabled') !== 'true' && !el.disabled;
  });
  if (submit && clickLikeUser(submit)) {
    await wait(1000);
    return 'sent_comment_button';
  }
  return 'comment_submit_not_found';
})()`, jsString(content))
}

func facebookInboxActionJS(content string) string {
	return fmt.Sprintf(`(async () => {
  const text = %s;
  const wait = (ms) => new Promise(resolve => setTimeout(resolve, ms));
  const norm = (v) => String(v || '').trim().toLowerCase();
  const visible = (el) => {
    if (!el) return false;
    const r = el.getBoundingClientRect();
    const style = getComputedStyle(el);
    return r.width > 8 && r.height > 8 && style.visibility !== 'hidden' && style.display !== 'none';
  };
  const labelOf = (el) => norm(el.innerText || el.getAttribute('aria-label') || el.getAttribute('placeholder') || el.title);
  const hasAny = (value, keys) => keys.some(k => value.includes(k));
  const messageKeys = ['message', 'messenger', 'send message', 'nh\u1eafn tin'];
  const sendKeys = ['send', 'press enter to send', 'g\u1eedi'];
  const clickLikeUser = (el) => {
    if (!el) return false;
    try { el.scrollIntoView({block:'center', inline:'center'}); } catch (_) {}
    try { el.click(); return true; } catch (_) { return false; }
  };
  let editors = Array.from(document.querySelectorAll('[contenteditable="true"], textarea')).filter(visible);
  if (!editors.length) {
    const messageButton = Array.from(document.querySelectorAll('div[role="button"], button, a[role="button"]')).filter(visible).find(el => hasAny(labelOf(el), messageKeys));
    if (!messageButton || !clickLikeUser(messageButton)) return 'message_button_not_found';
    await wait(1800);
    editors = Array.from(document.querySelectorAll('[contenteditable="true"], textarea')).filter(visible);
  }
  let editor = editors.find(el => hasAny(labelOf(el), messageKeys) || (el.getAttribute('role') || '').toLowerCase() === 'textbox');
  if (!editor) editor = editors[editors.length - 1];
  if (!editor) return 'message_box_not_found';
  try { editor.focus({preventScroll:true}); } catch (_) { try { editor.focus(); } catch (_) {} }
  if (editor.isContentEditable) {
    document.execCommand('insertText', false, text);
  } else if ('value' in editor) {
    const proto = editor instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    const setter = Object.getOwnPropertyDescriptor(proto, 'value') && Object.getOwnPropertyDescriptor(proto, 'value').set;
    if (setter) setter.call(editor, text); else editor.value = text;
  }
  try { editor.dispatchEvent(new InputEvent('input', {bubbles:true, inputType:'insertText', data:text})); } catch (_) { editor.dispatchEvent(new Event('input', {bubbles:true})); }
  await wait(700);
  const scope = editor.closest('[role="dialog"], form, div[aria-label]') || document;
  const send = Array.from(scope.querySelectorAll('div[role="button"], button, [aria-label]')).filter(visible).find(el => {
    const label = labelOf(el);
    return hasAny(label, sendKeys) && el.getAttribute('aria-disabled') !== 'true' && !el.disabled;
  });
  if (send && clickLikeUser(send)) {
    await wait(1000);
    return 'sent_inbox_button';
  }
  return 'inbox_submit_not_found';
})()`, jsString(content))
}

func facebookPostActionJS(content string) string {
	return fmt.Sprintf(`(async () => {
  const text = %s;
  const wait = (ms) => new Promise(resolve => setTimeout(resolve, ms));
  const norm = (v) => String(v || '').trim().toLowerCase();
  const visible = (el) => {
    if (!el) return false;
    const r = el.getBoundingClientRect();
    const style = getComputedStyle(el);
    return r.width > 8 && r.height > 8 && style.visibility !== 'hidden' && style.display !== 'none';
  };
  const labelOf = (el) => norm(el.innerText || el.getAttribute('aria-label') || el.getAttribute('placeholder') || el.title);
  const hasAny = (value, keys) => keys.some(k => value.includes(k));
  const composerKeys = ["what's on your mind", 'write something', 'create a public post', 'b\u1ea1n \u0111ang ngh\u0129 g\u00ec', 'vi\u1ebft g\u00ec \u0111\u00f3'];
  const postKeys = ['post', '\u0111\u0103ng'];
  const clickLikeUser = (el) => {
    if (!el) return false;
    try { el.scrollIntoView({block:'center', inline:'center'}); } catch (_) {}
    try { el.click(); return true; } catch (_) { return false; }
  };
  const composer = Array.from(document.querySelectorAll('div[role="button"], button, textarea, [contenteditable="true"], [aria-label]')).filter(visible).find(el => hasAny(labelOf(el), composerKeys));
  if (!composer || !clickLikeUser(composer)) return 'post_composer_not_found';
  await wait(1500);
  const editors = Array.from(document.querySelectorAll('[contenteditable="true"], textarea')).filter(visible);
  let editor = editors.find(el => (el.getAttribute('role') || '').toLowerCase() === 'textbox') || editors[editors.length - 1];
  if (!editor) return 'post_editor_not_found';
  try { editor.focus({preventScroll:true}); } catch (_) { try { editor.focus(); } catch (_) {} }
  if (editor.isContentEditable) {
    document.execCommand('insertText', false, text);
  } else if ('value' in editor) {
    const proto = editor instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    const setter = Object.getOwnPropertyDescriptor(proto, 'value') && Object.getOwnPropertyDescriptor(proto, 'value').set;
    if (setter) setter.call(editor, text); else editor.value = text;
  }
  try { editor.dispatchEvent(new InputEvent('input', {bubbles:true, inputType:'insertText', data:text})); } catch (_) { editor.dispatchEvent(new Event('input', {bubbles:true})); }
  await wait(900);
  const scope = editor.closest('[role="dialog"], form') || document;
  const postButton = Array.from(scope.querySelectorAll('div[role="button"], button, [aria-label]')).filter(visible).reverse().find(el => {
    const label = labelOf(el);
    return hasAny(label, postKeys) && !label.includes('comment') && !label.includes('cancel') && el.getAttribute('aria-disabled') !== 'true' && !el.disabled;
  });
  if (!postButton || !clickLikeUser(postButton)) return 'post_submit_not_found';
  await wait(1500);
  return 'sent_group_post';
})()`, jsString(content))
}

func firstNonEmptyStringSlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func localExtractPostsJS(limit int) string {
	return fmt.Sprintf(`
(() => {
  const out = [];
  const seen = new Set();
  const roots = Array.from(document.querySelectorAll('[role="article"], [role="feed"] > div, div[data-pagelet^="FeedUnit_"]'));
  for (const el of roots) {
    if (out.length >= %d) break;
    const text = (el.innerText || '').trim();
    if (!text || text.length < 30) continue;
    const messageEl = el.querySelector('[data-ad-comet-preview="message"], [data-ad-preview="message"]');
    const content = ((messageEl && messageEl.innerText) || text).trim().slice(0, 4000);
    if (!content || content.length < 20) continue;
    const postLink = Array.from(el.querySelectorAll('a[href]')).find(a => {
      const href = a.href || '';
      return href.includes('/posts/') || href.includes('story_fbid') || href.includes('/permalink/');
    });
    const authorLink = Array.from(el.querySelectorAll('a[href]')).find(a => {
      const href = a.href || '';
      const label = (a.getAttribute('aria-label') || a.innerText || '').trim();
      return label && href.includes('facebook.com') && !href.includes('/groups/');
    });
    const sourceURL = postLink ? postLink.href : location.href;
    const id = sourceURL || content.slice(0, 80);
    if (seen.has(id)) continue;
    seen.add(id);
    let reactions = 0, comments = 0, shares = 0;
    for (const node of Array.from(el.querySelectorAll('span, div[aria-label]'))) {
      const label = ((node.getAttribute && node.getAttribute('aria-label')) || node.innerText || '').toLowerCase();
      const n = parseInt(label.replace(/[^0-9]/g, '') || '0', 10);
      if (!n) continue;
      if (label.includes('reaction') || label.includes('like') || label.includes('thÃ­ch')) reactions = Math.max(reactions, n);
      if (label.includes('comment') || label.includes('bÃ¬nh luáº­n')) comments = Math.max(comments, n);
      if (label.includes('share') || label.includes('chia sáº»')) shares = Math.max(shares, n);
    }
    out.push({
      id,
      source_url: sourceURL,
      author_profile_url: authorLink ? authorLink.href : '',
      author_name: authorLink ? ((authorLink.getAttribute('aria-label') || authorLink.innerText || '').trim()) : '',
      content,
      reactions,
      comments,
      shares
    });
  }
  return JSON.stringify(out);
})()
`, limit)
}

func localExtractGroupsJS(limit int) string {
	return fmt.Sprintf(`
(() => {
  const out = [];
  const seen = new Set();
  const anchors = Array.from(document.querySelectorAll('a[href*="/groups/"]'));
  for (const a of anchors) {
    if (out.length >= %d) break;
    const href = a.href || '';
    if (!href || seen.has(href)) continue;
    const name = (a.innerText || a.getAttribute('aria-label') || '').trim();
    if (!name || name.length < 3) continue;
    seen.add(href);
    const card = a.closest('[role="article"], div') || a.parentElement;
    const text = ((card && card.innerText) || name).trim().slice(0, 2000);
    out.push({
      id: href,
      source_url: href,
      author_profile_url: href,
      author_name: name,
      content: text || name,
      reactions: 0,
      comments: 0,
      shares: 0
    });
  }
  return JSON.stringify(out);
})()
`, limit)
}
