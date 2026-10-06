package facebook

import (
	"context"
	"errors"
	"strings"

	"github.com/thg/scraper/internal/suppliersourcing"
)

type supplierLookupStatus uint8

const (
	lookupCompleted supplierLookupStatus = iota
	lookupUnavailable
	lookupQuotaExhausted
	lookupTimedOut
)

func classifySupplierLookupError(err error, unavailable bool) supplierLookupStatus {
	if errors.Is(err, suppliersourcing.ErrQuotaExceeded) {
		return lookupQuotaExhausted
	}
	if errors.Is(err, errSupplierBudget) || errors.Is(err, context.DeadlineExceeded) {
		return lookupTimedOut
	}
	if unavailable || (err != nil && !errors.Is(err, errSupplierNoMatch)) {
		return lookupUnavailable
	}
	return lookupCompleted
}

// noOfferSourcingNote is shown to the operator, never to the lead.
const noOfferSourcingNote = "chưa tìm được sản phẩm/nguồn phù hợp — sale cần kiểm tra thủ công"

// UnavailableLeadSuggestion is used only when enrichment did not finish.
// It never claims a marketplace search completed or invents an offer.
func UnavailableLeadSuggestion(leadText, author string) LeadSuggestion {
	out := noOfferSuggestion(leadPostBody(leadText), author)
	if out.SourcingNote != "" {
		out.SourcingNote = "gợi ý chưa xử lý kịp — sale cần kiểm tra nguồn thủ công"
	}
	return out
}

func noOfferSuggestionForLookup(leadText, author string, status supplierLookupStatus) LeadSuggestion {
	out := noOfferSuggestion(leadText, author)
	if out.SourcingNote == "" {
		return out
	}
	link, _ := leadMarketplaceURL(leadText)
	if supplierQuery(leadText) == "" && link == "" {
		out.SourcingNote = "chưa đủ mô tả sản phẩm để tìm nguồn — sale cần hỏi thêm"
	} else if status == lookupQuotaExhausted {
		out.SourcingNote = "Elim đã hết lượt tra cứu — sale cần kiểm tra nguồn thủ công"
	} else if status == lookupTimedOut {
		out.SourcingNote = "hết thời gian tra cứu nguồn sàn — sale cần kiểm tra thủ công"
	} else if status == lookupUnavailable {
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
	query := supplierQuery(leadText)
	supplierRequest := asksForSupplier(leadText)
	if !wantsBulkSourcing(leadText) && !wantsPersonalizedPOD(leadText) && query == "" && !supplierRequest {
		return LeadSuggestion{}
	}
	// "Kho Mỹ nhận hàng nhập từ 1688" asks for storage, not a product to source.
	// An explicit supplier/agent request still gets a draft when it also says "warehouse".
	if link, _ := leadMarketplaceURL(leadText); query == "" && link == "" && !wantsPersonalizedPOD(leadText) && !supplierRequest && asksForWarehouse(leadText) {
		return LeadSuggestion{}
	}
	out := LeadSuggestion{SourcingNote: noOfferSourcingNote}
	if supplierQueryLanguage(leadText) == "en" {
		name := leadSalutation(author)
		if strings.TrimSpace(author) == "" {
			name = "there"
		}
		out.Reply = "Hi " + name + ", we can check sourcing options."
		lower := strings.ToLower(leadText)
		if strings.Contains(lower, "european") || strings.Contains(lower, "supplier in europe") {
			out.Reply += " Would a China-based alternative work?"
		}
		var missing []string
		if link, _ := leadMarketplaceURL(leadText); link == "" {
			if query == "" {
				missing = append(missing, "the specific product or model")
			} else {
				missing = append(missing, "a model number or product link if available")
			}
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
	} else if query == "" && supplierRequest {
		detail = "sản phẩm cụ thể bạn cần"
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
