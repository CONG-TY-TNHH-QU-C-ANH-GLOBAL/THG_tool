package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRT is an http.RoundTripper that counts calls and returns a canned response,
// so UniversalClassify caching can be proven without a real OpenAI call.
type fakeRT struct {
	mu     sync.Mutex
	calls  int
	status int
	body   string
}

func (f *fakeRT) RoundTrip(_ *http.Request) (*http.Response, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return &http.Response{
		StatusCode: f.status,
		Body:       io.NopCloser(strings.NewReader(f.body)),
		Header:     make(http.Header),
	}, nil
}

func (f *fakeRT) n() int { f.mu.Lock(); defer f.mu.Unlock(); return f.calls }

// openAIBody wraps a message content string into an OpenAI chat-completion envelope.
func openAIBody(content string, withUsage bool) string {
	env := map[string]any{
		"choices": []map[string]any{{"message": map[string]any{"content": content}}},
	}
	if withUsage {
		env["usage"] = map[string]any{"prompt_tokens": 100, "completion_tokens": 20, "total_tokens": 120}
	}
	b, _ := json.Marshal(env)
	return string(b)
}

func validClassifierContent() string {
	return `{"score":0.9,"intent":"potential_customer","reason":"needs fulfillment","priority":"hot"}`
}

func mgWith(rt *fakeRT) *MessageGenerator {
	mg := NewMessageGenerator("test-key", "gpt-test")
	mg.client = &http.Client{Transport: rt}
	return mg
}

func sampleProfile(industry string) *BusinessProfile {
	return ProfileFromContext(map[string]string{"business_industry": industry})
}

// ── end-to-end UniversalClassify caching ──────────────────────────────────────

func TestUniversalClassify_CacheHitAvoidsSecondCall(t *testing.T) {
	setClassifierCacheForTest(newClassifierCache(true, time.Hour, 100))
	rt := &fakeRT{status: 200, body: openAIBody(validClassifierContent(), true)}
	mg := mgWith(rt)
	prof := sampleProfile("POD fulfillment")
	intent := ClassifyIntent{UserPrompt: "tim khach co nhu cau fulfill"}

	r1, err := mg.UniversalClassify(context.Background(), "Need fulfillment for my shop", "Alice", prof, intent)
	if err != nil {
		t.Fatalf("first classify: %v", err)
	}
	r2, err := mg.UniversalClassify(context.Background(), "Need fulfillment for my shop", "Alice", prof, intent)
	if err != nil {
		t.Fatalf("second classify: %v", err)
	}
	if rt.n() != 1 {
		t.Fatalf("expected 1 OpenAI call (2nd served from cache), got %d", rt.n())
	}
	if r1.Intent != r2.Intent || r1.Score != r2.Score {
		t.Errorf("cached result differs: %+v vs %+v", r1, r2)
	}
}

func TestUniversalClassify_DifferentProfileMissesCache(t *testing.T) {
	setClassifierCacheForTest(newClassifierCache(true, time.Hour, 100))
	rt := &fakeRT{status: 200, body: openAIBody(validClassifierContent(), true)}
	mg := mgWith(rt)
	intent := ClassifyIntent{UserPrompt: "x"}

	if _, err := mg.UniversalClassify(context.Background(), "same post", "Bob", sampleProfile("POD"), intent); err != nil {
		t.Fatal(err)
	}
	if _, err := mg.UniversalClassify(context.Background(), "same post", "Bob", sampleProfile("recruitment"), intent); err != nil {
		t.Fatal(err)
	}
	if rt.n() != 2 {
		t.Fatalf("different business profile must miss cache → 2 calls, got %d", rt.n())
	}
}

func TestUniversalClassify_InvalidResultNotCached(t *testing.T) {
	setClassifierCacheForTest(newClassifierCache(true, time.Hour, 100))
	rt := &fakeRT{status: 200, body: openAIBody("this is not valid json", true)}
	mg := mgWith(rt)
	prof := sampleProfile("POD")

	if _, err := mg.UniversalClassify(context.Background(), "p", "A", prof, ClassifyIntent{}); err == nil {
		t.Fatal("expected error on invalid LLM JSON (fail-closed)")
	}
	// Second call must hit the LLM again — the error must NOT have been cached.
	_, _ = mg.UniversalClassify(context.Background(), "p", "A", prof, ClassifyIntent{})
	if rt.n() != 2 {
		t.Fatalf("invalid result must not be cached → 2 calls, got %d", rt.n())
	}
	if c := getClassifierCache(); c.Len() != 0 {
		t.Fatalf("invalid result must not populate cache, len=%d", c.Len())
	}
}

func TestUniversalClassify_HTTPErrorNotCached(t *testing.T) {
	setClassifierCacheForTest(newClassifierCache(true, time.Hour, 100))
	rt := &fakeRT{status: 500, body: `{"error":"upstream"}`}
	mg := mgWith(rt)
	prof := sampleProfile("POD")

	if _, err := mg.UniversalClassify(context.Background(), "p", "A", prof, ClassifyIntent{}); err == nil {
		t.Fatal("expected error on HTTP 500 (fail-closed)")
	}
	_, _ = mg.UniversalClassify(context.Background(), "p", "A", prof, ClassifyIntent{})
	if rt.n() != 2 {
		t.Fatalf("HTTP error must not be cached → 2 calls, got %d", rt.n())
	}
}

func TestUniversalClassify_CacheDisabledAlwaysCalls(t *testing.T) {
	setClassifierCacheForTest(newClassifierCache(false, time.Hour, 100))
	rt := &fakeRT{status: 200, body: openAIBody(validClassifierContent(), true)}
	mg := mgWith(rt)
	prof := sampleProfile("POD")

	_, _ = mg.UniversalClassify(context.Background(), "p", "A", prof, ClassifyIntent{})
	_, _ = mg.UniversalClassify(context.Background(), "p", "A", prof, ClassifyIntent{})
	if rt.n() != 2 {
		t.Fatalf("disabled cache must call every time → 2 calls, got %d", rt.n())
	}
}
