package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/server/agent"
	"github.com/thg/scraper/internal/store/postgres"
)

// Compile-time proof the PostgreSQL adapter matches the PR10 outbound
// lifecycle seam exactly (same method names + signatures the active SQLite
// store exposes). Placed in the test package so production store code never
// imports internal/server/agent (which would be a bad dependency direction).
var _ agent.OutboundLifecycleRepository = (*postgres.OutboundStore)(nil)

// newTestStore applies a clean outbound schema to the POSTGRES_TEST_DSN
// database and returns an adapter + pool. Skips (not fails) when no DSN is
// configured — see internal/store/postgres/README.md for running it locally
// against deploy/dev/docker-compose.yml.
func newTestStore(t *testing.T) (*postgres.OutboundStore, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN not set; skipping PostgreSQL outbound integration tests (strict type-scan compatibility NOT proven)")
	}
	ctx := context.Background()
	pool, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	ddl, err := os.ReadFile("migrations/001_outbound_core.sql")
	if err != nil {
		pool.Close()
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, "DROP TABLE IF EXISTS outbound_messages"); err != nil {
		pool.Close()
		t.Fatalf("drop table: %v", err)
	}
	if _, err := pool.Exec(ctx, string(ddl)); err != nil {
		pool.Close()
		t.Fatalf("apply migration: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP TABLE IF EXISTS outbound_messages")
		pool.Close()
	})
	return postgres.NewOutboundStore(pool), pool
}

// insertPlanned inserts one planned outbound row and returns its id.
func insertPlanned(t *testing.T, pool *pgxpool.Pool, orgID, accountID int64, targetURL string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO outbound_messages (org_id, type, platform, account_id, target_url, content, execution_state)
		 VALUES ($1, 'comment', 'facebook', $2, $3, 'hello', 'planned') RETURNING id`,
		orgID, accountID, targetURL,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert planned: %v", err)
	}
	return id
}

func TestPostgresOutboundReadScans(t *testing.T) {
	store, pool := newTestStore(t)
	const org = int64(7)
	id := insertPlanned(t, pool, org, 11, "https://fb.com/p/1")

	msgs, err := store.GetOutboundByExecutionStateForOrg(org, models.ExecPlanned, "", 10)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want 1 planned row, got %d", len(msgs))
	}
	m := msgs[0]
	if m.ID != id || m.OrgID != org || m.Type != "comment" || m.Platform != "facebook" {
		t.Fatalf("scanned fields wrong: %+v", m)
	}
	if m.ExecutionState != models.ExecPlanned {
		t.Fatalf("want planned, got %q", m.ExecutionState)
	}
	if m.CreatedAt.IsZero() {
		t.Fatalf("created_at should scan as a non-zero timestamptz")
	}
	if !m.SentAt.IsZero() {
		t.Fatalf("sent_at should be zero for an unsent row, got %v", m.SentAt)
	}
	if m.VerificationOutcome != "" {
		t.Fatalf("verification_outcome should be empty (NULL) for planned, got %q", m.VerificationOutcome)
	}
}

func TestPostgresClaimIsSingleWinner(t *testing.T) {
	store, pool := newTestStore(t)
	const org = int64(7)
	id := insertPlanned(t, pool, org, 11, "https://fb.com/p/2")

	claim, err := store.ClaimPlannedOutboundForOrg(org, id, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if claim == nil || claim.ExecutionID == "" {
		t.Fatalf("claim must return a ClaimResult with an execution id")
	}
	if !claim.LeaseExpiry.After(time.Now()) {
		t.Fatalf("lease_expiry must be in the future, got %v", claim.LeaseExpiry)
	}

	// Second claim of the same row must fail — it is no longer planned.
	if _, err := store.ClaimPlannedOutboundForOrg(org, id, "worker-b", time.Minute); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("second claim should return sql.ErrNoRows, got %v", err)
	}

	// The pool is still usable after the no-op claim (proves the failed
	// claim's transaction was released, not leaked).
	executing, err := store.GetOutboundByExecutionStateForOrg(org, models.ExecExecuting, "", 10)
	if err != nil {
		t.Fatalf("read after claim: %v", err)
	}
	if len(executing) != 1 || executing[0].ExecutionID != claim.ExecutionID {
		t.Fatalf("row should be executing with the claim's execution id")
	}
}
