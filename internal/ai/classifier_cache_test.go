package ai

import (
	"strings"
	"testing"
	"time"
)

// ── key tests ────────────────────────────────────────────────────────────────

func TestClassifierCacheKey_SHA256AndSensitivity(t *testing.T) {
	k := classifierCacheKey("gpt-test", "PROMPT", "SCHEMA")
	if len(k) != 64 {
		t.Fatalf("key must be a full SHA-256 hex digest (64 chars), got %d", len(k))
	}
	if classifierCacheKey("gpt-test", "PROMPT", "SCHEMA") != k {
		t.Fatal("same inputs must produce the same key")
	}
	if classifierCacheKey("other-model", "PROMPT", "SCHEMA") == k {
		t.Error("different model must change the key")
	}
	if classifierCacheKey("gpt-test", "PROMPT-DRIFT", "SCHEMA") == k {
		t.Error("prompt drift must change the key")
	}
	if classifierCacheKey("gpt-test", "PROMPT", "SCHEMA-DRIFT") == k {
		t.Error("schema drift must change the key")
	}
}

func TestClassifierCacheKey_NoRawTextLeak(t *testing.T) {
	raw := "UNIQUE_SECRET_POST_TEXT_12345"
	k := classifierCacheKey("gpt-test", "prompt with "+raw, "schema")
	if strings.Contains(k, raw) {
		t.Error("cache key must not contain raw post text")
	}
	if p := keyHashPrefix(k); len(p) > 12 || strings.Contains(p, raw) {
		t.Errorf("log prefix unsafe: %q", p)
	}
}

// ── pure cache tests ──────────────────────────────────────────────────────────

func TestClassifierCache_TTLExpiry(t *testing.T) {
	c := newClassifierCache(true, 10*time.Second, 10)
	t0 := time.Unix(1_700_000_000, 0)
	c.Set("k", UniversalClassifyResult{Score: 0.5}, t0)
	if _, ok := c.Get("k", t0); !ok {
		t.Fatal("expected hit before TTL")
	}
	if _, ok := c.Get("k", t0.Add(11*time.Second)); ok {
		t.Fatal("expected miss after TTL expiry")
	}
}

func TestClassifierCache_MaxEntriesBounded(t *testing.T) {
	c := newClassifierCache(true, time.Hour, 3)
	now := time.Unix(1_700_000_000, 0)
	for i := 0; i < 50; i++ {
		c.Set(classifierCacheKey("m", "p", string(rune(i))), UniversalClassifyResult{Score: 0.1}, now)
	}
	if got := c.Len(); got > 3 {
		t.Fatalf("cache exceeded max_entries: len=%d > 3", got)
	}
}

func TestClassifierCache_Disabled(t *testing.T) {
	c := newClassifierCache(false, time.Hour, 10)
	now := time.Unix(1_700_000_000, 0)
	c.Set("k", UniversalClassifyResult{Score: 0.9}, now)
	if _, ok := c.Get("k", now); ok {
		t.Fatal("disabled cache must never hit")
	}
}
