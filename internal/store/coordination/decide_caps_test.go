package coordination

import (
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/dbutil"
)

func TestDecideCaps(t *testing.T) {
	caps := models.BehaviourCaps{CommentsPerDay: 5, RiskScoreCeiling: 0.8}
	// Fixed clock → deterministic, no UTC day-rollover flake.
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	today := dbutil.UTCDayKey(now)
	future := now.Add(time.Hour)

	cases := []struct {
		name        string
		comments    int
		risk        float64
		cooldown    time.Time
		actorBlock  bool
		adminPause  bool
		wantAllowed bool
		wantReason  string
	}{
		{"admin pause overrides all (gate #0)", 0, 0.9, future, true, true, false, "assignment_paused_by_admin"},
		{"actor blocked overrides pacing", 0, 0.9, future, true, false, false, "actor_mismatch_blocked"},
		{"cooldown active", 0, 0, future, false, false, false, "account_cooldown_active"},
		{"risk ceiling", 0, 0.9, time.Time{}, false, false, false, "risk_ceiling_exceeded"},
		{"daily limit (5>=5)", 5, 0.1, time.Time{}, false, false, false, "daily_limit_exceeded"},
		{"ok", 1, 0.1, time.Time{}, false, false, true, "ok"},
	}
	for _, c := range cases {
		d := DecideCaps(now, caps, today, c.comments, 0, 0, 0, c.risk, c.cooldown, c.actorBlock, c.adminPause, "comment")
		if d.Allowed != c.wantAllowed || d.Reason != c.wantReason {
			t.Fatalf("%s: got allowed=%v reason=%q, want allowed=%v reason=%q", c.name, d.Allowed, d.Reason, c.wantAllowed, c.wantReason)
		}
	}
}
