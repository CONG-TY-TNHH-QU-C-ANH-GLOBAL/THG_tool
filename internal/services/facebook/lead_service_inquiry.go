package facebook

import (
	"regexp"
	"strings"
)

// THG also sells EXPRESS (shipping lines) and WAREHOUSING (kho/fulfillment).
// A lead asking for those services gets a short draft that names the service
// and asks only for missing facts. It never quotes a rate or searches Elim.
const (
	serviceExpress     = "express"
	serviceWarehousing = "warehousing"
)

// Strong terms name a shipping provider. Weak terms only describe delivery
// ("ship hàng đi UAE") and must not override a request for a product supplier.
var expressServiceTerms = []string{
	"shipping agent", "shipping agents", "freight forwarder", "freight forwarders", "forwarder", "forwarders",
	"shipping line", "shipping company", "courier", "air freight", "sea freight", "ddp",
	"đơn vị vận chuyển", "bên vận chuyển", "line gửi hàng", "line vận chuyển", "chuyển phát", "line bay", "line biển",
	"bên ship", "bên ship hàng", "ship fba", "ship hàng fba", "fba shipping",
}
var expressDeliveryTerms = []string{"vận chuyển hàng", "gửi hàng đi", "ship hàng đi"}

var serviceNeedTerms = []string{
	"looking for", "need", "needs", "seeking", "searching for", "recommend", "recommendation", "recommendations",
	"anyone know", "who can", "cần", "tìm", "ai có", "bên nào", "ai biết", "có ai", "ai nhận",
}

// Sellers advertising their own service are not leads for a THG pitch.
var serviceOfferTerms = []string{"bên em nhận", "bên mình nhận", "chúng tôi cung cấp", "we offer", "we provide", "dm us", "inbox em"}

func serviceInquiryKind(text string) string {
	if !containsAnyLeadPhrase(text, serviceNeedTerms) || containsAnyLeadPhrase(text, serviceOfferTerms) {
		return ""
	}
	if containsAnyLeadPhrase(text, expressServiceTerms) {
		return serviceExpress
	}
	if asksForSupplier(text) {
		return ""
	}
	if containsAnyLeadPhrase(text, expressDeliveryTerms) {
		return serviceExpress
	}
	if asksForWarehouse(text) {
		return serviceWarehousing
	}
	return ""
}

func containsAnyLeadPhrase(text string, phrases []string) bool {
	for _, phrase := range phrases {
		if containsLeadPhrase(text, phrase) {
			return true
		}
	}
	return false
}

var leadShipmentVolume = regexp.MustCompile(`(?i)\d+(?:[.,]\d+)?\s*(?:kg|kgs|ký|ki lô|tấn|tons?|lbs?|cbm|kiện|thùng|cartons?|boxes|pallets?)\b`)
var leadOrderVolume = regexp.MustCompile(`(?i)\d+\s*(?:đơn|orders?|units?|pcs|sản phẩm)\b`)

// Destinations named in the post, for wording only. They never reach the
// CRM rate router, which accepts only leadDestinationCountry.
var serviceDestinations = []struct{ match, vi, en string }{
	{"mỹ", "Mỹ", "the US"}, {"usa", "Mỹ", "the US"}, {"the us", "Mỹ", "the US"}, {"united states", "Mỹ", "the US"},
	{"úc", "Úc", "Australia"}, {"australia", "Úc", "Australia"}, {"canada", "Canada", "Canada"},
	{"nước anh", "Anh", "the UK"}, {"uk", "Anh", "the UK"}, {"uae", "UAE", "the UAE"}, {"dubai", "UAE", "the UAE"},
	{"nhật", "Nhật", "Japan"}, {"japan", "Nhật", "Japan"}, {"hàn quốc", "Hàn Quốc", "Korea"}, {"korea", "Hàn Quốc", "Korea"},
	{"châu âu", "châu Âu", "Europe"}, {"europe", "châu Âu", "Europe"},
}

// Only "to/đi/sang/về X" names a destination; "agent in UAE" is where the agent is.
func serviceDestination(text string) (string, string) {
	for _, d := range serviceDestinations {
		for _, prefix := range []string{"đi ", "sang ", "về ", "to ", "to the "} {
			if containsLeadPhrase(text, prefix+d.match) {
				return d.vi, d.en
			}
		}
	}
	return "", ""
}

func serviceInquirySuggestion(leadText, author string) LeadSuggestion {
	kind := serviceInquiryKind(leadText)
	if kind == "" {
		return LeadSuggestion{}
	}
	destVI, destEN := serviceDestination(leadText)
	english := supplierQueryLanguage(leadText) == "en"
	var missing []string
	add := func(vi, en string) {
		if english {
			missing = append(missing, en)
		} else {
			missing = append(missing, vi)
		}
	}
	add("loại hàng", "the product type")
	if kind == serviceExpress {
		if !leadShipmentVolume.MatchString(leadText) {
			add("khối lượng hoặc số kiện mỗi tháng", "your monthly weight or parcel count")
		}
		if destEN == "" {
			add("nơi nhận", "the destination")
		}
	} else if !leadOrderVolume.MatchString(leadText) {
		add("số đơn mỗi tháng", "your monthly order volume")
	}
	if english {
		name := strings.TrimSpace(author)
		if name == "" {
			name = "there"
		}
		service := "THG can help with warehousing and fulfillment"
		if kind == serviceExpress {
			service = "THG can help with international shipping"
			if destEN != "" {
				service += " to " + destEN
			}
		}
		return LeadSuggestion{Reply: "Hi " + name + ", " + service + ". Could you share " + joinWithAnd(missing, "and") + " so we can suggest the right option?"}
	}
	service := "bên mình có dịch vụ kho và fulfillment"
	if kind == serviceExpress {
		service = "bên mình có dịch vụ vận chuyển quốc tế"
		if destVI != "" {
			service += " đi " + destVI
		}
	}
	return LeadSuggestion{Reply: leadSalutation(author) + ", " + service + ". Bạn cho mình xin " + joinWithAnd(missing, "và") + " để tư vấn phương án phù hợp nhé?"}
}

func joinWithAnd(items []string, and string) string {
	if len(items) <= 1 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " " + and + " " + items[len(items)-1]
}
