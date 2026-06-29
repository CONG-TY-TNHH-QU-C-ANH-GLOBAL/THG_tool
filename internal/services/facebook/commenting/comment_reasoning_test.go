package commenting

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

// fakeSink captures the prompt-log write so the seam (ARCHCM2c seam 4) is testable
// without the concrete KB/MsgGen happy path.
type fakeSink struct {
	called  bool
	orgID   int64
	action  string
	args    string
	success bool
}

func (f *fakeSink) InsertSystemPromptLog(orgID, _ int64, _, action, args string, success bool) error {
	f.called = true
	f.orgID, f.action, f.args, f.success = orgID, action, args, success
	return nil
}

// TestLogDecision pins the decision-log write: action label carries the mode, success
// is the negation of KnowledgeGap, and the payload is the marshaled decision.
func TestLogDecision(t *testing.T) {
	sink := &fakeSink{}
	logDecision(sink, Input{OrgID: 7, AccountID: 3, Mode: "live"}, &models.CommentDecision{Intent: "promote"})

	if !sink.called {
		t.Fatal("sink not called")
	}
	if sink.orgID != 7 {
		t.Errorf("orgID=%d, want 7", sink.orgID)
	}
	if sink.action != "comment_decision_live" {
		t.Errorf("action=%q, want comment_decision_live", sink.action)
	}
	if !sink.success {
		t.Errorf("success=false, want true (KnowledgeGap=false)")
	}
	if !strings.Contains(sink.args, "promote") {
		t.Errorf("args missing marshaled decision: %q", sink.args)
	}
}

// TestLogDecision_KnowledgeGap: a knowledge-gap decision logs success=false.
func TestLogDecision_KnowledgeGap(t *testing.T) {
	sink := &fakeSink{}
	logDecision(sink, Input{Mode: "dryrun"}, &models.CommentDecision{KnowledgeGap: true})

	if sink.action != "comment_decision_dryrun" {
		t.Errorf("action=%q, want comment_decision_dryrun", sink.action)
	}
	if sink.success {
		t.Errorf("success=true, want false (KnowledgeGap=true)")
	}
}

// Mode is the P2c knowledge-reasoning kill-switch read from env. This pins the
// off/dryrun/live decision (incl. the THG_COMMENT_REASONING_DRYRUN=1 alias and
// case/space tolerance) that the move out of cmd/scraper must preserve verbatim.
func TestMode(t *testing.T) {
	cases := []struct {
		name      string
		reasoning string // THG_COMMENT_REASONING
		dryrunEnv string // THG_COMMENT_REASONING_DRYRUN
		want      string
	}{
		{"unset defaults off", "", "", "off"},
		{"explicit live", "live", "", "live"},
		{"explicit dryrun", "dryrun", "", "dryrun"},
		{"mixed case + spaces live", "  Live  ", "", "live"},
		{"mixed case dryrun", "DryRun", "", "dryrun"},
		{"unknown value -> off", "on", "", "off"},
		{"dryrun alias =1", "", "1", "dryrun"},
		{"alias only honors exactly 1", "", "true", "off"},
		{"explicit value wins over alias", "live", "1", "live"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("THG_COMMENT_REASONING", tc.reasoning)
			t.Setenv("THG_COMMENT_REASONING_DRYRUN", tc.dryrunEnv)
			if got := Mode(); got != tc.want {
				t.Errorf("Mode() with reasoning=%q dryrun=%q = %q, want %q", tc.reasoning, tc.dryrunEnv, got, tc.want)
			}
		})
	}
}
