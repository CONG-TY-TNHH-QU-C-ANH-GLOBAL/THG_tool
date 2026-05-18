// Package soak is the production-soak validation harness for the
// retrieval substrate. It exists for one purpose: PROVE the
// pipeline behaves correctly under realistic load and failure
// conditions BEFORE the team builds orchestration on top.
//
// The harness CANNOT use real OpenAI embeddings in CI — there is no
// API key in test environments, calls cost money, and tests must
// be deterministic. Instead it ships a semantically-clustered mock
// embedder ([ClusteredEmbedder]) that produces realistic similarity
// signals without external dependencies.
//
// In production, the operator swaps ClusteredEmbedder for the real
// OpenAIEmbedder via the CLI flag — same harness, real data.
package soak

import (
	"context"
	"crypto/sha1"
	"encoding/binary"
	"math"
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/embedding"
)

// ClusteredEmbedder produces deterministic vectors that retain
// REALISTIC semantic structure. Texts mentioning "cat" cluster
// together; texts mentioning "shipping" cluster together; texts
// mentioning BOTH have intermediate similarity. This is the
// minimum semantic model that makes a retrieval-quality soak
// meaningful — random vectors would prove "the pipeline runs" but
// not "retrieval surfaces relevant assets".
//
// HOW IT WORKS (kept simple intentionally; full embedding model is
// not the goal — we are validating the SUBSTRATE):
//
//  1. Maintain a list of "cluster terms" — semantic concepts.
//  2. For each text, count how many of its tokens overlap each
//     cluster's term list. That count is the cluster's contribution.
//  3. Map clusters to fixed orthogonal basis vectors (one cluster =
//     one axis in dimension space). Sum the contributions.
//  4. Normalise to unit length so cosine similarity behaves cleanly.
//
// The result: a query about "cat shipping" produces a vector with
// signal on the cat axis AND the shipping axis. An asset about
// "cat tee with US shipping" lights the same two axes — high
// cosine. An asset about "dog mug" lights different axes — low cosine.
//
// Pitfall: this is NOT a real embedding model. It does NOT handle
// synonyms ("anime" ≈ "manga"), language transfer (VI ≈ EN), or
// concept abstraction ("oversized" ≈ "baggy"). The soak measures
// PIPELINE behavior under these limits — not "retrieval works on
// real natural language". For that, swap to OpenAIEmbedder in
// production runs.
type ClusteredEmbedder struct {
	clusters []cluster
	dim      int
}

// cluster pairs a semantic concept with the terms that activate it.
// Each cluster maps to ONE basis dimension.
type cluster struct {
	name  string
	terms map[string]struct{}
}

// NewClusteredEmbedder returns an embedder pre-loaded with clusters
// covering the POD/fulfillment domain — the actual workspace this
// system serves. The cluster list is tuned to make the soak
// realistic: lead about cat tees DOES match asset about cat shirts
// even with no exact-token overlap.
func NewClusteredEmbedder() *ClusteredEmbedder {
	clusterDefs := []struct {
		name  string
		terms []string
	}{
		// Animals / pet niche
		{"cat", []string{"cat", "kitten", "feline", "meo", "mèo"}},
		{"dog", []string{"dog", "puppy", "doggy", "canine", "cho", "chó"}},
		{"pet", []string{"pet", "pets", "animal", "thucung", "thú", "cưng"}},
		// Product / apparel
		{"shirt", []string{"shirt", "tee", "tshirt", "ao", "áo", "thun"}},
		{"hoodie", []string{"hoodie", "sweatshirt", "pullover"}},
		{"mug", []string{"mug", "cup", "ly", "cốc"}},
		// POD / fulfillment specifics
		{"pod", []string{"pod", "printondemand", "custom", "personalised", "personalized"}},
		{"fulfillment", []string{"fulfillment", "fulfilment", "fulfill", "ship", "shipping", "logistics", "warehouse", "kho"}},
		{"wholesale", []string{"wholesale", "bulk", "moq", "sỉ", "si"}},
		// Style / aesthetic
		{"oversized", []string{"oversized", "baggy", "loose", "rộng", "rong"}},
		{"gothic", []string{"gothic", "dark", "edgy", "alt", "alternative"}},
		{"anime", []string{"anime", "manga", "otaku", "japanese", "kawaii"}},
		// Geo / market
		{"us", []string{"us", "usa", "america", "americans", "mỹ", "my"}},
		{"eu", []string{"eu", "europe", "european", "uk", "germany", "france"}},
		// Commerce intent
		{"pricing", []string{"price", "pricing", "cost", "quote", "giá", "gia"}},
		{"policy", []string{"policy", "return", "refund", "warranty", "guarantee", "chính", "sách"}},
		{"cta", []string{"inbox", "dm", "message", "contact", "liên", "lien", "ket"}},
		{"compliance", []string{"banned", "forbidden", "claim", "guaranteed", "best", "cheapest"}},
	}

	clusters := make([]cluster, len(clusterDefs))
	for i, def := range clusterDefs {
		clusters[i] = cluster{name: def.name, terms: make(map[string]struct{}, len(def.terms))}
		for _, t := range def.terms {
			clusters[i].terms[t] = struct{}{}
		}
	}
	// Dimension: one axis per cluster, plus 8 "noise" dimensions
	// derived from the text hash to ensure unrelated texts don't
	// accidentally collide on shared zero vectors. Total = 26 dims
	// for the default config — way smaller than OpenAI's 1536 but
	// large enough for the soak to exercise the pipeline correctly.
	return &ClusteredEmbedder{
		clusters: clusters,
		dim:      len(clusters) + 8,
	}
}

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
