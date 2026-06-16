package directpost

import "testing"

func bvTarget() ExpectedTarget {
	return ExpectedTarget{PostFBID: tPostID, GroupRef: tGroup, CanonicalURL: tCanonical}
}

// Pre-submit guard: a correct live context (post id + group match) is ALLOWED.
func TestVerifyBrowserContext_PositivePath(t *testing.T) {
	ok, reason := VerifyBrowserContext(bvTarget(), BrowserContext{
		CurrentPostFBID: tPostID, CurrentGroupRef: tGroup,
		CurrentURL: tCanonical, AuthorSnippet: "frozen goods to Texas",
	})
	if !ok || reason != "" {
		t.Fatalf("correct live context must be allowed, got ok=%v reason=%q", ok, reason)
	}
	// Live post id inferred from the URL when not reported directly.
	if ok, _ := VerifyBrowserContext(bvTarget(), BrowserContext{CurrentURL: tCanonical}); !ok {
		t.Error("live post id from the current URL must be allowed")
	}
}

// Pre-submit guard aborts on each mismatch class.
func TestVerifyBrowserContext_Aborts(t *testing.T) {
	cases := []struct {
		name   string
		live   BrowserContext
		reason string
	}{
		{"post id missing", BrowserContext{CurrentURL: "https://www.facebook.com/groups/ship.viet.my/"}, ReasonBrowserPostIDMissing},
		{"post id mismatch", BrowserContext{CurrentPostFBID: "9999999999999999"}, ReasonBrowserPostIDMismatch},
		{"different named group", BrowserContext{CurrentPostFBID: tPostID, CurrentGroupRef: "other.named.group"}, ReasonBrowserGroupMismatch},
		{"foreign group in url", BrowserContext{CurrentPostFBID: tPostID, CurrentURL: "https://www.facebook.com/groups/other.group/permalink/" + tPostID + "/"}, ReasonBrowserGroupMismatch},
	}
	for _, c := range cases {
		ok, reason := VerifyBrowserContext(bvTarget(), c.live)
		if ok || reason != c.reason {
			t.Errorf("%s: expected abort %q, got ok=%v reason=%q", c.name, c.reason, ok, reason)
		}
	}
}

// A different NUMERIC group is ambiguous (vanity→numeric), not a hard group abort — but the
// post id must still match for it to be allowed.
func TestVerifyBrowserContext_NumericGroupAmbiguous(t *testing.T) {
	ok, _ := VerifyBrowserContext(bvTarget(), BrowserContext{
		CurrentPostFBID: tPostID,
		CurrentURL:      "https://www.facebook.com/groups/123456789/permalink/" + tPostID + "/",
	})
	if !ok {
		t.Error("matching post id with an ambiguous numeric group must be allowed (post id is the proof)")
	}
}

// Unknown target post id → require positive canonical proof of the live URL.
func TestVerifyBrowserContext_CanonicalFallback(t *testing.T) {
	target := ExpectedTarget{GroupRef: tGroup, CanonicalURL: tCanonical}
	if ok, _ := VerifyBrowserContext(target, BrowserContext{CurrentURL: tCanonical}); !ok {
		t.Error("canonical-equivalent live URL must be allowed when target id unknown")
	}
	if ok, reason := VerifyBrowserContext(target, BrowserContext{CurrentURL: "https://www.facebook.com/some/other"}); ok || reason != ReasonBrowserContextMismatch {
		t.Errorf("no live id + no canonical proof must abort context_mismatch, got ok=%v reason=%q", ok, reason)
	}
}
