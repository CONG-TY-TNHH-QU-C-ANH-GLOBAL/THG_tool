package render

import "testing"

// In ra tin thật để MẮT NGƯỜI đọc, không chỉ để máy so chuỗi.
//
// Chạy: go test ./internal/telegram/render/ -run TestPreviewDropshipNotice -v
//
// Giữ lại trong repo có chủ ý: khi ai đó sửa hình dạng tin, chạy một lệnh là
// thấy ngay tin sẽ trông thế nào trong Telegram — thay vì deploy rồi mới biết.
func TestPreviewDropshipNotice(t *testing.T) {
	t.Log("\n" + Lead(LeadMsg{
		Author:         "Ngọc Trâm Pet Shop",
		Heat:           "🔥 rất nóng",
		Workspace:      "THG Fulfill",
		SourceLabel:    "Nhóm Facebook — Dropship US",
		Excerpt:        "Shop mình cần nhập viên bổ khớp cho chó từ 1688 về Mỹ, mẫu này https://detail.1688.com/offer/1052769132343.html — khoảng 300 hộp/tháng, bên nào ship được báo giúp giá.",
		Reason:         "Tìm nguồn hàng + vận chuyển CN→US",
		ProductLine:    "Thực phẩm bổ sung dinh dưỡng cho khớp và hông chó · 20 CNY · MOQ 2包",
		ProductURL:     "https://detail.1688.com/offer/1052769132343.html",
		ShippingLine:   "$14.20 · 6–12 ngày làm việc",
		ShippingBasis:  "Epacket CN→US, 1 kiện 0.4 kg",
		SuggestedReply: "Ngọc Trâm Pet Shop ơi, mình thấy bạn đang tìm dịch vụ vận chuyển viên bổ khớp cho chó từ 1688 về Mỹ với khoảng 300 hộp/tháng. Tổng cước epacket cho một đơn 0.4 kg là $14.20, thời gian 6–12 ngày làm việc. Nếu cần báo giá cụ thể cho số lượng của mình, bạn nhắn tin nhé.",
		PostURL:        "https://facebook.com/groups/1312868109620530/posts/9999",
		CrmURL:         "https://crm.thgfulfill.com",
	}))
}
