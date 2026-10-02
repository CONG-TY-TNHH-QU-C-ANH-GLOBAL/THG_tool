package facebook

import "github.com/thg/scraper/internal/models"

// A sourced item for a personalized request is an alternative product source,
// not proof that the marketplace seller can perform customization.
func supplierReplyForIntent(author string, supplier *models.SupplierMatch, leadText string) string {
	reply := supplierFallbackReply(author, supplier, leadText)
	if !wantsPersonalizedPOD(leadText) {
		return reply
	}
	if supplierEnglishQuery(leadText) != "" {
		return reply + " This is an alternative product source; we need to confirm customization before quoting."
	}
	return reply + " Đây là nguồn sản phẩm thay thế; bên mình cần xác nhận khả năng in/cá nhân hóa trước khi báo giá."
}
