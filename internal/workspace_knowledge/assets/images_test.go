package assets

import (
	"encoding/json"
	"testing"
)

func TestImagesFromPayload_HappyPath(t *testing.T) {
	payload := json.RawMessage(`{
		"images": [
			{"url":"https://cdn/a.jpg","alt":"Cat Tee front","kind":"primary"},
			{"url":"https://cdn/b.jpg","alt":"Cat Tee back","kind":"alternate"}
		]
	}`)
	imgs := ImagesFromPayload(payload)
	if len(imgs) != 2 {
		t.Fatalf("got %d images; want 2", len(imgs))
	}
	if imgs[0].URL != "https://cdn/a.jpg" {
		t.Errorf("URL: got %q", imgs[0].URL)
	}
	if imgs[0].Kind != "primary" {
		t.Errorf("Kind should be lowercased; got %q", imgs[0].Kind)
	}
}

func TestImagesFromPayload_NoImagesField(t *testing.T) {
	payload := json.RawMessage(`{"price":"$18"}`)
	if got := ImagesFromPayload(payload); got != nil {
		t.Errorf("absent images field should return nil; got %+v", got)
	}
}

func TestImagesFromPayload_MalformedReturnsNilNotError(t *testing.T) {
	// Per the design comment, malformed payload returns nil cleanly
	// rather than aborting outreach.
	cases := []json.RawMessage{
		[]byte(`{"images":"not an array"}`),
		[]byte(`{"images":[{"url":""},{"url":"   "}]}`), // empty URLs filtered out
		[]byte(`not json`),
		[]byte(``),
	}
	for i, c := range cases {
		if got := ImagesFromPayload(c); len(got) > 0 {
			t.Errorf("case %d: expected nil/empty, got %+v", i, got)
		}
	}
}

func TestPrimaryImage_PrefersPrimaryKind(t *testing.T) {
	payload := json.RawMessage(`{
		"images": [
			{"url":"https://cdn/alt.jpg","kind":"alternate"},
			{"url":"https://cdn/primary.jpg","kind":"primary"}
		]
	}`)
	img := PrimaryImage(payload)
	if img == nil || img.URL != "https://cdn/primary.jpg" {
		t.Errorf("PrimaryImage should pick the 'primary' kind; got %+v", img)
	}
}

func TestPrimaryImage_FallsBackToFirst(t *testing.T) {
	// No image marked "primary" → return the first entry.
	payload := json.RawMessage(`{
		"images": [
			{"url":"https://cdn/x.jpg","kind":"alternate"},
			{"url":"https://cdn/y.jpg","kind":"mockup"}
		]
	}`)
	img := PrimaryImage(payload)
	if img == nil || img.URL != "https://cdn/x.jpg" {
		t.Errorf("fallback should pick the first entry; got %+v", img)
	}
}

func TestPrimaryImage_NilWhenEmpty(t *testing.T) {
	if got := PrimaryImage(json.RawMessage(`{}`)); got != nil {
		t.Errorf("empty payload should return nil PrimaryImage; got %+v", got)
	}
}
