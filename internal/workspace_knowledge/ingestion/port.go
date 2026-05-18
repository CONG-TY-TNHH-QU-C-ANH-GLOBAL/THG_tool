// Package ingestion defines the Layer-2 port for the Workspace
// Knowledge OS. An Ingestor pulls data from one external system
// (Shopify, CSV, Google Sheets, Notion, website crawler, …) and
// writes normalized [assets.Asset] rows via an [AssetWriter].
//
// This package contains the CONTRACT only. Concrete adapters live in
// sibling packages — one per source type — and land in Phase B of the
// roadmap. See [specs/WORKSPACE_KNOWLEDGE_OS.md §7].
//
// Why a port instead of a switch statement on SourceType: each
// connector has its own auth model, schema, error taxonomy, and
// retry shape. Hiding them all behind a single Ingestor interface
// keeps the dispatcher (and tests) free of conditional ladders. A
// future Shopify-Plus variant adds a new adapter without modifying
// any existing call site.
package ingestion

import (
	"context"
	"errors"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

// Ingestor is implemented by every connector adapter. The dispatcher
// selects one ingestor per source row by exact match on Type().
type Ingestor interface {
	// Type identifies which Source.Type values this ingestor handles.
	Type() sources.SourceType

	// Sync pulls fresh data from the external system identified by src
	// and writes normalized assets via the provided writer.
	//
	// Implementations MUST:
	//   - be idempotent: re-syncing the same source updates existing
	//     assets (matched on external_id), never duplicates.
	//   - be org-scoped: every asset written carries src.OrgID. The
	//     AssetWriter enforces this for you.
	//   - never panic on transient network errors: return them as
	//     SyncError entries so the dispatcher can decide retry vs.
	//     report.
	//   - normalize tags via [assets.NormalizeTags] before writing.
	//
	// Implementations SHOULD:
	//   - compute a deterministic external_id even when the source
	//     does not expose stable IDs (use [assets.ContentFingerprint]).
	//   - prefer streaming writes over batching, so large catalogs
	//     show progress in the UI instead of one big "syncing" pause.
	//
	// The returned SyncResult is informational — the caller will write
	// it to the source row's health columns via
	// store.UpdateKnowledgeSourceHealth. A non-nil error means the
	// sync was unrecoverable; partial-success cases (some assets in,
	// some rejected) report SyncResult.Errors and return nil error.
	Sync(ctx context.Context, src *sources.Source, w AssetWriter) (SyncResult, error)
}

// AssetWriter is the constrained write surface an ingestor sees.
// Hiding the *store.Store behind this interface means:
//
//  1. Ingestors are testable without a database — provide a fake writer.
//  2. Cross-tenant enforcement lives in exactly one place (the
//     store-backed writer). An ingestor cannot reach the store directly,
//     so it cannot smuggle assets into another tenant.
//  3. Future writers can route assets to a different backend (e.g. a
//     staging table for review-before-publish) without changing any
//     existing ingestor.
type AssetWriter interface {
	// Write upserts one asset under the source bound to this writer.
	// The writer ENFORCES asset.OrgID == src.OrgID and asset.SourceID
	// == src.ID — passing other values returns ErrCrossTenantWrite.
	//
	// Operator-controlled columns on the existing row (state, pinned,
	// boost) are NOT touched. See specs/WORKSPACE_KNOWLEDGE_OS.md §6.
	Write(ctx context.Context, a *assets.Asset) error
}

// ErrCrossTenantWrite is returned by AssetWriter implementations when
// the caller attempts to write an asset whose OrgID or SourceID does
// not match the writer's binding. This is a programmer error — a
// well-behaved ingestor never sees it.
var ErrCrossTenantWrite = errors.New("ingestion: asset OrgID/SourceID does not match writer binding")

// SyncResult is the report an ingestor returns. AssetsSeen is the
// canonical "did the source even respond" metric; AssetsCreated +
// AssetsUpdated decomposes it; AssetsRejected counts items that
// failed [assets.Asset.Validate] and were skipped.
type SyncResult struct {
	AssetsSeen     int
	AssetsCreated  int
	AssetsUpdated  int
	AssetsRejected int
	Errors         []SyncError
}

// SyncError describes one item the ingestor could not process. Carries
// enough information for the operator to locate the offending row in
// the source system, without leaking the entire row body into the log.
type SyncError struct {
	ExternalID string // "" if the source did not expose one
	Reason     string // short, human-readable; safe to surface in UI
	Detail     string // optional verbose info for debug logs only
}

// IsRecoverable reports whether the underlying error is transient
// (network blip, 429 rate limit, etc.) versus permanent (auth failure,
// schema mismatch). The dispatcher uses this to decide retry policy.
//
// Wired to errors.Is for explicit unwrap chains — implementations
// should return WrapRecoverable / WrapPermanent (helpers below) when
// reporting upstream so the dispatcher does not need to know the
// concrete error type.
func IsRecoverable(err error) bool {
	if err == nil {
		return false
	}
	var r recoverableError
	return errors.As(err, &r)
}

// WrapRecoverable marks err as a transient failure suitable for retry.
func WrapRecoverable(err error) error {
	if err == nil {
		return nil
	}
	return recoverableError{err: err}
}

// WrapPermanent marks err as a non-retryable failure (auth, schema).
func WrapPermanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err: err}
}

type recoverableError struct{ err error }

func (e recoverableError) Error() string { return e.err.Error() }
func (e recoverableError) Unwrap() error { return e.err }

type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }
