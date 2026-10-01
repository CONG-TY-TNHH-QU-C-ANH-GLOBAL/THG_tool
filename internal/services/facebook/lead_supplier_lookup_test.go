package facebook

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/thg/scraper/internal/suppliersourcing"
)

type fakeSupplierReader struct {
	queries  []string
	langs    []string
	items    []suppliersourcing.SearchItem
	product  *suppliersourcing.Product
	details  int
	searchFn func(string) ([]suppliersourcing.SearchItem, error)
	detailFn func(string) (*suppliersourcing.Product, error)
}

func (f *fakeSupplierReader) Search(_ context.Context, query, platform string, _ int) ([]suppliersourcing.SearchItem, error) {
	f.queries = append(f.queries, platform+":"+query)
	if f.searchFn != nil {
		return f.searchFn(platform)
	}
	return f.items, nil
}
func (f *fakeSupplierReader) Detail(_ context.Context, _ string, id, _ string) (*suppliersourcing.Product, error) {
	f.details++
	if f.detailFn != nil {
		return f.detailFn(id)
	}
	return f.product, nil
}
func (f *fakeSupplierReader) SearchLocalized(ctx context.Context, query, platform string, size int, lang string) ([]suppliersourcing.SearchItem, error) {
	f.langs = append(f.langs, lang)
	return f.Search(ctx, query, platform, size)
}
func (f *fakeSupplierReader) DetailLocalized(ctx context.Context, platform, id, link, lang string) (*suppliersourcing.Product, error) {
	f.langs = append(f.langs, lang)
	return f.Detail(ctx, platform, id, link)
}

func TestSupplierQueryExtractsProductAndSkipsVaguePost(t *testing.T) {
	query := supplierQuery("Shop mình cần nhập viên bổ khớp cho chó từ 1688 về Mỹ, khoảng 300 hộp/tháng")
	if query != "viên bổ khớp cho chó" {
		t.Fatalf("unexpected query %q", query)
	}
	if got := supplierQuery("Cần nhập 300 hộp viên bổ khớp cho chó từ 1688"); got != "viên bổ khớp cho chó" {
		t.Fatalf("quantity must not pollute sourcing search, got %q", got)
	}
	if got := supplierQuery("Cần đơn vị ship hàng đi Mỹ, báo giá giúp"); got != "" {
		t.Fatalf("vague logistics post should not spend Elim quota, got %q", got)
	}
}

func TestSupplierLookupUsesOnlyMatchingMarketplaceOffer(t *testing.T) {
	price, weight, moq := 20.0, 0.4, 2
	client := &fakeSupplierReader{
		items: []suppliersourcing.SearchItem{
			{ID: "1", Title: "Áo thun cotton", Link: "https://detail.1688.com/offer/1.html"},
			{ID: "2", Title: "Viên bổ khớp cho chó", Link: "https://detail.1688.com/offer/2.html"},
		},
		product: &suppliersourcing.Product{Platform: "alibaba", ID: "2", Title: "Viên bổ khớp cho chó", Link: "https://detail.1688.com/offer/2.html", Price: &price, WeightKG: &weight, MOQ: &moq},
	}
	got, err := NewSupplierLookup(client)(context.Background(), "Cần nhập viên bổ khớp cho chó từ 1688 về Mỹ")
	if err != nil || got == nil || got.Match == nil || got.Match.URL != client.product.Link || got.Match.PriceText != "¥20" || got.WeightKG == nil || *got.WeightKG != 0.4 {
		t.Fatalf("unexpected source result %+v, %v", got, err)
	}
	if client.details != 1 || len(client.queries) != 1 {
		t.Fatalf("wanted one search and one detail, got searches=%v details=%d", client.queries, client.details)
	}
}

func TestSupplierLookupPrefersProductLinkOverSearch(t *testing.T) {
	price := 20.0
	link := "https://detail.1688.com/offer/2.html"
	client := &fakeSupplierReader{product: &suppliersourcing.Product{
		Title: "Viên bổ khớp cho chó", Link: link, Price: &price,
	}}
	post := "Cần nhập viên bổ khớp cho chó từ 1688 về Mỹ, mẫu này " + link + ". Khoảng 300 hộp/tháng"
	got, err := NewSupplierLookup(client)(context.Background(), post)
	if err != nil || got == nil || got.Match.URL != link || client.details != 1 || len(client.queries) != 0 {
		t.Fatalf("linked item should need only one detail: %+v, searches=%v details=%d err=%v", got, client.queries, client.details, err)
	}
}

