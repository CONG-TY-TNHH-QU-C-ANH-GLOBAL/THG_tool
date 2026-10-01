package facebook

import (
	"context"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/suppliersourcing"
)

func similarTestReader() *fakeSupplierReader {
	price := 12.5
	link := "https://detail.1688.com/offer/2.html"
	return &fakeSupplierReader{
		items:   []suppliersourcing.SearchItem{{ID: "2", Title: "Viên bổ khớp cho chó", Link: link}},
		product: &suppliersourcing.Product{Platform: "alibaba", ID: "2", Title: "Viên bổ khớp cho chó", Link: link, Price: &price},
	}
}

func TestSearchedOfferIsMarkedSimilarInDraft(t *testing.T) {
	post := "Cần nhập viên bổ khớp cho chó từ 1688"
	result := BuildLeadSuggestion(context.Background(), nil, nil, nil, 1, post, "Lan", nil, NewSupplierLookup(similarTestReader()))
	if result.Supplier == nil || !result.Supplier.Similar || !strings.Contains(result.Reply, "mẫu tương tự Viên bổ khớp cho chó") {
		t.Fatalf("a title-matched offer must be labelled as similar: %+v", result)
	}
}

func TestOfferFromPastedLinkIsNotMarkedAsSearchLookalike(t *testing.T) {
	post := "Cần nhập viên bổ khớp cho chó https://detail.1688.com/offer/2.html"
	result := BuildLeadSuggestion(context.Background(), nil, nil, nil, 1, post, "Lan", nil, NewSupplierLookup(similarTestReader()))
	if result.Supplier == nil || result.Supplier.Similar || strings.Contains(result.Reply, "mẫu tương tự") {
		t.Fatalf("the lead's own listing must not be labelled as a search lookalike: %+v", result)
	}
}
