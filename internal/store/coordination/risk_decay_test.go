// Domain: coordination (see internal/store/DOMAINS.md)
package coordination_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/coordination"
	"github.com/thg/scraper/internal/store/dbutil"
)

// Decay reduces risk_score linearly with hours-idle. risk=0.5 with
// updated_at = 10h ago decays at 0.02/h * 10h = 0.20, landing at 0.30.
func TestApplyRiskDecay_LinearOverTime(t *testing.T) {
	full, _ := newCoordinationStore(t, "decay_linear.db")
	plantRuntimeRow(t, full.DB(), 11, 1, 0.5, time.Now().UTC().Add(-10*time.Hour))

	mustApplyDecay(t, full.DB(), 11)

	got := readRuntimeRisk(t, full.DB(), 11)
	const want = 0.30
	if diff := got - want; diff > 0.01 || diff < -0.01 {
		t.Errorf("after 10h decay: got %.4f, want ~%.4f (within 0.01)", got, want)
	}
}

// Decay clamps at 0. An old, low-risk row should land at exactly 0,
// never negative.
func TestApplyRiskDecay_ClampsAtZero(t *testing.T) {
	full, _ := newCoordinationStore(t, "decay_clamp.db")
	plantRuntimeRow(t, full.DB(), 12, 1, 0.05, time.Now().UTC().Add(-100*time.Hour))

	mustApplyDecay(t, full.DB(), 12)

	got := readRuntimeRisk(t, full.DB(), 12)
	if got != 0 {
		t.Errorf("over-decay must clamp at 0, got %.4f", got)
	}
}

// Missing runtime_state row is a no-op: no error, no insert.
func TestApplyRiskDecay_NoRowIsNoop(t *testing.T) {
	full, _ := newCoordinationStore(t, "decay_norow.db")

	mustApplyDecay(t, full.DB(), 999)

	var n int
	if err := full.DB().QueryRow(
		`SELECT COUNT(*) FROM account_runtime_state WHERE account_id = ?`, 999,
	).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("ApplyRiskDecayTx on missing account must not insert a row, got %d", n)
	}
}

// Zero risk is a no-op — must not touch updated_at, since doing so would
// reset the decay clock for future incoming signals.
func TestApplyRiskDecay_ZeroRiskIsNoop(t *testing.T) {
	full, _ := newCoordinationStore(t, "decay_zero.db")
	stamp := time.Now().UTC().Add(-5 * time.Hour)
	plantRuntimeRow(t, full.DB(), 13, 1, 0.0, stamp)

	mustApplyDecay(t, full.DB(), 13)

	gotUpdatedAt := readRuntimeUpdatedAt(t, full.DB(), 13)
	diff := gotUpdatedAt.Sub(stamp)
	if diff < 0 {
		diff = -diff
	}
	if diff > 2*time.Second {
		t.Errorf("ApplyRiskDecayTx with risk=0 must NOT bump updated_at; before=%s after=%s diff=%s",
			stamp.Format(time.RFC3339), gotUpdatedAt.Format(time.RFC3339), diff)
	}
}

// End-to-end: an account that previously exceeded the ceiling can recover
// after enough idle time. With TrustWarming (ceiling 0.60), seed risk=0.65
// updated_at=20h ago; after-decay score = 0.65 - 0.02*20 = 0.25 < 0.60,
// so the cap-check must now allow the action.
func TestCheckCapsTx_DecayAllowsRecoveryFromCeiling(t *testing.T) {
	full, coord := newCoordinationStore(t, "decay_recovery.db")
	ctx := context.Background()

	if err := coord.UpsertAccountBehaviourProfile(ctx, &models.AccountBehaviourProfile{
		AccountID: 14, OrgID: 1, TrustLevel: models.TrustWarming,
	}); err != nil {
		t.Fatalf("upsert profile: %v", err)
	}
	plantRuntimeRow(t, full.DB(), 14, 1, 0.65, time.Now().UTC().Add(-20*time.Hour))

	tx, err := full.DB().Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	dec, err := coord.CheckCapsTx(tx, 14, "comment")
	if err != nil {
		t.Fatalf("CheckCapsTx: %v", err)
	}
	if !dec.Allowed {
		t.Fatalf("after 20h idle decay, account at 0.65 (TrustWarming 0.60 ceiling) must be allowed, got reason=%q", dec.Reason)
	}
	if dec.Reason != "ok" {
		t.Errorf("reason: got %q, want %q", dec.Reason, "ok")
	}
}

