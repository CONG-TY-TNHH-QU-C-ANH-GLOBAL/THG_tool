package store

import (
	"crypto/sha256"
	"fmt"
)

// DedupHash generates a compound deduplication hash.
func DedupHash(platform, contentType, url, author, date, content string) string {
	raw := fmt.Sprintf("%s|%s|%s|%s|%s|%s", platform, contentType, url, author, date, content)
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h[:16]) // 32-char hex
}
