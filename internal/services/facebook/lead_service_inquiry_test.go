package facebook

import (
	"strings"
	"testing"
)

func TestServiceInquiriesGetServiceDrafts(t *testing.T) {
	for _, tc := range []struct{ post, want, notIn string }{
		{"Looking for a shipping agent from China to USA, around 200kg per month", "international shipping to the US", "weight"},
		{"Need a 3PL warehouse in the US for my Shopify store", "warehousing and fulfillment", "model"},
		{"Looking for an FBA prep center in the US, any recommendations?", "warehousing and fulfillment", "model"},
		{"Cần tìm đơn vị vận chuyển hàng từ Việt Nam đi Mỹ, mỗi tháng khoảng 300kg", "vận chuyển quốc tế đi Mỹ", "khối lượng"},
		{"Bên nào có kho ở Mỹ nhận fulfill đơn Etsy không ạ?", "dịch vụ kho và fulfillment", "mẫu"},
		{"Mình bán Amazon, cần tìm bên ship hàng FBA từ Trung Quốc về kho Amazon Mỹ", "vận chuyển quốc tế", "mẫu"},
		{"Ai có line gửi hàng đi Úc giá tốt không ạ", "vận chuyển quốc tế đi Úc", "nơi nhận"},
		// English post as crawled through the Vietnamese Facebook UI.
		{"Facebook Poster Name 5 phút trước · Need a freight forwarder to the UK for 3 pallets", "international shipping to the UK", "Bạn"},
	} {
		s, msg := renderedCrawlNotice(t, tc.post, "Lan")
		if !strings.Contains(s.Reply, tc.want) || strings.Contains(s.Reply, tc.notIn) || !strings.Contains(msg, "💬 Gợi ý trả lời") {
			t.Errorf("%q:\n got %q\n want %q without %q", tc.post, s.Reply, tc.want, tc.notIn)
		}
		if s.SourcingNote != "" || s.Supplier != nil || strings.ContainsAny(s.Reply, "$¥") || strings.Contains(s.Reply, "http") {
			t.Errorf("service draft must not carry sourcing status, offers or prices: %+v", s)
		}
	}
}

func TestServiceInquiryIgnoresOffersAndGreetings(t *testing.T) {
	for _, post := range []string{
		"Bên em nhận vận chuyển hàng đi Mỹ, kho tại Cali, inbox em nhé",
		"We offer freight forwarder services from China, DM us",
		"Xin chào cả nhà, hôm nay mình chia sẻ cách đóng kho hàng hiệu quả",
	} {
		if kind := serviceInquiryKind(post); kind != "" {
			t.Errorf("%q is not a service request, got %q", post, kind)
		}
	}
	if _, en := serviceDestination("Looking for a shipping agent in UAE"); en != "" {
		t.Fatalf("an agent's location is not the destination: %q", en)
	}
}

func TestPODQuantityWithProductUnitIsNotAskedAgain(t *testing.T) {
	if s, _ := renderedCrawlNotice(t, "Cần in logo lên 200 cốc sứ gửi về Mỹ", "Lan"); strings.Contains(s.Reply, "số lượng") {
		t.Fatalf("200 cốc already states the quantity: %q", s.Reply)
	}
}

func TestSupplierRequestStillWinsOverDeliveryWords(t *testing.T) {
	if kind := serviceInquiryKind("Mình đang tìm nhà cung cấp uy tín ship hàng đi UAE"); kind != "" {
		t.Fatalf("a supplier request that mentions delivery is sourcing, got %q", kind)
	}
}
