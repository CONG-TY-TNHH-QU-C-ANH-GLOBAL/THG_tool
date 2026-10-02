package suppliersourcing

import (
	"errors"
	"testing"
)

func TestScrapeRejectionClassifiesQuota(t *testing.T) {
	if err := scrapeRejection("search", "Hết lượt gọi API và hết credit"); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("Elim 402 message must stop further lookups: %v", err)
	}
	if err := scrapeRejection("detail", "product not found"); errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("ordinary source error mistaken for quota: %v", err)
	}
}
