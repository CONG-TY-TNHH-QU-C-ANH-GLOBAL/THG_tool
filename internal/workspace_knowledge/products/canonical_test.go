package products

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

func sampleProduct() *CanonicalProduct {
	pMin := 17.5
	pMax := 22.0
	return &CanonicalProduct{
		SourceID:         "cmp71ol4u00fy01qoaenw2w63",
		DisplaySKU:       "THG-US002-RHLG5I",
		VendorSKU:        "RHLG5I",
		Name:             "140Z Stainless Steel Car Cup",
		Description:      "Insulated 14oz travel mug for car cup holders.",
		Category:         "Home & Living",
		Origin:           "us",
		Sizes:            []string{"  14oz ", "14OZ", ""},
		Colors:           []string{"White"},
		Tags:             []string{"travel", "Travel", " mug "},
		PriceMin:         &pMin,
		PriceMax:         &pMax,
		Currency:         "usd",
		Images:           []string{"https://cdn.example.com/a.jpg", "https://cdn.example.com/a.jpg", "https://cdn.example.com/b.jpg"},
		Availability:     AvailInStock,
		SourceURL:        "https://hub.example.com/p/cmp71ol4u00fy01qoaenw2w63",
		SourceUpdatedAt:  time.Date(2026, 5, 17, 15, 44, 47, 0, time.UTC),
		RawPayloadHash:   "deadBEEF1234",
		ExtractorVersion: "rest_json/v1",
	}
}

func TestNormalize_IsIdempotent(t *testing.T) {
	p := sampleProduct()
	p.Normalize()
	first, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}
	p.Normalize()
	second, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("normalize not idempotent.\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestNormalize_TrimsLowersDedupesSortsTags(t *testing.T) {
	p := sampleProduct()
	p.Normalize()
	if got := p.Tags; len(got) != 2 || got[0] != "mug" || got[1] != "travel" {
		t.Fatalf("tags not normalised: %#v", got)
	}
	if got := p.Sizes; len(got) != 1 || got[0] != "14oz" {
		t.Fatalf("sizes not normalised: %#v", got)
	}
	if p.Origin != "US" {
		t.Fatalf("origin should be uppercased, got %q", p.Origin)
	}
	if p.Currency != "USD" {
		t.Fatalf("currency should be uppercased, got %q", p.Currency)
	}
	if p.RawPayloadHash != "deadbeef1234" {
		t.Fatalf("raw_payload_hash should be lowercased, got %q", p.RawPayloadHash)
	}
}

func TestNormalize_DedupesImagesPreservingOrder(t *testing.T) {
	p := sampleProduct()
	p.Normalize()
	want := []string{
		"https://cdn.example.com/a.jpg",
		"https://cdn.example.com/b.jpg",
	}
	if len(p.Images) != len(want) {
		t.Fatalf("images dedupe wrong size: %#v", p.Images)
	}
	for i := range want {
		if p.Images[i] != want[i] {
			t.Fatalf("images[%d]=%q want %q", i, p.Images[i], want[i])
		}
	}
}

func TestValidate_RejectsMissingRequiredFields(t *testing.T) {
	cases := map[string]func(*CanonicalProduct){
		"source_id":         func(p *CanonicalProduct) { p.SourceID = "" },
		"name":              func(p *CanonicalProduct) { p.Name = "" },
		"extractor_version": func(p *CanonicalProduct) { p.ExtractorVersion = "" },
		"source_updated_at": func(p *CanonicalProduct) { p.SourceUpdatedAt = time.Time{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := sampleProduct()
			mutate(p)
			if err := p.Validate(); err == nil {
				t.Fatalf("expected validate error for missing %s", name)
			}
		})
	}
}

func TestValidate_RejectsBadCurrency(t *testing.T) {
	p := sampleProduct()
	p.Normalize()
	p.Currency = "usd" // post-normalize this should not happen, but Validate must still reject it.
	if err := p.Validate(); err == nil {
		t.Fatalf("expected validate error for lowercase currency")
	}
	p.Currency = "USDD"
	if err := p.Validate(); err == nil {
		t.Fatalf("expected validate error for 4-letter currency")
	}
}

func TestValidate_RejectsInvertedPrice(t *testing.T) {
	p := sampleProduct()
	p.Normalize()
	hi := 30.0
	lo := 10.0
	p.PriceMin = &hi
	p.PriceMax = &lo
	if err := p.Validate(); err == nil {
		t.Fatalf("expected validate error for inverted price")
	}
}

func TestValidate_RejectsUnknownAvailability(t *testing.T) {
	p := sampleProduct()
	p.Normalize()
	p.Availability = "definitely-not-a-real-state"
	if err := p.Validate(); err == nil {
		t.Fatalf("expected validate error for unknown availability")
	}
}

func TestValidate_RejectsBadRawPayloadHash(t *testing.T) {
	p := sampleProduct()
	p.Normalize()
	p.RawPayloadHash = "not_hex_!"
	if err := p.Validate(); err == nil {
		t.Fatalf("expected validate error for non-hex hash")
	}
}

func TestValidate_VariantRequiresSKU(t *testing.T) {
	p := sampleProduct()
	p.Variants = []ProductVariant{{Title: "no sku"}}
	p.Normalize()
	if err := p.Validate(); err == nil {
		t.Fatalf("expected validate error for variant without sku")
	}
}

