package facebook

import (
	"fmt"

	"github.com/thg/scraper/internal/suppliersourcing"
)

// 1688 can return several prices for different order volumes. Never present
// the lowest MOQ tier as though it were the price for the lead's quantity.
func supplierSourcePriceText(product *suppliersourcing.Product, platform string, quantity int, lang string) string {
	base := formatYuan(product.Price)
	if platform != suppliersourcing.PlatformAlibaba || product.QuoteType != "by_volume" || len(product.PriceRange) == 0 {
		return base
	}
	var lowest, applicable *suppliersourcing.PriceTier
	for i := range product.PriceRange {
		tier := &product.PriceRange[i]
		price := supplierTierPrice(tier)
		if tier.MOQ <= 0 || price == nil || *price <= 0 {
			continue
		}
		if lowest == nil || tier.MOQ < lowest.MOQ {
			lowest = tier
		}
		if quantity >= tier.MOQ && (applicable == nil || tier.MOQ > applicable.MOQ) {
			applicable = tier
		}
	}
	if lowest == nil {
		return base
	}
	selected := applicable
	if selected == nil {
		selected = lowest
	}
	label := formatYuan(supplierTierPrice(selected))
	if label == "" {
		return base
	}
	if quantity == 0 {
		if lang == "en" {
			return fmt.Sprintf("%s (from MOQ %d; quantity to confirm)", label, selected.MOQ)
		}
		return fmt.Sprintf("%s (giá từ MOQ %d; cần chốt số lượng)", label, selected.MOQ)
	}
	if applicable == nil {
		if lang == "en" {
			return fmt.Sprintf("%s (MOQ %d+; not valid for %d items)", label, selected.MOQ, quantity)
		}
		return fmt.Sprintf("%s (chỉ từ MOQ %d; chưa áp dụng cho %d sản phẩm)", label, selected.MOQ, quantity)
	}
	if lang == "en" {
		return fmt.Sprintf("%s (tier %d+; variant to confirm)", label, selected.MOQ)
	}
	return fmt.Sprintf("%s (giá bậc %d+; tùy phân loại hàng)", label, selected.MOQ)
}

func supplierTierPrice(tier *suppliersourcing.PriceTier) *float64 {
	if tier.PromotionPrice != nil && *tier.PromotionPrice > 0 {
		return tier.PromotionPrice
	}
	return tier.Price
}
