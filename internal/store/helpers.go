package store

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
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

// isSQLiteBusy returns true when err looks like SQLite's
// SQLITE_BUSY / SQLITE_LOCKED. modernc.org/sqlite returns these as
// "database is locked (5) (SQLITE_BUSY)" or similar; we also catch
// SQLITE_BUSY_SNAPSHOT which fires when a deferred-tx upgrade races
// a concurrent committed writer.
//
// Why this is needed: in WAL mode, a default `BEGIN` is DEFERRED —
// the connection becomes a reader at some snapshot, then must
// upgrade to writer on first INSERT. If another writer committed
// between BEGIN and INSERT, the upgrade fails with SQLITE_BUSY
// regardless of busy_timeout. The fix is either BEGIN IMMEDIATE
// (driver-specific to enable) or retry-on-busy. We use retry so the
// store stays portable across drivers.
func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "database is locked") ||
		strings.Contains(s, "sqlite_busy") ||
		strings.Contains(s, "sqlite_locked") ||
		strings.Contains(s, "database table is locked")
}

// retryOnBusy executes fn up to maxAttempts times, sleeping between
// attempts with jittered exponential backoff when fn returns a
// SQLite busy error. Non-busy errors short-circuit immediately.
//
// Default backoff: 10ms → 20ms → 40ms → 80ms → 160ms → 320ms → 640ms
// (totaling ~1.3s of waiting on top of the 5s busy_timeout configured
// in the DSN). 7 attempts is generous enough that a CI machine under
// load won't flake even with 16+ concurrent writers.
func retryOnBusy(maxAttempts int, fn func() error) error {
	if maxAttempts <= 0 {
		maxAttempts = 7
	}
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		if !isSQLiteBusy(err) {
			return err
		}
		lastErr = err
		// Exponential backoff with ~25% jitter so a thundering herd
		// of contenders doesn't sync up on identical retry windows.
		base := 10 * (1 << attempt) // 10, 20, 40, 80, 160, 320, 640 ms
		jitter := rand.Intn(base/4 + 1) //nolint:gosec // non-crypto jitter
		time.Sleep(time.Duration(base+jitter) * time.Millisecond)
	}
	return lastErr
}
