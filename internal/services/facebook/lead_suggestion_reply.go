package facebook

import (
	"strings"

	"github.com/thg/scraper/internal/models"
)

func supplierFallbackReply(author string, supplier *models.SupplierMatch, leadText string) string {
	if supplierQueryLanguage(leadText) == "en" {
		name := leadSalutation(author)
		if strings.TrimSpace(author) == "" {
			name = "there"
		}
		article := "a similar "
		if !supplier.Similar {
			article = "the "
		}
		first := "Hi " + name + ", we found " + article + shortLeadTitle(supplier.Name) + " from China"
		if supplier.PriceText != "" {
			first += ", reference product price " + supplier.PriceText
		}
		if supplier.MOQText != "" {
			first += " (MOQ " + supplier.MOQText + ")"
		}
		if supplier.Shipping != nil && supplier.Shipping.PriceText != "" {
			first += "; reference shipping " + supplier.Shipping.PriceText
			if strings.Contains(supplier.Shipping.Basis, "không phải tổng cước lô") {
				first += " if one item ships separately (not the lot total)"
			} else {
				first += " per parcel"
			}
		}
		missingFacts := "parcel details"
		if leadQuantity(leadText) == 0 {
			missingFacts = "quantity and parcel details"
		}
		if leadDestinationCountry(leadText) == "" {
			if leadQuantity(leadText) == 0 {
				missingFacts = "quantity and destination, plus parcel details"
			} else {
				missingFacts = "destination and parcel details"
			}
		}
		if strings.Contains(strings.ToLower(leadText), "european") || strings.Contains(strings.ToLower(leadText), "supplier in europe") {
			return first + ". Would a China-based alternative work? Please share the " + missingFacts + " for a shipping quote."
		}
		return first + ". Please share the " + missingFacts + " for a shipping quote."
	}
	first := leadSalutation(author) + ", bên mình có thể tìm nguồn "
	if supplier.Similar {
		first += "mẫu tương tự "
	}
	first += shortLeadTitle(supplier.Name)
	if supplier.PriceText != "" {
		first += ", giá nguồn tham khảo " + supplier.PriceText
	}
	if supplier.MOQText != "" {
		first += " (MOQ " + supplier.MOQText + ")"
	}
	if supplier.Shipping != nil && supplier.Shipping.PriceText != "" {
		first += "; cước tham chiếu " + supplier.Shipping.PriceText
		if strings.Contains(supplier.Shipping.Basis, "không phải tổng cước lô") {
			first += "/sản phẩm nếu gửi riêng (chưa phải cước lô)"
		} else {
			first += "/kiện"
		}
		if supplier.Shipping.Transit != "" {
			first += " (" + supplier.Shipping.Transit + ")"
		}
	} else if destination := leadDestinationCountry(leadText); destination != "" {
		first += "; tuyến CN→" + destination + ", cước cần xác nhận theo quy cách kiện"
	}
	return first + ". Inbox mình để chốt báo giá nhé?"
}

func leadSalutation(author string) string {
	if name := strings.TrimSpace(author); name != "" {
		return name
	}
	return "Bạn"
}

func shortLeadTitle(title string) string {
	runes := []rune(strings.TrimSpace(title))
	if len(runes) > 100 {
		return string(runes[:100]) + "…"
	}
	return string(runes)
}
