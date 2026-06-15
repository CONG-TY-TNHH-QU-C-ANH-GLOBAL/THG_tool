package agent

import (
	"testing"

	"github.com/thg/scraper/internal/store/coordination"
)

// C) Source-URL preservation + identity matching for the connector crawl-result ingest.
// The connector reports a lossy permalink.php URL; the explicit direct-post lead must
// instead carry the requested context-preserving group permalink, and only the post the
// workflow actually requested may be overridden.
func TestDirectPostItemOverride(t *testing.T) {
	const requested = "https://www.facebook.com/groups/ship.viet.my/permalink/4505595319766639/"
	const observedLossy = "https://www.facebook.com/permalink.php?story_fbid=4505595319766639"
	wf := &coordination.DirectPostCommentWorkflow{
		ID: 11, CanonicalPostURL: requested, PostFBID: "4505595319766639", GroupRef: "ship.viet.my",
	}

	// Observed lossy permalink.php for the SAME post id → override to the canonical
	// group URL (preserves group context; lets the P1.1 exact-canonical lookup match).
	id, ok := directPostItemOverride(wf, observedLossy, "4505595319766639")
	if !ok {
		t.Fatal("the requested post must be overridden")
	}
	if id.primaryURL != requested {
		t.Errorf("primaryURL must be the requested group permalink, got %q", id.primaryURL)
	}
	if id.postFBID != "4505595319766639" || id.groupRef != "ship.viet.my" {
		t.Errorf("identity not preserved: %+v", id)
	}

	// Post id inferred from the URL when the connector omits post_fbid.
	if _, ok := directPostItemOverride(wf, observedLossy, ""); !ok {
		t.Error("must infer post id from the observed URL when post_fbid is empty")
	}

	// A DIFFERENT post returned by the same crawl is NOT overridden (normal filtering).
	if _, ok := directPostItemOverride(wf, "https://www.facebook.com/groups/ship.viet.my/permalink/999/", "999"); ok {
		t.Error("a different post must not be overridden")
	}

	// Nil workflow (ordinary crawl) → never overridden.
	if _, ok := directPostItemOverride(nil, observedLossy, "4505595319766639"); ok {
		t.Error("nil workflow must not override")
	}

	// Empty canonical degrades to the observed URL (never empties the lead URL).
	wfNoCanon := &coordination.DirectPostCommentWorkflow{ID: 12, PostFBID: "4505595319766639"}
	got, ok := directPostItemOverride(wfNoCanon, observedLossy, "4505595319766639")
	if !ok || got.primaryURL != observedLossy {
		t.Errorf("empty canonical must degrade to observed URL, got ok=%v url=%q", ok, got.primaryURL)
	}
}
