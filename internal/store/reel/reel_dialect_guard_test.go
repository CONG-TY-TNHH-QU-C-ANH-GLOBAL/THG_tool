// Dialect-guard tests: no real database needed (unlike reel_test.go /
// reel_org_scope_test.go, which are gated on POSTGRES_PLATFORM_TEST_DSN),
// since requirePostgres must fail BEFORE any query ever reaches *sql.DB —
// these pass a nil db to prove it.
package reel_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/thg/scraper/internal/store/dbutil"
	"github.com/thg/scraper/internal/store/reel"
)

func TestReel_NonPostgresDialect_FailsBeforeQuerying(t *testing.T) {
	s := reel.NewStore(nil, dbutil.NewSQLiteDialect())
	ctx := context.Background()

	if _, err := s.CreateReel(ctx, 1, "t", "b", 1); !errors.Is(err, reel.ErrUnsupportedDialect) {
		t.Errorf("CreateReel: err = %v, want ErrUnsupportedDialect", err)
	}
	if _, err := s.GetReel(ctx, 1, 1); !errors.Is(err, reel.ErrUnsupportedDialect) {
		t.Errorf("GetReel: err = %v, want ErrUnsupportedDialect", err)
	}
	if _, err := s.ListReels(ctx, 1); !errors.Is(err, reel.ErrUnsupportedDialect) {
		t.Errorf("ListReels: err = %v, want ErrUnsupportedDialect", err)
	}
	if err := s.UpdateReelStatus(ctx, 1, 1, "draft"); !errors.Is(err, reel.ErrUnsupportedDialect) {
		t.Errorf("UpdateReelStatus: err = %v, want ErrUnsupportedDialect", err)
	}
	if _, err := s.CreateScript(ctx, 1, 1, 1, "{}"); !errors.Is(err, reel.ErrUnsupportedDialect) {
		t.Errorf("CreateScript: err = %v, want ErrUnsupportedDialect", err)
	}
	if _, err := s.GetLatestScript(ctx, 1, 1); !errors.Is(err, reel.ErrUnsupportedDialect) {
		t.Errorf("GetLatestScript: err = %v, want ErrUnsupportedDialect", err)
	}
	if _, err := s.ListScripts(ctx, 1, 1); !errors.Is(err, reel.ErrUnsupportedDialect) {
		t.Errorf("ListScripts: err = %v, want ErrUnsupportedDialect", err)
	}
	if err := s.ApproveScript(ctx, 1, 1); !errors.Is(err, reel.ErrUnsupportedDialect) {
		t.Errorf("ApproveScript: err = %v, want ErrUnsupportedDialect", err)
	}
	// Every call above returned before touching s.db (which is nil) — no
	// panic reaching this line is the proof.
}

// TestReelEnriched_NonPostgresDialect_FailsBeforeQuerying pins the same
// guard for the enriched-format accessors (migration 0112): all must fail
// with ErrUnsupportedDialect before touching the nil db.
func TestReelEnriched_NonPostgresDialect_FailsBeforeQuerying(t *testing.T) {
	s := reel.NewStore(nil, dbutil.NewSQLiteDialect())
	ctx := context.Background()

	if _, err := s.GetEnriched(ctx, 1, 1); !errors.Is(err, reel.ErrUnsupportedDialect) {
		t.Errorf("GetEnriched: err = %v, want ErrUnsupportedDialect", err)
	}
	if err := s.SetSource(ctx, 1, 1, "k", "audio"); !errors.Is(err, reel.ErrUnsupportedDialect) {
		t.Errorf("SetSource: err = %v, want ErrUnsupportedDialect", err)
	}
	if err := s.SetAvatarKey(ctx, 1, 1, "k"); !errors.Is(err, reel.ErrUnsupportedDialect) {
		t.Errorf("SetAvatarKey: err = %v, want ErrUnsupportedDialect", err)
	}
	if err := s.SetFinalOutput(ctx, 1, 1, "k"); !errors.Is(err, reel.ErrUnsupportedDialect) {
		t.Errorf("SetFinalOutput: err = %v, want ErrUnsupportedDialect", err)
	}
	if err := s.AddCost(ctx, 1, 1, 0.3); !errors.Is(err, reel.ErrUnsupportedDialect) {
		t.Errorf("AddCost: err = %v, want ErrUnsupportedDialect", err)
	}
	if _, err := s.ClaimRender(ctx, 1, 1, "idem", time.Now()); !errors.Is(err, reel.ErrUnsupportedDialect) {
		t.Errorf("ClaimRender: err = %v, want ErrUnsupportedDialect", err)
	}
	if _, err := s.CreateTranscript(ctx, 1, 1, reel.TranscriptInput{Segments: "[]", LangSrc: "vi", LangTgt: "en", Source: "whisper", CostUSD: 0.01}); !errors.Is(err, reel.ErrUnsupportedDialect) {
		t.Errorf("CreateTranscript: err = %v, want ErrUnsupportedDialect", err)
	}
	if _, err := s.GetLatestTranscript(ctx, 1, 1); !errors.Is(err, reel.ErrUnsupportedDialect) {
		t.Errorf("GetLatestTranscript: err = %v, want ErrUnsupportedDialect", err)
	}
}
