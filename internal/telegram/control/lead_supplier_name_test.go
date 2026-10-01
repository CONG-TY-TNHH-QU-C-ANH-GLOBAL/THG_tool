package control

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

func TestSupplierName_FlagsSimilarOfferForOperatorCheck(t *testing.T) {
	got := supplierName(&models.SupplierMatch{Name: "Hand massager", Similar: true})
	if !strings.HasPrefix(got, "Hand massager") || !strings.Contains(got, "mẫu tương tự, sale cần đối chiếu ảnh/mã hàng") {
		t.Fatalf("similar offer must carry the check tag, got %q", got)
	}
}

func TestSupplierName_ExactOfferHasNoTag(t *testing.T) {
	if got := supplierName(&models.SupplierMatch{Name: "Hand massager"}); got != "Hand massager" {
		t.Fatalf("exact offer must render its name only, got %q", got)
	}
	if got := supplierName(nil); got != "" {
		t.Fatalf("nil match must render nothing, got %q", got)
	}
}
