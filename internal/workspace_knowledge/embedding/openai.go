package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// OpenAIEmbedder is the production [Embedder] backed by the OpenAI
// embeddings API. Construct via [NewOpenAIEmbedder].
//
// Operational notes:
//
//   - Default model is "text-embedding-3-small" (1536 dimensions,
//     $0.02 / 1M tokens at time of writing). text-embedding-3-large
//     is 3072 dimensions and 6.5x more expensive — only switch when
//     evaluation on the workspace's actual catalog shows quality
//     gains worth the cost.
//
//   - Single HTTP client reused across calls. We do NOT pool — the
//     net/http client pools connections internally.
//
//   - Per-batch timeout 30s. OpenAI's published p99 for
//     text-embedding-3-small is ~2s for a batch of 32. 30s is the
//     ceiling that distinguishes "API is slow" from "API has died".
//
//   - Auth: API key from constructor, NOT from env at request time.
//     Boot-time configuration. Failure during a batch reads exclusively
//     from the cached key.
type OpenAIEmbedder struct {
	apiKey     string
	model      string
	dimensions int
	httpClient *http.Client
	baseURL    string // overridable for tests
}

// NewOpenAIEmbedder builds an embedder. apiKey is required; model
// defaults to "text-embedding-3-small" when blank.
//
// The returned embedder is safe for concurrent use across many
// workers. There is no per-instance state beyond the immutable
// config above.
func NewOpenAIEmbedder(apiKey, model string) *OpenAIEmbedder {
	if model == "" {
		model = "text-embedding-3-small"
	}
	dims := dimensionsFor(model)
	return &OpenAIEmbedder{
		apiKey:     apiKey,
		model:      model,
		dimensions: dims,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    "https://api.openai.com/v1",
	}
}

// dimensionsFor returns the published vector size for known OpenAI
// embedding models. Unknown models default to 1536 — the most likely
// dim — but we surface a panic-prone wrong value at boot via the
// Embedder.Dimensions vs VECTOR(N) check in the worker.
func dimensionsFor(model string) int {
	switch model {
	case "text-embedding-3-small":
		return 1536
	case "text-embedding-3-large":
		return 3072
	case "text-embedding-ada-002":
		return 1536
	default:
		return 1536
	}
}

// ModelVersion satisfies Embedder. Composed from provider + model + a
// schema-revision suffix. Bump the v-suffix only when our embedding
// INPUT format changes (e.g. we start including a new field in
// BuildInputText) — then existing embeddings need re-backfill even
// though the OpenAI model itself didn't change.
func (e *OpenAIEmbedder) ModelVersion() string {
	return "openai:" + e.model + ":v1"
}

func (e *OpenAIEmbedder) Dimensions() int { return e.dimensions }

// SetBaseURL retargets the embedder at a different API endpoint.
// Test-only escape hatch — the production embedder always points at
// api.openai.com. Used by [internal/workspace_knowledge/soak] to
// drive the REAL HTTP code path against a faithful fake server,
// proving serialisation, headers, and error-classification work
// without spending real OpenAI budget on every CI run.
//
// Callers in non-test code paths that need to retarget (e.g.
// Azure OpenAI Service deployments) should construct a new
// embedder via a future NewOpenAIEmbedderWithBase rather than
// mutating an already-constructed instance.
func (e *OpenAIEmbedder) SetBaseURL(url string) {
	e.baseURL = url
}

// Embed posts the batch to /v1/embeddings and returns vectors in
// input order. Errors are wrapped as recoverable / permanent so the
// worker can decide retry policy.
func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if e.apiKey == "" {
		return nil, WrapPermanent(errors.New("openai embedder: API key missing"))
	}
	if len(texts) == 0 {
		return nil, nil
	}
	// Defensive: empty strings produce garbage embeddings AND OpenAI
	// rejects the request. The worker should never pass empty in,
	// but enforce here too so a bug in BuildInputText fails loudly.
	for i, t := range texts {
		if t == "" {
			return nil, WrapPermanent(fmt.Errorf("openai embedder: text at index %d is empty", i))
		}
	}

	body, err := json.Marshal(map[string]any{
		"model": e.model,
		"input": texts,
	})
	if err != nil {
		return nil, WrapPermanent(fmt.Errorf("encode request: %w", err))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, WrapPermanent(fmt.Errorf("build request: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		// Network errors are recoverable — DNS hiccup, TCP reset,
		// transient proxy issue. The worker retries.
		return nil, WrapRecoverable(fmt.Errorf("openai embedder: http: %w", err))
	}
	defer resp.Body.Close()

	if statusErr := embedHTTPStatusError(resp); statusErr != nil {
		return nil, statusErr
	}
	return parseEmbeddingVectors(resp, len(texts), e.dimensions)
}
