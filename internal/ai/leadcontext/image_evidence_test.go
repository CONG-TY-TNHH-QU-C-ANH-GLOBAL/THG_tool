package leadcontext

import (
	"strings"
	"testing"
)

const imageRequestContext = `{"intent":"product_sourcing","product":"cotton bags","text_evidence":[],"missing_fields":["quantity"],"visual":{"product":"cotton bags","observations":["Visible request: Need a supplier for cotton bags"],"uncertainties":[],"text_image_conflict":false},"image_evidence":[{"image_index":0,"quote":"Need a supplier for cotton bags"}]}`

func TestParseAcceptsImageRequestEvidence(t *testing.T) {
	if _, err := Parse([]byte(imageRequestContext), "", true); err != nil {
		t.Fatal("buyer request in the supplied image cannot be represented")
	}
}

func TestImageRequestEvidenceUsesActualPhotoCount(t *testing.T) {
	for _, count := range []int{-1, 0, 4} {
		if _, err := ParseWithImages([]byte(imageRequestContext), "", count); err == nil {
			t.Fatalf("unsupported image count accepted: %d", count)
		}
	}
	third := strings.Replace(imageRequestContext, `"image_index":0`, `"image_index":2`, 1)
	if _, err := ParseWithImages([]byte(third), "", 1); err == nil {
		t.Fatal("evidence referenced a photo that was never supplied")
	}
	if _, err := ParseWithImages([]byte(third), "", 3); err != nil {
		t.Fatal("third supplied image lost its evidence")
	}
}

func TestImageRequestEvidenceRejectsUnbackedOrMalformedQuotes(t *testing.T) {
	for _, entry := range []string{
		`null`,
		`[{"quote":"Need a supplier for cotton bags"}]`,
		`[{"image_index":null,"quote":"Need a supplier for cotton bags"}]`,
		`[{"image_index":-1,"quote":"Need a supplier for cotton bags"}]`,
		`[{"image_index":0,"quote":"Need winter gloves"}]`,
		`[{"image_index":0,"quote":""}]`,
		`[{"image_index":0,"quote":null}]`,
		`[{"image_index":0,"quote":"Need a supplier for cotton bags","price":20}]`,
		`[{"image_index":0,"quote":"Need a supplier for cotton bags"},{"image_index":0,"quote":"Need a supplier for cotton bags"}]`,
	} {
		data := strings.Replace(imageRequestContext, `[{"image_index":0,"quote":"Need a supplier for cotton bags"}]`, entry, 1)
		if _, err := ParseWithImages([]byte(data), "", 1); err == nil {
			t.Fatalf("unsupported evidence accepted: %s", entry)
		}
	}
}

func TestImageProductAloneDoesNotAuthorizeBuyerIntent(t *testing.T) {
	data := strings.Replace(imageRequestContext, `[{"image_index":0,"quote":"Need a supplier for cotton bags"}]`, `[]`, 1)
	if _, err := ParseWithImages([]byte(data), "", 1); err == nil {
		t.Fatal("an observed object without a request became a sourcing need")
	}
	data = strings.Replace(data, `"product_sourcing"`, `"unclear"`, 1)
	if _, err := ParseWithImages([]byte(data), "", 1); err != nil {
		t.Fatal("uncertain image context should remain representable")
	}
}
