package directpost

import "testing"

const (
	tPostID    = "4506703172989187"
	tGroup     = "ship.viet.my"
	tCanonical = "https://www.facebook.com/groups/ship.viet.my/permalink/4506703172989187/"
	tShipping  = "Em ở Q7 HCM cần gửi hàng đông lạnh tầm 22kg đi Texas ạ."
	tJobs      = "Backend & Frontend Developer Vietnam (Jobs) [CLUEGA] TUYỂN DỤNG BACKEND ENGINEER (AI SAAS / MCP ADS SYSTEM)"
	tForeignGr = "https://www.facebook.com/groups/1112083256270739/user/100029/"
)

func exp() ExpectedTarget {
	return ExpectedTarget{PostFBID: tPostID, GroupRef: tGroup, CanonicalURL: tCanonical}
}

// A — fail-closed on a missing/unextractable observed post id.
func TestPositivePostIDMatch_FailClosedOnMissing(t *testing.T) {
	ok, reason := PositivePostIDMatch(exp(), ObservedItem{
		PostFBID: "", SourceURL: "https://www.facebook.com/groups/ship.viet.my/", // group shell, no post id
	})
	if ok || reason != ReasonPostIDUnverified {
		t.Fatalf("missing post id must be unverified, got ok=%v reason=%q", ok, reason)
	}
}

// B — different observed post id is rejected.
func TestPositivePostIDMatch_Mismatch(t *testing.T) {
	ok, reason := PositivePostIDMatch(exp(), ObservedItem{PostFBID: "9999999999999999"})
	if ok || reason != ReasonPostIDMismatch {
		t.Fatalf("different post id must mismatch, got ok=%v reason=%q", ok, reason)
	}
}

func TestPositivePostIDMatch_PositiveAndCanonicalFallback(t *testing.T) {
	// Exact connector-reported id match.
	if ok, _ := PositivePostIDMatch(exp(), ObservedItem{PostFBID: tPostID}); !ok {
		t.Error("matching post id must pass")
	}
	// Id inferred from a GROUP-permalink observed URL (group context preserved) → pass.
	if ok, _ := PositivePostIDMatch(exp(), ObservedItem{SourceURL: tCanonical}); !ok {
		t.Error("post id from a group-permalink URL must pass")
	}
	// Unknown expected id → require positive canonical proof; absent → unverified.
	noID := ExpectedTarget{GroupRef: tGroup, CanonicalURL: tCanonical}
	if ok, _ := PositivePostIDMatch(noID, ObservedItem{SourceURL: tCanonical}); !ok {
		t.Error("canonical-equivalent source must pass when expected id unknown")
	}
	if ok, reason := PositivePostIDMatch(noID, ObservedItem{SourceURL: "https://www.facebook.com/some/other"}); ok || reason != ReasonPostIDUnverified {
		t.Errorf("no id + no canonical proof must be unverified, got ok=%v reason=%q", ok, reason)
	}
}

// 1c — a bare permalink.php?story_fbid=N observed URL with NO connector post id is the
// P1.1-ambiguous GLOBAL id; for a GROUP workflow it must NOT be positive proof → no
// ForceLead. (A non-group workflow may still use it.)
func TestPositivePostIDMatch_BarePermalinkPhpNotProofForGroup(t *testing.T) {
	bare := ObservedItem{PostFBID: "", SourceURL: "https://www.facebook.com/permalink.php?story_fbid=" + tPostID}

	// Group workflow → unverified (cannot ForceLead on a bare permalink.php alone).
	if ok, reason := PositivePostIDMatch(exp(), bare); ok || reason != ReasonPostIDUnverified {
		t.Fatalf("bare permalink.php must be unverified for a group workflow, got ok=%v reason=%q", ok, reason)
	}
	// But the same bare URL still works when the connector DID report the post id.
	if ok, _ := PositivePostIDMatch(exp(), ObservedItem{PostFBID: tPostID, SourceURL: bare.SourceURL}); !ok {
		t.Error("connector-reported id must pass even with a lossy permalink.php URL")
	}
	// Non-group workflow (no GroupRef) may use the permalink.php id.
	nonGroup := ExpectedTarget{PostFBID: tPostID}
	if ok, _ := PositivePostIDMatch(nonGroup, bare); !ok {
		t.Error("non-group workflow may use the permalink.php id")
	}
}

