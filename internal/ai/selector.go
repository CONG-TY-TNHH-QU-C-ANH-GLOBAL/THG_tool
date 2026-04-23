package ai

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"
)

// SelectorAI dùng AI để tìm CSS selector phù hợp từ HTML thực tế của trang.
// Kết quả được cache theo (purpose, htmlHash) để tránh gọi AI nhiều lần cho cùng cấu trúc.
type SelectorAI struct {
	mg    *MessageGenerator
	cache sync.Map // key: purpose+":"+htmlHash → selector string
}

// NewSelectorAI tạo instance mới.
func NewSelectorAI(mg *MessageGenerator) *SelectorAI {
	return &SelectorAI{mg: mg}
}

// FindSelector gửi đoạn HTML cho AI và nhờ AI trả về CSS selector phù hợp với mục đích.
// purpose ví dụ: "photo attachment button in comment toolbar"
// html: outerHTML của vùng cần tìm (comment form, toolbar…)
// Trả về selector string (ví dụ `div[aria-label="Đính kèm ảnh"]`) hoặc "" nếu không tìm được.
func (s *SelectorAI) FindSelector(ctx context.Context, purpose, html string) string {
	if s.mg == nil || !s.mg.Available() {
		return ""
	}

	// Simple hash key: dùng độ dài + 64 ký tự đầu để phân biệt cấu trúc trang
	cacheKey := purpose + ":" + quickHash(html)
	if cached, ok := s.cache.Load(cacheKey); ok {
		sel := cached.(string)
		log.Printf("[SelectorAI] Cache hit for %q → %s", purpose, sel)
		return sel
	}

	// Giới hạn HTML gửi cho AI (tránh token quá lớn)
	trimmed := html
	if len(trimmed) > 6000 {
		trimmed = trimmed[:6000] + "...[truncated]"
	}

	prompt := `You are a web automation expert. Given the following HTML fragment from a Facebook page, return ONLY the single best CSS selector to find: ` + purpose + `

Rules:
- Return ONLY the CSS selector string, nothing else, no explanation, no markdown
- Prefer aria-label, role, or data attributes over class names (classes change often)
- The selector must work with document.querySelector()
- If you cannot find a suitable element, return the exact string: NOT_FOUND

HTML:
` + trimmed

	ctxTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	result, err := s.mg.callOpenAI(ctxTimeout, prompt)
	if err != nil {
		log.Printf("[SelectorAI] AI error for %q: %v", purpose, err)
		return ""
	}

	sel := strings.TrimSpace(result)
	sel = strings.Trim(sel, "`\"'")
	if sel == "NOT_FOUND" || sel == "" {
		log.Printf("[SelectorAI] AI could not find selector for %q", purpose)
		return ""
	}

	log.Printf("[SelectorAI] AI found selector for %q → %s", purpose, sel)
	s.cache.Store(cacheKey, sel)
	return sel
}

// quickHash tạo chuỗi ngắn để làm cache key từ HTML.
func quickHash(s string) string {
	if len(s) > 128 {
		s = s[:128]
	}
	return s
}