func TestSupplierLookupRejectsLinkedProductWithDifferentTitle(t *testing.T) {
	price := 20.0
	link := "https://detail.1688.com/offer/2.html"
	client := &fakeSupplierReader{product: &suppliersourcing.Product{
		Title: "Áo thun cotton", Link: link, Price: &price,
	}}
	got, err := NewSupplierLookup(client)(context.Background(), "Cần nhập viên bổ khớp cho chó "+link)
	if err == nil || got != nil || len(client.queries) != 0 {
		t.Fatalf("mismatched linked product must not be offered: %+v, err=%v", got, err)
	}
}

func TestSupplierLookupRejectsLinkedProductWithDifferentID(t *testing.T) {
	price := 20.0
	client := &fakeSupplierReader{product: &suppliersourcing.Product{
		Title: "Viên bổ khớp cho chó", Link: "https://detail.1688.com/offer/3.html", Price: &price,
	}}
	got, err := NewSupplierLookup(client)(context.Background(),
		"Cần nhập viên bổ khớp cho chó https://detail.1688.com/offer/2.html")
	if err == nil || got != nil {
		t.Fatalf("pasted listing must not be called exact when detail resolves elsewhere: %+v, %v", got, err)
	}
	if !sameMarketplaceListing("https://m.1688.com/offer/2.html?from=share", "https://detail.1688.com/offer/2.html") {
		t.Fatal("mobile and desktop links with one offer ID should match")
	}
}

func TestLeadMarketplaceURLRejectsUntrustedHost(t *testing.T) {
	if link, _ := leadMarketplaceURL("https://detail.1688.com.evil.example/offer/2.html"); link != "" {
		t.Fatalf("untrusted host accepted: %s", link)
	}
	link, platform := leadMarketplaceURL("Mẫu https://item.taobao.com/item.htm?id=123, nhờ báo giá")
	if link != "https://item.taobao.com/item.htm?id=123" || platform != suppliersourcing.PlatformTaobao {
		t.Fatalf("unexpected marketplace link %q %q", link, platform)
	}
}

func TestSupplierLookupRejectsIrrelevantAndUnsafeLinks(t *testing.T) {
	client := &fakeSupplierReader{items: []suppliersourcing.SearchItem{{Title: "Áo thun cotton", Link: "https://detail.1688.com/offer/1.html"}}}
	got, err := NewSupplierLookup(client)(context.Background(), "Cần nhập viên bổ khớp cho chó từ 1688")
	if err != nil || got != nil || client.details != 0 {
		t.Fatalf("irrelevant item must not be suggested: %+v, %v", got, err)
	}
	if len(client.queries) != 1 || client.queries[0] != "alibaba:viên bổ khớp cho chó" {
		t.Fatalf("explicit 1688 request should not search Taobao, got %v", client.queries)
	}
	if marketplaceURL("https://evil.example/offer/2.html") || marketplaceURL("http://detail.1688.com/offer/2.html") {
		t.Fatal("only HTTPS marketplace product links are allowed")
	}
}

func TestSupplierLookupRespectsNamedMarketplace(t *testing.T) {
	client := &fakeSupplierReader{}
	_, err := NewSupplierLookup(client)(context.Background(), "Cần nhập áo hoodie từ Taobao")
	if err != nil || len(client.queries) != 1 || client.queries[0] != "taobao:áo hoodie" {
		t.Fatalf("Taobao request should only search Taobao: %v, err=%v", client.queries, err)
	}
	client.queries = nil
	_, err = NewSupplierLookup(client)(context.Background(), "Cần nhập áo hoodie số lượng lớn")
	if err != nil || len(client.queries) != 2 {
		t.Fatalf("unnamed marketplace should try both sources: %v, err=%v", client.queries, err)
	}
}

func TestSupplierLookupFallsBackWhenFirstMarketplaceSearchFails(t *testing.T) {
	price := 18.0
	client := &fakeSupplierReader{
		searchFn: func(platform string) ([]suppliersourcing.SearchItem, error) {
			if platform == suppliersourcing.PlatformAlibaba {
				return nil, errors.New("1688 temporarily unavailable")
			}
			return []suppliersourcing.SearchItem{{ID: "5", Title: "Áo hoodie cotton", Link: "https://item.taobao.com/item.htm?id=5"}}, nil
		},
		product: &suppliersourcing.Product{Title: "Áo hoodie cotton", Link: "https://item.taobao.com/item.htm?id=5", Price: &price},
	}
	got, err := NewSupplierLookup(client)(context.Background(), "Cần nhập áo hoodie cotton số lượng lớn")
	if err != nil || got == nil || got.Match.Platform != "Taobao" || len(client.queries) != 2 || client.details != 1 {
		t.Fatalf("Taobao fallback failed: %+v, searches=%v details=%d err=%v", got, client.queries, client.details, err)
	}
}

