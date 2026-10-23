package facebook

import (
	"regexp"
	"strings"
)

// Only the text fallback uses this request shape; it does not filter crawl
// admission or decide whether the model gets to analyze a post and its images.
var productFulfillmentRequest = regexp.MustCompile(`(?i)^(?:(?:mình|tôi|shop mình|bên mình)\s+)?(?:cần tìm|cần|tìm)\s+(?:bên|đơn vị|xưởng|sup|supplier)\s+(?:(?:nhận|hỗ trợ)\s+)?(?:fulfill|fullfill|fulfillment|fulfilment)\s+(.+)$`)

func fulfillmentProductTail(post string) string {
	match := productFulfillmentRequest.FindStringSubmatch(strings.TrimSpace(post))
	if len(match) != 2 || containsAnyLeadPhrase(post, []string{"kho", "warehouse", "3pl", "storage", "prep center"}) {
		return ""
	}
	tail := strings.TrimSpace(match[1])
	for _, prefix := range []string{"đơn", "orders", "order", "store", "shop", "shopify", "etsy", "amazon"} {
		if strings.EqualFold(tail, prefix) || strings.HasPrefix(strings.ToLower(tail), prefix+" ") {
			return "" // Order handling alone is still a fulfillment service inquiry.
		}
	}
	return tail
}

func fulfillmentProductQuery(post string) string {
	tail := fulfillmentProductTail(post)
	if tail == "" {
		return ""
	}
	tail = strings.ToLower(trimSupplierProperNameTail(tail))
	for _, marker := range []string{" như ", " theo mẫu", " mẫu này", " này", " số lượng", " in logo", " in theo", "\n", ".", ",", ";", "?"} {
		if at := strings.Index(tail, marker); at >= 0 {
			tail = tail[:at]
		}
	}
	tail = strings.TrimSpace(strings.TrimSuffix(tail, " ạ"))
	tail = trimSupplierQueryContext(tail)
	tail = supplierLeadingQuantity.ReplaceAllString(tail, "")
	tail = supplierLeadingNumber.ReplaceAllString(tail, "")
	if len(strings.Fields(tail)) > 8 || !safeSupplierQuery(tail) {
		return ""
	}
	return tail
}

// A contact invitation, not proof that THG can manufacture this particular
// item. Keep procurement/production eligibility and image review with sales.
func productFulfillmentReply(post string) string {
	item := "mẫu sản phẩm bạn đang tìm"
	if query := fulfillmentProductQuery(post); query != "" {
		item = "mẫu " + query + " bạn đang tìm"
	}
	reply := "Mình có thể kiểm tra nguồn và phương án fulfill cho " + item + "."
	if leadQuantity(post) == 0 {
		return reply + " Bạn nhắn mình số lượng dự kiến để trao đổi cụ thể nhé."
	}
	return reply + " Bạn nhắn mình để trao đổi phương án phù hợp nhé."
}
