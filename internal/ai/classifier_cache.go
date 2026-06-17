package ai

// Phase-1 LLM cost control: a process-local, bounded, TTL'd EXACT result cache for
// the classifier (UniversalClassify — the highest-volume burner, one call per
// crawled post). A re-classified identical post (reclassify endpoint, re-crawl of
// the same group surfacing the same post) is served from cache instead of paying
// for a second OpenAI call.
//
// Safety doctrine: cache only VALIDATED successful results; never cache errors,
// refusals, or invalid JSON; fail-closed is preserved (a miss/error falls through
// to the normal live call / error return). Process-local limitation (multi-pod): a
// hit on one process is a miss on another — acceptable for Phase 1; a distributed
// cache is a separate follow-up. Structured usage logging lives in
// classifier_usage_log.go.

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type classifierCacheEntry struct {
	result    UniversalClassifyResult
	expiresAt time.Time
}

type classifierCache struct {
	mu      sync.Mutex
	enabled bool
	max     int
	ttl     time.Duration
	entries map[string]classifierCacheEntry
}

func newClassifierCache(enabled bool, ttl time.Duration, max int) *classifierCache {
	if max < 1 {
		max = 1
	}
	return &classifierCache{enabled: enabled, max: max, ttl: ttl, entries: make(map[string]classifierCacheEntry)}
}

func (c *classifierCache) Enabled() bool { return c != nil && c.enabled }

// Get returns a COPY of the stored result (UniversalClassifyResult is a value type
// with no reference fields, so the caller cannot mutate the cached entry).
func (c *classifierCache) Get(key string, now time.Time) (UniversalClassifyResult, bool) {
	if !c.Enabled() {
		return UniversalClassifyResult{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return UniversalClassifyResult{}, false
	}
	if now.After(e.expiresAt) {
		delete(c.entries, key) // opportunistic expiry on read
		return UniversalClassifyResult{}, false
	}
	return e.result, true
}

// Set stores a validated result. Bounded eviction: when at capacity it evicts
// entries (effectively random — Go map iteration is randomized) until back under
// max, deleting exactly one per iteration. In steady state this removes a single
// entry, so Set NEVER holds the lock for an O(max) full-table scan — keeping the
// critical section tiny under high classifier traffic. TTL correctness is enforced
// by Get (it never returns an expired entry and deletes it on read); expired
// entries that are never re-read are reclaimed here under capacity pressure. No LRU
// and no background goroutine: bounded memory + a minimal critical section are
// preferred over ideal eviction for Phase 1.
func (c *classifierCache) Set(key string, result UniversalClassifyResult, now time.Time) {
	if !c.Enabled() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// Evict down to max-1 so the new entry fits without exceeding the bound.
	for len(c.entries) >= c.max {
		evicted := false
		for k := range c.entries {
			delete(c.entries, k)
			evicted = true
			break
		}
		if !evicted {
			break // map already empty (defensive; cannot happen while len >= max >= 1)
		}
	}
	c.entries[key] = classifierCacheEntry{result: result, expiresAt: now.Add(c.ttl)}
}

func (c *classifierCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// classifierCacheKey hashes EVERY input that can change the classifier output: the
// model, the EXACT composed prompt (which already embeds the business-profile
// block, the per-crawl intent block, the author, the language rule, AND the fixed
// instruction template — so prompt drift, profile/org change, and intent all
// invalidate automatically), and the JSON schema. Full SHA-256 digest; exact match
// (no normalization → no cache poisoning). Hashing the composed prompt is strictly
// stronger than enumerating fields because no influencing input can be forgotten.
func classifierCacheKey(model, prompt, schemaJSON string) string {
	h := sha256.New()
	h.Write([]byte("clf-v1\x1f"))
	h.Write([]byte(model))
	h.Write([]byte{0x1f})
	h.Write([]byte(prompt))
	h.Write([]byte{0x1f})
	h.Write([]byte(schemaJSON))
	return hex.EncodeToString(h.Sum(nil))
}

// keyHashPrefix is the only part of the key safe to log.
func keyHashPrefix(key string) string {
	if len(key) > 12 {
		return key[:12]
	}
	return key
}

// ── env-configured process singleton (+ test seam) ───────────────────────────

var (
	clfCacheMu sync.Mutex
	clfCache   *classifierCache
)

func getClassifierCache() *classifierCache {
	clfCacheMu.Lock()
	defer clfCacheMu.Unlock()
	if clfCache == nil {
		clfCache = newClassifierCache(
			clfEnvBool("LLM_CLASSIFIER_CACHE_ENABLED", true),
			time.Duration(clfEnvInt("LLM_CLASSIFIER_CACHE_TTL_SECONDS", 21600))*time.Second, // 6h default
			clfEnvInt("LLM_CLASSIFIER_CACHE_MAX_ENTRIES", 5000),
		)
	}
	return clfCache
}

// setClassifierCacheForTest installs a cache instance (test seam only).
func setClassifierCacheForTest(c *classifierCache) {
	clfCacheMu.Lock()
	defer clfCacheMu.Unlock()
	clfCache = c
}

func clfEnvBool(k string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func clfEnvInt(k string, def int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}
