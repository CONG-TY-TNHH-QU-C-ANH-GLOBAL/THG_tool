package facebook

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/thg/scraper/internal/suppliersourcing"
)

func TestSecondReviewPODCatalogVietnamese(t *testing.T) {
	for _, tc := range []struct{ post, title string }{
		{"Cần in logo lên cốc sứ cho công ty", "Cốc sứ in logo theo yêu cầu"},
		{"Cần in logo lên áo phông đồng phục", "Áo phông in logo theo yêu cầu"},
		{"Cần in logo lên ly sứ", "Ly sứ in hình theo yêu cầu"},
		{"Cần in logo lên mũ lưỡi trai", "Mũ lưỡi trai in logo"},
	} {
		if !matchesLeadProduct(tc.post, tc.title) {
			t.Errorf("matching company POD item omitted: %q / %q", tc.post, tc.title)
		}
	}
	if matchesLeadProduct("Cần in logo lên cốc sứ", "Áo phông in logo") {
		t.Fatal("unrelated POD item matched")
	}
}

func TestSecondReviewLookupBudgetAndQuota(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client := &fakeSupplierReader{}
	got, err := NewSupplierLookup(client)(ctx, "Cần nhập áo hoodie từ 1688")
	if got != nil || !errors.Is(err, errSupplierBudget) || len(client.queries) != 0 {
		t.Fatalf("too little time must not spend Elim calls: %+v queries=%v err=%v", got, client.queries, err)
	}
	client = &fakeSupplierReader{searchFn: func(string) ([]suppliersourcing.SearchItem, error) {
		return nil, suppliersourcing.ErrQuotaExceeded
	}}
	got, err = NewSupplierLookup(client)(context.Background(), "Cần nhập áo hoodie số lượng lớn")
	if got != nil || !errors.Is(err, suppliersourcing.ErrQuotaExceeded) || len(client.queries) != 1 {
		t.Fatalf("quota failure must stop before another platform: %+v queries=%v err=%v", got, client.queries, err)
	}
	status := classifySupplierLookupError(err, false)
	if note := noOfferSuggestionForLookup("Cần nhập áo hoodie", "Lan", status).SourcingNote; !strings.Contains(note, "hết lượt") {
		t.Fatalf("quota status omitted from operator note: %q", note)
	}
}

func TestSecondReviewLeadProvidedMarketplaceLinks(t *testing.T) {
	price := 18.0
	for _, tc := range []struct{ post, returned string }{
		{"Cần nhập viên bổ khớp cho chó https://detail.1688.com/offer/777.html", "https://detail.1688.com/offer/777.html"},
		{"Cần nhập mẫu này https://e.tb.cn/h.abc123", "https://item.taobao.com/item.htm?id=777"},
	} {
		client := &fakeSupplierReader{product: &suppliersourcing.Product{Title: "Glucosamine cho chó", Link: tc.returned, Price: &price}}
		got, err := NewSupplierLookup(client)(context.Background(), tc.post)
		if err != nil || got == nil || got.Match.Similar || len(client.queries) != 0 || client.details != 1 {
			t.Errorf("lead-provided listing should use one detail without title guess: %+v searches=%v details=%d err=%v", got, client.queries, client.details, err)
		}
	}
}

func TestSecondReviewSupplierQueryDoesNotLeakContact(t *testing.T) {
	post := "Cần mua áo hoodie nỉ liên hệ 0909123456 chị Lan"
	if got := supplierQuery(post); got != "áo hoodie nỉ" {
		t.Fatalf("query should contain only the product, got %q", got)
	}
	for _, post := range []string{
		"Cần nhập viên bổ khớp cho chó loại tốt giá rẻ, 300 hộp/tháng về Mỹ",
		"Cần nhập viên bổ khớp cho chó giao tới Mỹ, liên hệ zalo 0909123456",
		"Cần nhập áo hoodie nỉ email lan@example.com",
		"Cần nhập áo hoodie nỉ 0909123456",
	} {
		got := supplierQuery(post)
		if strings.Contains(got, "0909") || strings.Contains(got, "@") || strings.Contains(got, "liên hệ") || strings.Contains(got, "giá rẻ") || strings.Contains(got, "giao tới") {
			t.Errorf("query retained non-product text: %q => %q", post, got)
		}
	}
}

func TestSecondReviewRateAndEnglishSalutation(t *testing.T) {
	for _, product := range []string{"Áo khoác sưởi điện USB", "LED hoodie", "Rechargeable heated jacket"} {
		if isOrdinaryApparel(product) {
			t.Errorf("electrical garment given ordinary apparel rate: %q", product)
		}
	}
	reply := noOfferSuggestion("Looking for a dropshipping supplier for this hand massager", "").Reply
	if !strings.HasPrefix(reply, "Hi there,") {
		t.Fatalf("English reply without author has wrong salutation: %q", reply)
	}
}

func TestSecondReviewSupplierTitleVariants(t *testing.T) {
	for _, tc := range []struct{ query, title string }{
		{"viên bổ khớp cho chó", "Viên nhai bổ xương khớp cho chó"},
		{"hand massager", "Electric Hand Massage Machine"},
		{"hand massager", "Handheld massager deep tissue"},
	} {
		if !matchesSupplierQuery(tc.query, tc.title) {
			t.Errorf("matching marketplace title omitted: %q / %q", tc.query, tc.title)
		}
	}
	if matchesSupplierQuery("viên bổ khớp cho chó", "Viên nhai bổ xương khớp cho mèo") {
		t.Fatal("wrong target animal matched")
	}
}

func TestSecondReviewDestinationAndEnglishPOD(t *testing.T) {
	for _, post := range []string{
		"Ship 300 hoodies to the US", "Ship 300 hoodies to the USA",
		"Cần nhập áo hoodie giao tới Mỹ", "Cần nhập áo hoodie gửi Mỹ",
	} {
		if got := leadDestinationCountry(post); got != "US" {
			t.Errorf("destination omitted: %q => %q", post, got)
		}
	}
	post := "Need custom logo printing on 500 tote bags to the US"
	if !wantsPersonalizedPOD(post) || supplierQuery(post) != "tote bags" || supplierQueryLanguage(post) != "en" {
		t.Fatalf("English POD product should be isolated: %q", supplierQuery(post))
	}
}
