package facebook

import "regexp"

// This generated branch has no verified transit quote or image evidence.
// Reject durations even when the digits also appear as the buyer's quantity.
// Supplier drafts have a separate deterministic path with actual quote data.
var unquotedReplyDuration = regexp.MustCompile(`(?i)\b\d+(?:[.,]\d+)?\s*(?:[-–]\s*\d+\s*)?(?:(?:business|working)\s+days?|days?|hours?|weeks?|months?|ngày(?:\s+làm\s+việc)?|giờ|tuần|tháng|ngay|gio|tuan|thang)(?:$|[^\p{L}\p{N}])`)

func unsupportedReplyClaim(reply string) bool {
	if unquotedReplyDuration.MatchString(reply) {
		return true
	}
	return containsAnyLeadPhrase(reply, []string{
		"guaranteed delivery", "guaranteed profit", "cam kết giao", "đảm bảo lợi nhuận", "in stock", "có sẵn hàng",
		"free shipping", "miễn phí vận chuyển", "next day delivery", "same day delivery", "giao trong ngày",
		"matches your photo", "exactly matches the image", "I checked your photo", "I reviewed your image",
		"đúng mẫu trong ảnh", "đã xem ảnh", "đã đối chiếu ảnh",
	})
}
