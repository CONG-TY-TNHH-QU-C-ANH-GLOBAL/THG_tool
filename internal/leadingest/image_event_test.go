package leadingest

import (
	"context"
	"testing"
)

func TestOriginalPostImagesReachLeadEvent(t *testing.T) {
	db, appStore := newIngestStores(t)
	var events []LeadEvent
	deps := qualifyingDeps(db, appStore, func(ev LeadEvent) { events = append(events, ev) })
	in := qualifyingInput("image-context", "https://facebook.com/groups/1/posts/901")
	in.ImageURLs = []string{"https://scontent.fbcdn.net/post.jpg", "https://localhost/private.jpg", "https://scontent.fbcdn.net/post.jpg"}
	out, err := IngestPost(context.Background(), deps, in)
	if err != nil || !out.Inserted || len(events) != 1 {
		t.Fatalf("lead event missing: %+v, %v", out, err)
	}
	if events[0].OrgID != in.OrgID || len(events[0].ImageURLs) != 1 || events[0].ImageURLs[0] != in.ImageURLs[0] {
		t.Fatal("image references lost their org/post association or filtering")
	}
	in.ImageURLs[0] = "changed-after-ingest"
	if events[0].ImageURLs[0] == in.ImageURLs[0] {
		t.Fatal("event media must not alias the caller's mutable input slice")
	}
}