func TestToAsset_ProducesValidPODProductAsset(t *testing.T) {
	p := sampleProduct()
	p.Normalize()
	if err := p.Validate(); err != nil {
		t.Fatalf("sample product should validate: %v", err)
	}
	a, err := ToAsset(p)
	if err != nil {
		t.Fatalf("ToAsset: %v", err)
	}
	if a.Type != assets.AssetPODProduct {
		t.Fatalf("asset type = %q, want %q", a.Type, assets.AssetPODProduct)
	}
	if a.ExternalID != p.SourceID {
		t.Fatalf("external_id = %q, want %q", a.ExternalID, p.SourceID)
	}
	if a.Title != p.Name {
		t.Fatalf("title = %q, want %q", a.Title, p.Name)
	}
	if a.State != assets.StatePending {
		t.Fatalf("state = %q, want pending", a.State)
	}
	// Tags must include category, origin, availability, sizes, colors,
	// and free-form tags, normalised + sorted.
	tagSet := make(map[string]bool, len(a.Tags))
	for _, t := range a.Tags {
		tagSet[t] = true
	}
	for _, want := range []string{"home & living", "us", "in_stock", "14oz", "white", "travel", "mug"} {
		if !tagSet[want] {
			t.Fatalf("expected tag %q in %#v", want, a.Tags)
		}
	}
	// Payload must be valid JSON with schema_version set.
	var pl PayloadV1
	if err := json.Unmarshal(a.Payload, &pl); err != nil {
		t.Fatalf("payload json invalid: %v", err)
	}
	if pl.SchemaVersion != PayloadSchemaVersion {
		t.Fatalf("payload schema_version = %d, want %d", pl.SchemaVersion, PayloadSchemaVersion)
	}
	if pl.ExtractorVersion != "rest_json/v1" {
		t.Fatalf("payload extractor_version = %q, want rest_json/v1", pl.ExtractorVersion)
	}
}

func TestToAsset_TitleFallbackWhenNameEmpty(t *testing.T) {
	cases := []struct {
		name string
		p    CanonicalProduct
		want string
	}{
		{"category and sku", CanonicalProduct{Category: "Apparel", DisplaySKU: "AOP-1"}, "Apparel — AOP-1"},
		{"sku only", CanonicalProduct{DisplaySKU: "AOP-1"}, "AOP-1"},
		{"category only", CanonicalProduct{Category: "Apparel"}, "Apparel"},
		{"nothing", CanonicalProduct{}, "Untitled product"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.p.SourceID = "s"
			c.p.ExtractorVersion = "v"
			c.p.SourceUpdatedAt = time.Now()
			c.p.Availability = AvailUnknown
			c.p.Normalize()
			a, err := ToAsset(&c.p)
			if err != nil {
				t.Fatalf("ToAsset: %v", err)
			}
			if a.Title != c.want {
				t.Fatalf("title = %q, want %q", a.Title, c.want)
			}
		})
	}
}

func TestMarshalPayload_StableJSONAcrossReNormalize(t *testing.T) {
	p := sampleProduct()
	p.Normalize()
	a, err := MarshalPayload(p)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	p.Normalize()
	b, err := MarshalPayload(p)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if string(a) != string(b) {
		t.Fatalf("payload not stable across re-normalize\nfirst=%s\nsecond=%s", a, b)
	}
}

// ── Writer tests with a fake AssetWriter ───────────────────────────

type fakeWriter struct {
	got []*assets.Asset
	err error
}

func (f *fakeWriter) Write(_ context.Context, a *assets.Asset) error {
	if f.err != nil {
		return f.err
	}
	f.got = append(f.got, a)
	return nil
}

func TestWriter_Write_NormalizesValidatesAndPersists(t *testing.T) {
	fw := &fakeWriter{}
	w := NewWriter(fw)
	if w == nil {
		t.Fatal("NewWriter returned nil for non-nil inner")
	}
	p := sampleProduct() // un-normalised on purpose
	if err := w.Write(context.Background(), p); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if len(fw.got) != 1 {
		t.Fatalf("inner writer called %d times, want 1", len(fw.got))
	}
	// The product passed through should have been normalised (tags lower
	// + deduped) — proves the writer called Normalize.
	if p.Tags[0] != "mug" || p.Tags[1] != "travel" {
		t.Fatalf("writer did not normalise product, tags=%#v", p.Tags)
	}
}

func TestWriter_Write_PropagatesInnerError(t *testing.T) {
	wantErr := errors.New("boom")
	fw := &fakeWriter{err: wantErr}
	w := NewWriter(fw)
	p := sampleProduct()
	if err := w.Write(context.Background(), p); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestWriter_Write_RejectsInvalidProduct(t *testing.T) {
	fw := &fakeWriter{}
	w := NewWriter(fw)
	bad := &CanonicalProduct{
		SourceID:         "id",
		ExtractorVersion: "v1",
		// Missing Name + SourceUpdatedAt → Validate fails.
	}
	if err := w.Write(context.Background(), bad); err == nil {
		t.Fatal("expected validate error from Writer.Write")
	}
	if len(fw.got) != 0 {
		t.Fatalf("inner writer should not have been called, got %d", len(fw.got))
	}
}

func TestNewWriter_NilInnerReturnsNil(t *testing.T) {
	if NewWriter(nil) != nil {
		t.Fatal("NewWriter(nil) should return nil")
	}
}
