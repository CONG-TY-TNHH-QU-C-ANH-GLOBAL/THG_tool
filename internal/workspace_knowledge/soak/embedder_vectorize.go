package soak

// This file owns the ClusteredEmbedder vectorisation algorithm:
// tokenisation, cluster scoring, hash-noise axes, and L2 normalisation.
// The type and its cluster taxonomy live in embedder.go.

import (
	"context"
	"crypto/sha1"
	"encoding/binary"
	"math"
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/embedding"
)

// ModelVersion satisfies the Embedder contract. Distinct from any
// OpenAI model so persistence rows produced by the soak harness
// are never confused with production rows.
func (e *ClusteredEmbedder) ModelVersion() string {
	return "soak:clustered-pod:v1"
}

func (e *ClusteredEmbedder) Dimensions() int { return e.dim }

// Embed converts each text to a clustered vector + hash noise, then
// L2-normalises. Deterministic for the same input.
func (e *ClusteredEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = e.embedOne(t)
	}
	return out, nil
}

func (e *ClusteredEmbedder) embedOne(text string) []float32 {
	v := make([]float32, e.dim)

	// Tokenise the text — simple lowercase + non-alphanum split.
	tokens := tokenise(text)

	// Cluster contributions.
	for i, c := range e.clusters {
		count := 0
		for _, tok := range tokens {
			if _, ok := c.terms[tok]; ok {
				count++
			}
		}
		if count > 0 {
			// Use sqrt of count so a doc with 5 cat mentions doesn't
			// dominate one with 1. Same idea as TF-IDF's diminishing
			// returns, but simpler.
			v[i] = float32(math.Sqrt(float64(count)))
		}
	}

	// Hash-noise axes — let unrelated texts have orthogonal residue
	// instead of identical zero vectors. SHA-1 over the text, split
	// into 8 float32 axes in [-0.05, 0.05] range. Small enough that
	// it doesn't dominate cluster signal; large enough to prevent
	// degenerate cosine=NaN for empty-cluster texts.
	h := sha1.Sum([]byte(strings.ToLower(text)))
	for j := range 8 {
		// Each axis uses 2 bytes of the hash → uint16 → [-1, 1] → scale.
		raw := binary.BigEndian.Uint16(h[j*2 : j*2+2])
		normalised := float32(raw)/65535.0*2 - 1 // [-1, 1]
		v[len(e.clusters)+j] = normalised * 0.05
	}

	// L2-normalise so cosine similarity behaves cleanly.
	var sumSq float64
	for _, x := range v {
		sumSq += float64(x) * float64(x)
	}
	if sumSq > 0 {
		mag := float32(math.Sqrt(sumSq))
		for i := range v {
			v[i] /= mag
		}
	}
	return v
}

func tokenise(s string) []string {
	out := []string{}
	cur := strings.Builder{}
	flush := func() {
		if cur.Len() >= 2 {
			out = append(out, cur.String())
		}
		cur.Reset()
	}
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

// Compile-time check: ClusteredEmbedder satisfies the Embedder port.
var _ embedding.Embedder = (*ClusteredEmbedder)(nil)
