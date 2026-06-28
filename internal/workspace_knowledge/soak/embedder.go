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
//
// This file owns the embedder's IDENTITY: its type and the
// POD/fulfillment cluster taxonomy it is pre-loaded with. The
// vectorisation algorithm lives in embedder_vectorize.go.
package soak

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
