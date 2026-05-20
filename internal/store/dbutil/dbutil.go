// Package dbutil holds pure DB helpers shared across every domain
// subpackage of internal/store/. Foundational layer — sits at the
// bottom of the dependency graph (see internal/store/DOMAINS.md §1).
//
// Phase 1 of STORE_SUBPACKAGE_REFACTOR (2026-05-21) extracted these
// helpers out of the god-package so every future subpackage extraction
// can depend on a stable set of utilities without fighting cycles.
//
// Locked invariants (per feedback_store_subpackage_locks):
//
//   - **No business logic here.** Anything that touches a domain table
//     (outbound_messages, leads, knowledge_assets, ...) lives in its
//     own subpackage, not here.
//   - **No `*Store` methods.** dbutil is the lowest layer; it cannot
//     import store. The store package (and every subpackage) imports
//     dbutil, never the other way around.
//   - **Cycle-free by construction.** If a helper would need to import
//     a domain package, it doesn't belong here — promote it to the
//     domain that owns its concern.
package dbutil

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// ParseSQLiteTime parses a SQLite-formatted timestamp string into a
// Go time.Time. SQLite stores TIMESTAMP columns as TEXT in several
// layouts (CURRENT_TIMESTAMP yields "YYYY-MM-DD HH:MM:SS"; explicit
// values may use RFC3339). Returns the zero Time on parse failure
// so callers can compare against IsZero() without branching on err.
func ParseSQLiteTime(value string) time.Time {
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

// IsSQLiteBusy reports whether err looks like SQLite's SQLITE_BUSY /
// SQLITE_LOCKED. modernc.org/sqlite returns these as "database is
// locked (5) (SQLITE_BUSY)" or similar; we also catch
// SQLITE_BUSY_SNAPSHOT which fires when a deferred-tx upgrade races
// a concurrent committed writer.
//
// In WAL mode, a default `BEGIN` is DEFERRED — the connection becomes
// a reader at some snapshot, then must upgrade to writer on first
// INSERT. If another writer committed between BEGIN and INSERT, the
// upgrade fails with SQLITE_BUSY regardless of busy_timeout. The fix
// is either BEGIN IMMEDIATE (driver-specific) or retry-on-busy. We
// use retry so the store stays portable across drivers.
func IsSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "database is locked") ||
		strings.Contains(s, "sqlite_busy") ||
		strings.Contains(s, "sqlite_locked") ||
		strings.Contains(s, "database table is locked")
}

// RetryOnBusy executes fn up to maxAttempts times, sleeping between
// attempts with jittered exponential backoff when fn returns a SQLite
// busy error. Non-busy errors short-circuit immediately.
//
// Default backoff: 10ms → 20ms → 40ms → 80ms → 160ms → 320ms → 640ms
// (totaling ~1.3s of waiting on top of the 5s busy_timeout configured
// in the DSN). 7 attempts is generous enough that a CI machine under
// load won't flake even with 16+ concurrent writers.
func RetryOnBusy(maxAttempts int, fn func() error) error {
	if maxAttempts <= 0 {
		maxAttempts = 7
	}
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		if !IsSQLiteBusy(err) {
			return err
		}
		lastErr = err
		// Exponential backoff with ~25% jitter so a thundering herd of
		// contenders doesn't sync up on identical retry windows.
		base := 10 * (1 << attempt)     // 10, 20, 40, 80, 160, 320, 640 ms
		jitter := rand.Intn(base/4 + 1) //nolint:gosec // non-crypto jitter
		time.Sleep(time.Duration(base+jitter) * time.Millisecond)
	}
	return lastErr
}

// UTCDayKey formats a time as the "YYYY-MM-DD" UTC day stamp used for
// per-day counter reset logic (account_runtime_state, ledger rollovers).
// Centralised so every callsite agrees on the day boundary — midnight UTC,
// no local-timezone drift.
func UTCDayKey(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

// BoolToInt converts a Go bool to the SQLite-friendly 0/1 int we store
// for "boolean" columns (SQLite has no native bool type — convention is
// INTEGER 0/1). Standard 1-liner used across many tables.
func BoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// DedupHash generates a compound deduplication hash from the listed
// fields. The returned value is a 32-char hex string (the first 16
// bytes of a SHA-256 digest). Used as the UNIQUE column for posts,
// comments, and similar dedup-by-content tables.
func DedupHash(platform, contentType, url, author, date, content string) string {
	raw := fmt.Sprintf("%s|%s|%s|%s|%s|%s", platform, contentType, url, author, date, content)
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h[:16]) // 32-char hex
}
