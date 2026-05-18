// Package embedding is the Layer-2.5 of the Workspace Knowledge OS:
// the asynchronous pipeline that converts asset text into vector
// representations the pgvector Searcher (Layer 4, PR-2) will query.
//
// THIS PACKAGE CONTAINS THE CONTRACT ONLY. Concrete adapters
// (OpenAI, future local models) live in sibling files. The worker
// that drives the pipeline lives in worker.go.
//
// Architectural rules — read before touching anything here:
//
//   1. Embedders are stateless. Construct once at boot, reuse for
//      every batch. No per-call mutation. Concurrency-safe by
//      construction.
//
//   2. The Embedder.ModelVersion() string is the source of truth for
//      "which model produced this vector". Persisted in
//      knowledge_assets.embedding_model_version. When the team
//      upgrades to a different embedding model, this string MUST
//      change — the Searcher uses it to detect stale embeddings.
//
//   3. Embedder.Dimensions() is checked at boot against the actual
//      VECTOR(N) column type. Mismatches fail loudly so we never
//      INSERT a wrong-dim vector that PG silently rejects later.
//
//   4. The embedding pipeline is best-effort. A pending asset that
//      fails MaxAttempts times moves to status='failed' and stops
//      retrying until operator intervention. Retrieval gracefully
//      degrades to hybrid for failed assets (PR-2 behavior).
package embedding

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"sort"
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// Embedder is implemented by every embedding-model adapter. The
// dispatcher (Worker) holds exactly one instance for the lifetime of
// the binary — there is no per-call routing logic, no model
// selection at query time. To switch models, change the boot config
// and re-deploy.
type Embedder interface {
	// Embed converts a batch of texts into vectors. The output slice
	// has the same length as the input slice; output[i] is the vector
	// for texts[i]. The implementation MAY split the batch internally
	// (e.g. OpenAI's per-request token limit) but the caller sees
	// exactly one input → one output mapping.
	//
	// Error contract:
	//   - returns [WrapRecoverable] on transient failures (rate limit,
	//     5xx, network timeout). The Worker retries the batch up to
	//     MaxAttempts.
	//   - returns [WrapPermanent] on auth/validation failures (401,
	//     400 with malformed input). The Worker marks the batch
	//     failed without retrying.
	//   - returns a bare error on unclassified failure. The Worker
	//     treats it as recoverable (conservative default).
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// ModelVersion is the stable identifier persisted on every row
	// this embedder generates. Format convention:
	//
	//   <provider>:<model-name>:v<schema-version>
	//
	// Examples:
	//   "openai:text-embedding-3-small:v1"
	//   "openai:text-embedding-3-large:v1"
	//   "local:bge-base-en-v1.5:v1"
	//
	// MUST change when the underlying model is swapped, even
	// minor revisions — the Searcher uses string equality to detect
	// vectors that need re-embedding.
	ModelVersion() string

	// Dimensions returns the vector size this embedder produces.
	// Validated at boot against the VECTOR(N) column. Mismatches
	// cause the embedding worker to refuse to start.
	Dimensions() int
}

// BuildInputText is the canonical conversion from asset → embedding
// input string. Called by:
//
//   - the worker, when it loads pending rows and needs to embed them
//   - the store hook (Upsert*) when computing embedding_input_hash
//     to decide if re-embedding is needed
//
// Both call sites MUST use this function — drift between them would
// cause the worker to re-embed assets the hook thought were
// unchanged (wasted API calls) or skip assets that actually changed
// (stale embeddings).
//
// Format is structured rather than free-form concat so the embedding
// model has a chance to weight the fields. Tags are sorted so
// re-ordering of the source data does not invalidate the hash.
func BuildInputText(a *assets.Asset) string {
	if a == nil {
		return ""
	}
	tags := append([]string(nil), a.Tags...)
	sort.Strings(tags)
	var b strings.Builder
	b.WriteString("Title: ")
	b.WriteString(strings.TrimSpace(a.Title))
	b.WriteString("\nType: ")
	b.WriteString(string(a.Type))
	if d := strings.TrimSpace(a.Description); d != "" {
		b.WriteString("\nDescription: ")
		b.WriteString(d)
	}
	if len(tags) > 0 {
		b.WriteString("\nTags: ")
		b.WriteString(strings.Join(tags, ", "))
	}
	return b.String()
}

// InputHash returns a stable digest of the embedding-input text.
// Used as the change-detection signal: when an asset's hash differs
// from the stored embedding_input_hash, the Upsert hook marks
// embedding_status='pending' so the worker re-embeds.
//
// SHA-1 chosen for the same reason as in assets/normalize.go: not
// cryptographic strength but a short, well-distributed hex string.
// Collision risk on workspace-scale (thousands of assets per org) is
// astronomically low.
func InputHash(a *assets.Asset) string {
	h := sha1.New()
	_, _ = h.Write([]byte(BuildInputText(a)))
	return hex.EncodeToString(h.Sum(nil))
}

// ErrCrossModelVersion is returned by the worker when the current
// Embedder's ModelVersion does not match what a row was previously
// embedded with AND the team has not explicitly opted into automatic
// re-embedding via env. Surfaced so operators see "you upgraded the
// model; here's how many rows need re-backfill" instead of silently
// re-embedding the entire catalog.
var ErrCrossModelVersion = errors.New("embedding: model version changed; explicit backfill required")

// --- recoverable / permanent error wrappers ---
//
// Same shape as ingestion.WrapRecoverable / WrapPermanent. Duplicated
// (not imported from ingestion) to avoid the embedding package
// depending on the ingestion package — these are sibling layers,
// neither owns the other.

func WrapRecoverable(err error) error {
	if err == nil {
		return nil
	}
	return recoverableError{err: err}
}

func WrapPermanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err: err}
}

func IsRecoverable(err error) bool {
	if err == nil {
		return false
	}
	var r recoverableError
	return errors.As(err, &r)
}

func IsPermanent(err error) bool {
	if err == nil {
		return false
	}
	var p permanentError
	return errors.As(err, &p)
}

type recoverableError struct{ err error }

func (e recoverableError) Error() string { return e.err.Error() }
func (e recoverableError) Unwrap() error { return e.err }

type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }
