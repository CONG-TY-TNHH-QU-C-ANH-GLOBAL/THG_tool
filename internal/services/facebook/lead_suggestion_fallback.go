package facebook

import "strings"

// noOfferSourcingNote is shown to the operator, never to the lead.
const noOfferSourcingNote = "chưa tìm được sản phẩm/nguồn phù hợp — sale cần kiểm tra thủ công"

func noOfferSuggestionForLookup(leadText, author string, lookupUnavailable bool) LeadSuggestion {
	out := noOfferSuggestion(leadText, author)
	if out.SourcingNote == "" {
		return out
	}
	link, _ := leadMarketplaceURL(leadText)
	if supplierQuery(leadText) == "" && link == "" {
		out.SourcingNote = "chưa đủ mô tả sản phẩm để tìm nguồn — sale cần hỏi thêm"
	} else if lookupUnavailable {
		out.SourcingNote = "chưa tra cứu được nguồn sàn — sale cần kiểm tra thủ công"
	}
	return out
}

// noOfferSuggestion is the draft for a lead that asks for a product or source
// when neither the catalog nor Pricing Hub returned a usable match. It asks for
// the missing details instead of inventing a link, price or shipping cost.
// Posts without a product or sourcing need get no draft, so a vague logistics
// post is not answered with an off-topic question.
func noOfferSuggestion(leadText, author string) LeadSuggestion {
	if !wantsBulkSourcing(leadText) && !wantsPersonalizedPOD(leadText) && supplierQuery(leadText) == "" {
		return LeadSuggestion{}
	}
	out := LeadSuggestion{SourcingNote: noOfferSourcingNote}
	if supplierEnglishQuery(leadText) != "" {
		out.Reply = "Hi " + leadSalutation(author) + ", we can check sourcing options for this product."
		lower := strings.ToLower(leadText)
		if strings.Contains(lower, "european") || strings.Contains(lower, "supplier in europe") {
			out.Reply += " Would a China-based alternative work?"
		}
		var missing []string
		if link, _ := leadMarketplaceURL(leadText); link == "" {
			missing = append(missing, "a model number or product link if available")
		}
		if leadQuantity(leadText) == 0 {
			missing = append(missing, "quantity")
		}
		if leadDestinationCountry(leadText) == "" {
			missing = append(missing, "destination")
		}
		if len(missing) > 0 {
			out.Reply += " Please share " + strings.Join(missing, ", ") + " so we can check pricing."
		}
		return out
	}
	service := "tìm nguồn hàng"
	detail := "mã mẫu hoặc link sản phẩm nếu có"
	if wantsPersonalizedPOD(leadText) {
		service = "in theo yêu cầu"
		detail = "mẫu thiết kế nếu có"
	}
	out.Reply = leadSalutation(author) + ", bên mình có thể hỗ trợ " + service + "."
	var missing []string
	if link, _ := leadMarketplaceURL(leadText); link == "" {
		missing = append(missing, detail)
	}
	if leadQuantity(leadText) == 0 {
		missing = append(missing, "số lượng")
	}
	if leadDestinationCountry(leadText) == "" {
		missing = append(missing, "nơi nhận hàng")
	}
	if len(missing) > 0 {
		out.Reply += " Bạn cho mình xin " + strings.Join(missing, ", ") + " để kiểm tra nhé?"
	}
	return out
}