// When CheckCapsTx denies with risk_ceiling_exceeded, the decision must
// carry the runtime risk_score + the effective ceiling. The operator
// surface (queueLeadOutreach return string) reads these to render
// "account=N risk=X ceiling=Y" inline without a separate diagnostic
// round-trip.
func TestCheckCapsTx_DenyCarriesRiskAndCeiling(t *testing.T) {
	full, coord := newCoordinationStore(t, "decay_carries.db")
	ctx := context.Background()

	if err := coord.UpsertAccountBehaviourProfile(ctx, &models.AccountBehaviourProfile{
		AccountID: 15, OrgID: 1, TrustLevel: models.TrustWarming,
	}); err != nil {
		t.Fatalf("upsert profile: %v", err)
	}
	// Seed risk above the TrustWarming ceiling (0.60) with updated_at NOW
	// so the decay path does not lower it within the test.
	plantRuntimeRow(t, full.DB(), 15, 1, 0.80, time.Now().UTC())

	tx, err := full.DB().Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	dec, err := coord.CheckCapsTx(tx, 15, "comment")
	if err != nil {
		t.Fatalf("CheckCapsTx: %v", err)
	}
	if dec.Allowed {
		t.Fatalf("risk=0.80 vs ceiling=0.60 must deny, got allowed")
	}
	if dec.Reason != "risk_ceiling_exceeded" {
		t.Fatalf("reason: got %q, want risk_ceiling_exceeded", dec.Reason)
	}
	if diff := dec.RiskScore - 0.80; diff > 0.001 || diff < -0.001 {
		t.Errorf("RiskScore: got %.4f, want ~0.80", dec.RiskScore)
	}
	if diff := dec.RiskCeiling - 0.60; diff > 0.001 || diff < -0.001 {
		t.Errorf("RiskCeiling: got %.4f, want ~0.60 (TrustWarming)", dec.RiskCeiling)
	}
}

// --- helpers shared across decay tests ---

func mustApplyDecay(t *testing.T, db *sql.DB, accountID int64) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := coordination.ApplyRiskDecayTx(tx, accountID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("ApplyRiskDecayTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func plantRuntimeRow(t *testing.T, db *sql.DB, accountID, orgID int64, risk float64, updatedAt time.Time) {
	t.Helper()
	today := time.Now().UTC().Format("2006-01-02")
	if _, err := db.Exec(
		`INSERT INTO account_runtime_state
			(account_id, org_id, counters_day, comments_today, inbox_today,
			 group_posts_today, profile_posts_today, risk_score, recent_failures,
			 updated_at)
		 VALUES (?, ?, ?, 0, 0, 0, 0, ?, 0, ?)
		 ON CONFLICT(account_id) DO UPDATE SET
			risk_score = excluded.risk_score,
			updated_at = excluded.updated_at`,
		accountID, orgID, today, risk,
		updatedAt.UTC().Format("2006-01-02 15:04:05"),
	); err != nil {
		t.Fatalf("plantRuntimeRow: %v", err)
	}
}

func readRuntimeRisk(t *testing.T, db *sql.DB, accountID int64) float64 {
	t.Helper()
	var v float64
	if err := db.QueryRow(
		`SELECT risk_score FROM account_runtime_state WHERE account_id = ?`, accountID,
	).Scan(&v); err != nil {
		t.Fatalf("readRuntimeRisk: %v", err)
	}
	return v
}

func readRuntimeUpdatedAt(t *testing.T, db *sql.DB, accountID int64) time.Time {
	t.Helper()
	var s string
	if err := db.QueryRow(
		`SELECT updated_at FROM account_runtime_state WHERE account_id = ?`, accountID,
	).Scan(&s); err != nil {
		t.Fatalf("readRuntimeUpdatedAt: %v", err)
	}
	parsed := dbutil.ParseSQLiteTime(s)
	if parsed.IsZero() {
		t.Fatalf("parse updated_at %q: returned zero", s)
	}
	return parsed.UTC()
}
