package soak

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// FakeOpenAI is a faithful HTTP server that emulates OpenAI's
// /v1/embeddings endpoint. It exists so the soak can exercise the
// REAL [embedding.OpenAIEmbedder] HTTP code path — serialisation,
// headers, status-code branching, response parsing — without
// hitting api.openai.com from CI.
//
// Faithful means:
//
//   - Same URL shape: /v1/embeddings
//   - Same request body: {"model": "...", "input": [...]}
//   - Same response body: {"data": [{"index": i, "embedding": [...]}], "usage": {"total_tokens": N}}
//   - Same status-code semantics: 200 success, 429 rate limit, 5xx
//     transient, 4xx permanent
//
// The fake server is deterministic — same input always yields the
// same embedding (hash-derived) so soak retries are stable.
//
// Failure injection knobs:
//
//   - FailureRate429: probability per request to respond with 429.
//     Lets the soak prove rate-limit retry behaviour without
//     flooding a real API.
//   - FailureRate5xx: probability per request to respond with 500.
//   - DelayPerRequest: latency injected per call. Latency-distribution
//     measurements use this.
type FakeOpenAI struct {
	server *httptest.Server
	mu     sync.Mutex

	// Counters atomic so concurrent soaks can read without locking.
	requestCount   atomic.Int64
	tokensServed   atomic.Int64
	failures429    atomic.Int64
	failures5xx    atomic.Int64
	successes      atomic.Int64
	bytesIn        atomic.Int64

	// Configuration — set BEFORE Start, frozen after.
	FailureRate429   float64
	FailureRate5xx   float64
	DelayPerRequest  time.Duration
	Dimensions       int
}

// NewFakeOpenAI constructs a fake server. Call Start before use.
func NewFakeOpenAI() *FakeOpenAI {
	return &FakeOpenAI{Dimensions: 1536}
}

// Start binds the server to a random local port and returns the
// base URL the embedder should target.
func (f *FakeOpenAI) Start() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.server != nil {
		return f.server.URL
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/embeddings", f.handleEmbeddings)
	f.server = httptest.NewServer(mux)
	return f.server.URL
}

// Close shuts the server down. Idempotent.
func (f *FakeOpenAI) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.server != nil {
		f.server.Close()
		f.server = nil
	}
}

// RequestCount reports how many embedding requests the soak issued.
// Used by cost-telemetry assertions ("we expected ~17 embeddings
// for a 17-asset catalog, but the worker issued 23 — why?").
func (f *FakeOpenAI) RequestCount() int64 { return f.requestCount.Load() }
func (f *FakeOpenAI) TokensServed() int64 { return f.tokensServed.Load() }
func (f *FakeOpenAI) Failures429() int64  { return f.failures429.Load() }
func (f *FakeOpenAI) Failures5xx() int64  { return f.failures5xx.Load() }
func (f *FakeOpenAI) Successes() int64    { return f.successes.Load() }

func (f *FakeOpenAI) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	f.requestCount.Add(1)

	if f.DelayPerRequest > 0 {
		time.Sleep(f.DelayPerRequest)
	}

	// Decide failure mode for THIS request. Failure rates are
	// independent; if both fire, 429 wins (rate limit is the more
	// realistic primary failure mode at scale).
	if pseudoRandUnit(f.requestCount.Load()) < f.FailureRate429 {
		f.failures429.Add(1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"rate_limit_exceeded","message":"rate limit"}}`))
		return
	}
	if pseudoRandUnit(f.requestCount.Load()+1_000_000) < f.FailureRate5xx {
		f.failures5xx.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"type":"server_error"}}`))
		return
	}

	// Parse the embedding request.
	var req struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.Model == "" || len(req.Input) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Build deterministic embeddings using the same ClusteredEmbedder
	// math, scaled up to OpenAI's 1536 dims (the production wire
	// shape). The soak's measurements thus reflect production-shaped
	// payload sizes and JSON serialisation costs.
	emb := NewClusteredEmbedder()
	type embeddingEntry struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
		Object    string    `json:"object"`
	}
	type usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	}
	type response struct {
		Object string           `json:"object"`
		Data   []embeddingEntry `json:"data"`
		Model  string           `json:"model"`
		Usage  usage            `json:"usage"`
	}
	out := response{Object: "list", Model: req.Model}
	totalTokens := 0
	for i, text := range req.Input {
		vec := emb.embedOne(text)
		// Pad to 1536 dims with zeros so the response matches the
		// production model shape. Real OpenAI returns 1536-element
		// arrays for text-embedding-3-small.
		if len(vec) < f.Dimensions {
			padded := make([]float32, f.Dimensions)
			copy(padded, vec)
			vec = padded
		}
		out.Data = append(out.Data, embeddingEntry{
			Index:     i,
			Embedding: vec,
			Object:    "embedding",
		})
		// Realistic token estimate: ~4 chars per token (OpenAI's
		// rough rule of thumb for English).
		tokens := max(len(strings.TrimSpace(text))/4, 1)
		totalTokens += tokens
	}
	out.Usage = usage{PromptTokens: totalTokens, TotalTokens: totalTokens}
	f.tokensServed.Add(int64(totalTokens))
	f.successes.Add(1)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

// pseudoRandUnit returns a deterministic-from-seed pseudo-random
// float in [0, 1). Used by the fake server to inject failures at
// the configured rate without needing math/rand state — same
// request count always yields the same outcome.
//
// We're not doing cryptography here; correlation between
// consecutive calls is fine.
func pseudoRandUnit(seed int64) float64 {
	// Simple LCG.
	x := uint64(seed*1664525 + 1013904223)
	return float64(x%10000) / 10000.0
}
