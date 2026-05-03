package ai

import (
	"regexp"
	"strings"
)

func facebookActionNotExecutedMessage() string {
	return "Mình hiểu đây là yêu cầu cần hành động trên Facebook, nhưng chưa thể chuyển thành lệnh thực thi an toàn.\n\n" +
		"Để chạy được ngay, hãy bổ sung một trong các phần còn thiếu: link group/post Facebook, account Facebook muốn dùng, hoặc hành động cụ thể như crawl, lọc leads, comment, inbox hay posting. Khi đủ dữ kiện, Copilot sẽ tạo command thật cho Browser/THG Local Kit thay vì chỉ trả lời dạng tư vấn."
}

func facebookScopeGuardMessage() string {
	return "Mình là Facebook AI Copilot của workspace này, nên chỉ xử lý các việc liên quan đến Facebook: tìm nguồn/group, crawl bài viết, lọc leads, phân tích fanpage/profile, soạn content, comment, inbox, posting, chăm sóc khách hàng và automation qua Browser/Telegram.\n\n" +
		"Câu hỏi hiện tại nằm ngoài phạm vi Facebook nên mình sẽ không dùng token để trả lời. Bạn hãy đưa nhu cầu về bối cảnh Facebook, ví dụ: “tìm tệp khách trong group này”, “phân tích fanpage này”, “soạn comment cho lead này”, hoặc “lên lịch chăm sóc profile Facebook”."
}

func polishActionResponse(action, raw, prompt string) string {
	switch action {
	case "scrape_group", "scrape_comments":
		return crawlerQueuedMessage(raw, prompt, "group/post Facebook đã chọn")
	case "search_groups":
		return crawlerQueuedMessage(raw, prompt, "tìm nguồn Facebook phù hợp")
	default:
		return raw
	}
}

func crawlerResponseFailed(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	lower := strings.ToLower(raw)
	return strings.Contains(raw, "❌") ||
		strings.Contains(raw, "âŒ") ||
		strings.Contains(lower, "error") ||
		strings.Contains(lower, "lỗi") ||
		strings.Contains(lower, "loi") ||
		strings.Contains(lower, "lá»")
}

func crawlerFailureMessage(raw string) string {
	detail := strings.TrimSpace(raw)
	if detail == "" {
		detail = "Action handler did not return a crawler command or background job id."
	}
	return "Chưa thể khởi động crawler cho lệnh này.\n\n" +
		"Chi tiết backend: " + detail + "\n\n" +
		"Bạn giữ THG Local Kit online trong tab Browser rồi gửi lại lệnh. Nếu thông báo vẫn lặp lại, dòng chi tiết ở trên là lỗi thật cần xử lý, không còn bị che bởi câu báo chung."
}

func crawlerQueuedMessage(raw, prompt, sourceLabel string) string {
	raw = strings.TrimSpace(raw)
	if crawlerResponseFailed(raw) {
		return crawlerFailureMessage(raw)
	}
	if raw == "" || strings.Contains(raw, "❌") || strings.Contains(strings.ToLower(raw), "lỗi") || strings.Contains(strings.ToLower(raw), "error") {
		return "Chưa thể khởi động crawler cho lệnh này.\n\n" +
			"Lý do kỹ thuật: backend chưa trả về mã thực thi hợp lệ từ hàng đợi crawl.\n\n" +
			"Bạn giữ THG Local Kit đang online ở tab Browser, sau đó gửi lại lệnh. Nếu vẫn lặp lại, kiểm tra terminal Runtime xem có dòng `[Input] received ... command(s)` hoặc lỗi `crawl command` để xác định Runtime có nhận lệnh chưa."
	}
	jobID := ""
	if m := regexp.MustCompile(`job #(\d+)`).FindStringSubmatch(raw); len(m) == 2 {
		jobID = m[1]
	}
	localCommandID := ""
	if m := regexp.MustCompile(`local crawler command #(\d+)`).FindStringSubmatch(raw); len(m) == 2 {
		localCommandID = m[1]
	}
	taskID := ""
	if m := regexp.MustCompile(`task=([a-zA-Z0-9_-]+)`).FindStringSubmatch(raw); len(m) == 2 {
		taskID = m[1]
	}
	var sb strings.Builder
	if localCommandID != "" {
		sb.WriteString("Đã gửi lệnh crawl xuống THG Local Runtime đang online.\n\n")
	} else {
		sb.WriteString("Đã nhận lệnh crawl và đưa vào hàng đợi xử lý.\n\n")
	}
	sb.WriteString("Mục tiêu: ")
	sb.WriteString(strings.TrimSpace(stripDashboardContext(prompt)))
	sb.WriteString("\n")
	sb.WriteString("Nguồn: ")
	sb.WriteString(sourceLabel)
	sb.WriteString("\n")
	if jobID != "" {
		sb.WriteString("Job: #")
		sb.WriteString(jobID)
		sb.WriteString("\n")
	}
	if localCommandID != "" {
		sb.WriteString("Local command: #")
		sb.WriteString(localCommandID)
		sb.WriteString("\n")
	}
	if taskID != "" {
		sb.WriteString("Task: ")
		sb.WriteString(taskID)
		sb.WriteString("\n")
	}
	sb.WriteString("\nAutomation 24/7: hệ thống sẽ ghi nhớ nhu cầu này thành lịch crawl định kỳ 30 phút cho workspace. Các vòng sau dùng scheduler và THG Local Runtime, không gọi AI lại nếu không cần phân tích/ngôn ngữ.")
	if localCommandID != "" {
		sb.WriteString("\nRuntime sẽ điều khiển Chrome Facebook thật trên thiết bị đã ghép, thu dữ liệu từ nguồn bạn đưa, lọc tín hiệu theo prompt và lưu leads đủ điều kiện về Leads. Bạn có thể quan sát luồng chạy trong tab Browser.")
	} else {
		sb.WriteString("\nHệ thống đã tạo job nền. Nếu bạn đang dùng THG Local Runtime, phản hồi chuẩn phải có `Local command`. Khi không thấy `Local command`, nghĩa là lệnh chưa được dispatch xuống Chrome local và cần kiểm tra account/session routing.")
	}
	return sb.String()
}