// C — a foreign GROUP author context is a conflict; a normal user author is not.
func TestContextConflict(t *testing.T) {
	// Author profile is a DIFFERENT group → conflict.
	if c, r := ContextConflict(tGroup, tCanonical, tForeignGr, ""); !c || r != ReasonGroupConflict {
		t.Errorf("foreign-group author must conflict, got %v %q", c, r)
	}
	// Normal user-profile author → NOT a conflict (must not reject normal users).
	if c, _ := ContextConflict(tGroup, tCanonical, "https://www.facebook.com/nhii.tran", ""); c {
		t.Error("a normal user author must not conflict")
	}
	// Same group → no conflict.
	if c, _ := ContextConflict(tGroup, tCanonical, "", ""); c {
		t.Error("same group must not conflict")
	}
	// Different NAMED source group → conflict.
	if c, _ := ContextConflict(tGroup, "https://www.facebook.com/groups/other.named.group/permalink/"+tPostID+"/", "", ""); !c {
		t.Error("different named source group must conflict")
	}
	// Different NUMERIC source group → ambiguous (possible vanity→numeric redirect), NOT a conflict.
	if c, _ := ContextConflict(tGroup, "https://www.facebook.com/groups/1112083256270739/permalink/"+tPostID+"/", "", ""); c {
		t.Error("different numeric source group must be ambiguous, not a hard conflict")
	}
	// No expected group → never conflict.
	if c, _ := ContextConflict("", tForeignGr, tForeignGr, ""); c {
		t.Error("no expected group must not conflict")
	}
}

// D — UI-chrome / boilerplate content is rejected; real text passes.
func TestValidContent(t *testing.T) {
	cases := []struct {
		name    string
		content string
		valid   bool
	}{
		{"empty", "", false},
		{"whitespace", "    ", false},
		{"repeated facebook chrome", "Facebook Facebook Facebook Facebook Facebook", false},
		{"repeated UI tokens", "Like Like Like Comment Comment Share Share", false},
		{"short", "ok thanks", false},
		{"real shipping text", tShipping, true},
		{"chrome prefix + real jobs text", "Facebook Facebook " + tJobs, true},
	}
	for _, c := range cases {
		if got := ValidContent(c.content); got != c.valid {
			t.Errorf("%s: ValidContent=%v want %v (meaningful=%q)", c.name, got, c.valid, MeaningfulText(c.content))
		}
	}
}

// The incident, end-to-end at the invariant layer: post id matches but the author is a
// foreign group → IdentityMatched (so the workflow is failed), Valid=false, group conflict.
func TestValidate_Incident(t *testing.T) {
	v := Validate(exp(), ObservedItem{
		PostFBID: tPostID, SourceURL: tCanonical, GroupFBID: tGroup,
		AuthorName: "Backend & Frontend Developer Vietnam (Jobs)", AuthorProfileURL: tForeignGr,
		Content: tJobs,
	})
	if !v.IdentityMatched || v.Valid || v.Reason != ReasonGroupConflict {
		t.Fatalf("incident must be identity-matched, invalid, group conflict; got %+v", v)
	}
}

// E — a valid shipping post (lossy source URL, matching id, normal author) passes.
func TestValidate_ValidShippingPost(t *testing.T) {
	v := Validate(exp(), ObservedItem{
		PostFBID: tPostID, SourceURL: "https://www.facebook.com/permalink.php?story_fbid=" + tPostID,
		AuthorName: "Nhii Tran", AuthorProfileURL: "https://www.facebook.com/nhii.tran", Content: tShipping,
	})
	if !v.Valid {
		t.Fatalf("valid shipping post must pass, got %+v", v)
	}
}
