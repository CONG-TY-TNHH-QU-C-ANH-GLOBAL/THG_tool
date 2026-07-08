package reel

import (
	"context"
	"time"

	reelstore "github.com/thg/scraper/internal/store/reel"
)

// EnrichedStore is the persistence port the enriched pipeline depends on —
// exactly the reel-store methods the service calls, no more. Declaring it as
// an interface (not the concrete *reelstore.Store) is what lets the
// orchestration be exercised locally with an in-memory double
// (InMemoryStore) while production injects the Postgres-backed store, which
// satisfies this interface structurally. No new SQLite reel SCHEMA is
// introduced — the in-memory store is a test/dev double, not persisted
// tables (per the ADR's Postgres-only data-plane rule).
type EnrichedStore interface {
	GetEnriched(ctx context.Context, orgID, reelID int64) (*reelstore.Enriched, error)
	SetSource(ctx context.Context, orgID, reelID int64, sourceKey, inputBranch string) error
	SetAvatarKey(ctx context.Context, orgID, reelID int64, avatarKey string) error
	SetFinalOutput(ctx context.Context, orgID, reelID int64, finalKey string) error
	AddCost(ctx context.Context, orgID, reelID int64, deltaUSD float64) error
	ClaimRender(ctx context.Context, orgID, reelID int64, key string, leaseExpiry time.Time) (bool, error)
	CreateTranscript(ctx context.Context, orgID, reelID int64, in reelstore.TranscriptInput) (int64, error)
	GetLatestScript(ctx context.Context, orgID, reelID int64) (*reelstore.Script, error)
	CreateScript(ctx context.Context, orgID, reelID int64, version int, content string) (int64, error)
	UpdateReelStatus(ctx context.Context, orgID, reelID int64, status string) error
}

// Compile-time proof the Postgres store satisfies the port.
var _ EnrichedStore = (*reelstore.Store)(nil)
