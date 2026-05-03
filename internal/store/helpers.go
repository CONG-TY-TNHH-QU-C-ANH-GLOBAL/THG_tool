package store

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

// DedupHash generates a compound deduplication hash.
func DedupHash(platform, contentType, url, author, date, content string) string {
	raw := fmt.Sprintf("%s|%s|%s|%s|%s|%s", platform, contentType, url, author, date, content)
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h[:16]) // 32-char hex
}

func parseSQLiteTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05Z07:00"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return time.Time{}
}
