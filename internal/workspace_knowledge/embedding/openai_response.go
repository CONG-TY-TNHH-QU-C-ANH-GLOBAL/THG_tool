package embedding

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// embedHTTPStatusError maps a non-2xx OpenAI response to the wrapped recoverable
// (5xx / 429) or permanent (other 4xx) error the worker uses for retry policy.
// Returns nil for an acceptable status. Same classification as the former inline
// checks; order preserved (5xx, then 429, then other 4xx).
func embedHTTPStatusError(resp *http.Response) error {
	switch {
	case resp.StatusCode >= 500:
		return WrapRecoverable(fmt.Errorf("openai embedder: HTTP %d", resp.StatusCode))
	case resp.StatusCode == 429:
		// Rate limit — recoverable but with operator visibility. The
		// worker's exponential backoff will accommodate.
		return WrapRecoverable(fmt.Errorf("openai embedder: rate limited (HTTP 429)"))
	case resp.StatusCode >= 400:
		// 4xx other than rate-limit: auth, malformed payload, model
		// not available, etc. Permanent — operator must intervene.
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return WrapPermanent(fmt.Errorf("openai embedder: HTTP %d: %s", resp.StatusCode, string(bodyBytes)))
	default:
		return nil
	}
}

// parseEmbeddingVectors decodes the embeddings response and returns vectors in
// input order (OpenAI does not guarantee order), validating count + dimensions.
// expected is len(texts); dimensions is the embedder's configured vector size.
func parseEmbeddingVectors(resp *http.Response, expected, dimensions int) ([][]float32, error) {
	var parsed struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, WrapRecoverable(fmt.Errorf("decode response: %w", err))
	}
	if len(parsed.Data) != expected {
		return nil, WrapPermanent(fmt.Errorf("openai embedder: response had %d embeddings, expected %d", len(parsed.Data), expected))
	}

	out := make([][]float32, expected)
	for _, d := range parsed.Data {
		if d.Index < 0 || d.Index >= len(out) {
			return nil, WrapPermanent(fmt.Errorf("openai embedder: bad index %d", d.Index))
		}
		if len(d.Embedding) != dimensions {
			return nil, WrapPermanent(fmt.Errorf("openai embedder: vector at index %d has %d dims, expected %d",
				d.Index, len(d.Embedding), dimensions))
		}
		out[d.Index] = d.Embedding
	}
	for i := range out {
		if out[i] == nil {
			return nil, WrapPermanent(fmt.Errorf("openai embedder: missing embedding for index %d", i))
		}
	}
	return out, nil
}
