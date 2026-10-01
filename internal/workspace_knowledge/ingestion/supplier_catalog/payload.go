package supplier_catalog

import (
	"github.com/thg/scraper/internal/suppliersourcing"
	"github.com/thg/scraper/internal/workspace_knowledge/suppliers"
	"strings"
	"time"
)

// payloadFrom copies the upstream record into the persisted schema. It only
// copies: nothing here estimates a weight, converts a currency, or invents a
// tier the marketplace did not publish.
func payloadFrom(product *suppliersourcing.Product, fetchedAt time.Time) suppliers.PayloadV1 {
	payload := suppliers.PayloadV1{
		Platform: product.Platform, ProductID: product.ID,
		Title: product.Title, TitleCN: product.TitleCN,
		ShopName: product.ShopName, Category: product.Category,
		PriceCNY: product.Price, PriceNote: product.PriceNote,
		MOQ: product.MOQ, Unit: product.Unit,
		WeightKG: product.WeightKG, LengthCM: product.Length,
		WidthCM: product.Width, HeightCM: product.Height,
		ShipFrom: product.ShipFrom, SoldCount: product.Sold,
		Images: product.Images, SourceURL: product.Link,
		SourceFetchedAt: fetchedAt.UTC(),
	}
	for _, tier := range product.PriceRange {
		price := tier.PromotionPrice
		if price == nil {
			price = tier.Price
		}
		if price == nil {
			continue
		}
		payload.PriceTiers = append(payload.PriceTiers, suppliers.PriceTier{MOQ: tier.MOQ, Price: *price})
	}
	return payload
}

// platformFromLink recognises which marketplace a pasted URL belongs to.
func platformFromLink(link string) string {
	lower := strings.ToLower(link)
	switch {
	case strings.Contains(lower, "1688.com"):
		return suppliersourcing.PlatformAlibaba
	case strings.Contains(lower, "taobao.com"), strings.Contains(lower, "tmall.com"), strings.Contains(lower, "tb.cn"):
		return suppliersourcing.PlatformTaobao
	default:
		return ""
	}
}
