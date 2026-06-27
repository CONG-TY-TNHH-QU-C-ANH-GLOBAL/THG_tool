package server

import (
	"log"
	"os"
	"strings"
)

// loadMarketingGuide reads the brand marketing playbook file used to ground the reel script
// prompt. It degrades honestly: an unset path or a read error returns "" (no extra grounding)
// and logs once, so reel creation never fails just because the notes file is missing.
func loadMarketingGuide(path string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		log.Printf("[reel] marketing guide %q unreadable, skipping grounding: %v", p, err)
		return ""
	}
	return strings.TrimSpace(string(b))
}
