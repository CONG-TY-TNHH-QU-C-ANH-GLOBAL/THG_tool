package facebook

import (
	"regexp"
	"strings"

	"github.com/thg/scraper/internal/models"
)

// Match complete amounts, not a currency symbol alone: a quantity from the
// post cannot authorize an invented fee merely because a real quote uses USD.
var supplierReplyMoney = regexp.MustCompile(`(?i)(?:[¥￥$₫€£]\s*\d[\d.,]*|\d[\d.,]*\s*(?:usd|vnd|cny|rmb|yuan|eur|gbp|đồng|元))`)

// This conservative syntax guard preserves source facts/qualifications. It is
// not semantic verification; rejected wording retains the deterministic draft.
func safeSupplierReply(reply, post string, supplier *models.SupplierMatch, custom bool) bool {
	if reply == "" || len([]rune(reply)) > 900 || unsupportedReplyClaim(reply) || !containsExactReplyURL(reply, supplier.URL) {
		return false
	}
	return supplierReplyNumbersValid(reply, post, supplier) && supplierReplyLabelsPreserved(reply, post, supplier, custom) && supplierShippingReplyPreserved(reply, supplier.Shipping)
}

func supplierReplyNumbersValid(reply, post string, supplier *models.SupplierMatch) bool {
	for _, found := range podReplyURL.FindAllString(reply, -1) {
		if strings.TrimRight(found, ".,);") != supplier.URL {
			return false
		}
	}
	facts := BuildGroundedFacts(SuggestedProduct{}, supplier)
	allowed := replyNumber.FindAllString(post+" "+facts, -1)
	for _, number := range replyNumber.FindAllString(podReplyURL.ReplaceAllString(reply, ""), -1) {
		if !containsReplyNumber(allowed, number) {
			return false
		}
	}
	for _, money := range supplierReplyMoney.FindAllString(reply, -1) {
		if !supplierReplyMoneyKnown(money, supplier) {
			return false
		}
	}
	return true
}

func supplierReplyMoneyKnown(money string, supplier *models.SupplierMatch) bool {
	prices := supplier.PriceText
	if supplier.Shipping != nil {
		prices += "\n" + supplier.Shipping.PriceText
	}
	for _, allowed := range supplierReplyMoney.FindAllString(prices, -1) {
		if money == allowed {
			return true
		}
	}
	return false
}

func supplierReplyLabelsPreserved(reply, post string, supplier *models.SupplierMatch, custom bool) bool {
	if supplier.PriceText != "" && (!strings.Contains(reply, supplier.PriceText) || !containsAnyLeadPhrase(reply, []string{"reference", "tham khảo"})) {
		return false
	}
	if supplier.MOQText != "" && !strings.Contains(reply, supplier.MOQText) {
		return false
	}
	if supplier.Similar && !containsAnyLeadPhrase(reply, []string{"similar", "mẫu tương tự"}) {
		return false
	}
	if supplier.Similar && containsAnyLeadPhrase(reply, []string{"exact match", "identical", "exact same", "đúng mã", "giống hệt"}) {
		return false
	}
	if !containsAnyLeadPhrase(reply, []string{"China", "Chinese", "Trung Quốc"}) {
		return false
	}
	if containsAnyLeadPhrase(post, []string{"European", "supplier in Europe"}) && !containsAnyLeadPhrase(reply, []string{"alternative", "thay thế"}) {
		return false
	}
	if custom && !containsAnyLeadPhrase(reply, []string{"confirm customization", "customization needs confirmation", "xác nhận khả năng in", "xác nhận khả năng cá nhân hóa"}) {
		return false
	}
	return true
}

func supplierShippingReplyPreserved(reply string, shipping *models.ShippingReference) bool {
	if shipping == nil || shipping.PriceText == "" {
		return true
	}
	if !strings.Contains(reply, shipping.PriceText) || !containsAnyLeadPhrase(reply, []string{"reference shipping", "cước tham chiếu"}) {
		return false
	}
	if !containsAnyLeadPhrase(reply, []string{"per parcel", "ships separately", "gửi riêng", "/kiện"}) {
		return false
	}
	if strings.Contains(shipping.Basis, "không phải tổng cước lô") {
		return containsAnyLeadPhrase(reply, []string{"not the lot total", "not the bulk total", "chưa phải cước lô", "không phải tổng cước lô"})
	}
	return true
}
