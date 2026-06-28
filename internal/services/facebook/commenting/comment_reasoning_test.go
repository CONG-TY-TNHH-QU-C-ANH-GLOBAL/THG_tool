package commenting

import "testing"

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