func TestSupplierLookupThroughPricingHubContract(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("x-thg-integration-key") != "test-key" {
			t.Errorf("missing integration authentication")
		}
		w.Header().Set("content-type", "application/json")
		switch r.URL.Path {
		case "/api/scrape/search":
			_, _ = w.Write([]byte(`{"ok":true,"items":[{"id":"2","title":"Viên bổ khớp cho chó","link":"https://detail.1688.com/offer/2.html"}]}`))
		case "/api/scrape/detail":
			_, _ = w.Write([]byte(`{"ok":true,"product":{"platform":"alibaba","id":"2","title":"Viên bổ khớp cho chó","link":"https://detail.1688.com/offer/2.html","price":20,"weight":0.4,"fetchedAt":"2026-09-30T00:00:00Z"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := suppliersourcing.NewClient(server.URL, "test-key", 2*time.Second)
	got, err := NewSupplierLookup(client)(context.Background(), "Cần nhập viên bổ khớp cho chó từ 1688 về Mỹ")
	if err != nil || got == nil || got.Match.PriceText != "¥20" || got.Match.CapturedAt != "2026-09-30T00:00:00Z" || requests != 2 {
		t.Fatalf("pricing hub contract failed: %+v, requests=%d, err=%v", got, requests, err)
	}
}

func TestSupplierLookupEnglishDropshippingPost(t *testing.T) {
	post := "Hi everyone! I'm looking for a European dropshipping supplier for this exact hand massager or a very similar model."
	if got := supplierQuery(post); got != "hand massager" {
		t.Fatalf("English query=%q", got)
	}
	price := 42.0
	client := &fakeSupplierReader{
		items:   []suppliersourcing.SearchItem{{ID: "12", Title: "Electric hand massager", Link: "https://detail.1688.com/offer/12.html"}},
		product: &suppliersourcing.Product{ID: "12", Title: "Electric hand massager", Link: "https://detail.1688.com/offer/12.html", Price: &price},
	}
	got, err := NewSupplierLookup(client)(context.Background(), post)
	if err != nil || got == nil || got.Match.PriceText != "¥42" || len(client.langs) != 2 || client.langs[0] != "en" || client.langs[1] != "en" {
		t.Fatalf("English sourcing failed: got=%+v langs=%v err=%v", got, client.langs, err)
	}
	if supplierQuery("Looking for a European dropshipping supplier") != "" {
		t.Fatal("no product noun must not trigger a sourcing search")
	}
}

func TestSupplierQueryFindsPODBlankWhenCatalogMisses(t *testing.T) {
	for _, post := range []string{
		"Cần in logo lên 300 áo hoodie đi Mỹ",
		"Cần áo hoodie in logo công ty",
		"Cần mua 300 áo hoodie in logo công ty",
	} {
		if got := supplierQuery(post); got != "áo hoodie" {
			t.Fatalf("POD product should be searchable: %q => %q", post, got)
		}
	}
	if got := supplierQuery("Cần in logo lên sản phẩm theo thiết kế"); got != "" {
		t.Fatalf("unspecified POD product should not spend Elim quota: %q", got)
	}
}

func TestSupplierLookupSkipsUnusableFirstDetailWithinBudget(t *testing.T) {
	price := 24.0
	client := &fakeSupplierReader{
		items: []suppliersourcing.SearchItem{
			{ID: "1", Title: "Viên bổ khớp cho chó", Link: "https://detail.1688.com/offer/1.html"},
			{ID: "2", Title: "Viên bổ khớp cho chó", Link: "https://detail.1688.com/offer/2.html"},
		},
		detailFn: func(id string) (*suppliersourcing.Product, error) {
			if id == "1" {
				return &suppliersourcing.Product{Title: "Viên bổ khớp cho chó", Link: "https://detail.1688.com/offer/1.html"}, nil
			}
			return &suppliersourcing.Product{Title: "Viên bổ khớp cho chó", Link: "https://detail.1688.com/offer/2.html", Price: &price}, nil
		},
	}
	got, err := NewSupplierLookup(client)(context.Background(), "Cần nhập viên bổ khớp cho chó từ 1688")
	if err != nil || got == nil || got.Match.URL != "https://detail.1688.com/offer/2.html" || client.details != 2 {
		t.Fatalf("second usable offer should be selected within detail budget: got=%+v details=%d err=%v", got, client.details, err)
	}
}

func TestSupplierQueryRejectsWrongProductQualifier(t *testing.T) {
	if matchesSupplierQuery("viên bổ khớp cho chó", "Viên bổ khớp cho mèo") {
		t.Fatal("supplier title must retain the target animal")
	}
	if !matchesSupplierQuery("viên bổ khớp cho chó", "Thực phẩm bổ sung dinh dưỡng khớp và hông chó") {
		t.Fatal("product wording may vary while identity remains")
	}
}
