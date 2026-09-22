package crmenrich

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A nil client must behave like "no enrichment configured", not panic. The
// suggestion path holds this client unconditionally, so every method is called
// on a possibly-nil receiver.
func TestNilClientIsInert(t *testing.T) {
	var c *Client
	if c.Available() {
		t.Fatal("nil client must not report available")
	}
	got := c.Enrich(context.Background(), "text", "", "hot")
	if got.HasProduct() || got.HasQuote() || got.PriceText() != "" || got.ShippingText() != "" {
		t.Fatalf("nil client must yield an empty result, got %+v", got)
	}
}

// New returns nil when either half of the config is missing, so a half-set
// deployment silently degrades to "no numbers" instead of hammering a URL with
// no key (or a key at no URL).
func TestNewRequiresBothHalves(t *testing.T) {
	for _, tc := range []struct{ url, key string }{{"", "k"}, {"https://x.test", ""}, {"", ""}} {
		if New(tc.url, tc.key) != nil {
			t.Fatalf("New(%q,%q) should be nil", tc.url, tc.key)
		}
	}
	if New("https://x.test/", "k") == nil {
		t.Fatal("fully configured New must return a client")
	}
}

// A non-200, a malformed body, or a transport failure must all degrade to an
// empty result — never an error the notification path has to handle, because a
// lead notice must go out regardless.
func TestFailuresDegradeToEmpty(t *testing.T) {
	for _, handler := range []http.HandlerFunc{
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("not json")) },
	} {
		server := httptest.NewServer(handler)
		got := New(server.URL, "k").Enrich(context.Background(), "t", "", "hot")
		server.Close()
		if got.HasProduct() || got.HasQuote() {
			t.Fatalf("failure should yield empty result, got %+v", got)
		}
	}
}

func TestEnrichRendersFigures(t *testing.T) {
	var gotKey, gotCategory string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-thg-integration-key")
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotCategory = body["category"]
		_, _ = w.Write([]byte(`{"ok":true,"product":{"title":"Viên bổ khớp","priceCny":20,"priceNote":"giá bậc MOQ 2+","weightKg":0.368,"url":"https://detail.1688.com/offer/1.html"},
			"quote":{"ok":true,"totalUsd":14.2,"currency":"USD","transit":"6–12 ngày làm việc","basis":"Epacket CN→US, 1 kiện 0.4 kg"}}`))
	}))
	defer server.Close()

	got := New(server.URL, "secret").Enrich(context.Background(), "cần ship", "https://fb.test/p/1", "hot")
	if gotKey != "secret" {
		t.Fatalf("key header not sent, got %q", gotKey)
	}
	if gotCategory != "hot" {
		t.Fatalf("category not forwarded, got %q", gotCategory)
	}
	if !got.HasProduct() || !got.HasQuote() {
		t.Fatalf("expected both product and quote, got %+v", got)
	}
	if want := "20 CNY (giá bậc MOQ 2+)"; got.PriceText() != want {
		t.Fatalf("PriceText = %q, want %q", got.PriceText(), want)
	}
	if want := "$14.20 · 6–12 ngày làm việc (Epacket CN→US, 1 kiện 0.4 kg)"; got.ShippingText() != want {
		t.Fatalf("ShippingText = %q, want %q", got.ShippingText(), want)
	}
}

// A product with no published price must not render an empty "CNY", and a
// quote the CRM marked not-ok must not render a "$0.00" — both would put a
// wrong number in front of a customer.
func TestMissingFiguresRenderNothing(t *testing.T) {
	r := Result{Product: &Product{Title: "Áo"}, Quote: &Quote{OK: false, TotalUSD: 0}}
	if r.PriceText() != "" {
		t.Fatalf("missing price must render nothing, got %q", r.PriceText())
	}
	if r.ShippingText() != "" {
		t.Fatalf("failed quote must render nothing, got %q", r.ShippingText())
	}
	if r.HasQuote() {
		t.Fatal("a not-ok quote must not count as a quote")
	}
}
