package control

import (
	"github.com/thg/scraper/internal/models"
	"strings"
)

// supplierSummary renders the sourcing numbers as one operator-scannable strip
// ("¥28.9 · 0.29 kg · MOQ 1 件 · 广东省广州市"). Missing numbers are dropped
// rather than shown as blanks, so the strip never implies a value we don't have.
func supplierSummary(s *models.SupplierMatch) string {
	if s == nil {
		return ""
	}
	parts := make([]string, 0, 4)
	for _, value := range []string{s.PriceText, s.WeightKG} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	if moq := strings.TrimSpace(s.MOQText); moq != "" {
		parts = append(parts, "MOQ "+moq)
	}
	if from := strings.TrimSpace(s.ShipFrom); from != "" {
		parts = append(parts, from)
	}
	return strings.Join(parts, " · ")
}

// supplierName flags a title-matched offer so the operator checks the photo or
// model code before telling the lead it is the exact item.
func supplierName(s *models.SupplierMatch) string {
	if s == nil {
		return ""
	}
	if s.Name == "" || !s.Similar {
		return s.Name
	}
	return s.Name + " · mẫu tương tự, sale cần đối chiếu ảnh/mã hàng"
}

func supplierShipping(s *models.SupplierMatch) string {
	if s == nil || s.Shipping == nil || s.Shipping.PriceText == "" {
		return ""
	}
	parts := []string{s.Shipping.PriceText, s.Shipping.Basis}
	if s.Shipping.Transit != "" {
		parts = append(parts, s.Shipping.Transit)
	}
	return strings.Join(parts, " · ")
}

func supplierCapturedAt(s *models.SupplierMatch) string {
	if s == nil {
		return ""
	}
	return s.CapturedAt
}
