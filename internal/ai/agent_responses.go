package ai

import (
	"regexp"
	"strings"
)

func facebookActionNotExecutedMessage() string {
	return "Mình hiểu đây là yêu cầu cần hành động trên Facebook, nhưng chưa đủ dữ kiện để chuyển thành lệnh thực thi an toàn.\n\n" +
		"Hãy bổ sung link group/post Facebook, account Facebook muốn dùng, hoặc hành động cụ thể như crawl, lọc leads, comment, inbox hay posting. Khi đủ dữ kiện, Copilot sẽ tạo command thật cho Browser/Chrome Extension thay vì chỉ trả lời dạng tư vấn."
}

func facebookScopeGuardMessage() string {
	return "Mình là Facebook AI Copilot của workspace này, nên chỉ xử lý các việc liên quan đến Facebook: tìm nguồn/group, crawl bài viết, lọc leads, phân tích fanpage/profile, comment, inbox, posting, chăm sóc khách hàng và automation qua Browser/Telegram.\n\n" +
		"Câu hỏi hiện tại nằm ngoài phạm vi Facebook nên mình sẽ không dùng token để trả lời. Bạn hãy đưa nhu cầu về bối cảnh Facebook, ví dụ: \"tìm tệp khách trong group này\", \"phân tích fanpage này\", \"comment cho các leads hot\", hoặc \"lên lịch chăm sóc profile Facebook\"."
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
	return strings.Contains(lower, "error") ||
		strings.Contains(lower, "lỗi") ||
		strings.Contains(lower, "loi") ||
		strings.Contains(raw, "❌")
}

func crawlerFailureMessage(raw string) string {
	detail := strings.TrimSpace(raw)
	if detail == "" {
		detail = "Action handler did not return a crawler command or background job id."
	}
	return "Chưa thể khởi động crawler cho lệnh này.\n\n" +
		"Chi tiết backend: " + detail + "\n\n" +
		"Hãy kiểm tra Browser dashboard: Chrome Extension phải online, tab Facebook đã đăng nhập phải sẵn sàng, rồi gửi lại lệnh. Nếu thông báo vẫn lặp lại, đây là lỗi routing thật cần xử lý chứ không phải phản hồi demo."
}

func crawlerQueuedMessage(raw, prompt, sourceLabel string) string {
	raw = strings.TrimSpace(raw)
	if crawlerResponseFailed(raw) {
		return crawlerFailureMessage(raw)
	}
	commandID := ""
	if m := regexp.MustCompile(`Chrome Extension crawler command #(\d+)`).FindStringSubmatch(raw); len(m) == 2 {
		commandID = m[1]
	}
	if commandID == "" {
		if m := regexp.MustCompile(`local crawler command #(\d+)`).FindStringSubmatch(raw); len(m) == 2 {
			commandID = m[1]
		}
	}
	jobID := ""
	if m := regexp.MustCompile(`job #(\d+)`).FindStringSubmatch(raw); len(m) == 2 {
		jobID = m[1]
	}
	taskID := ""
	if m := regexp.MustCompile(`task=([a-zA-Z0-9_-]+)`).FindStringSubmatch(raw); len(m) == 2 {
		taskID = m[1]
	}

	var sb strings.Builder
	if commandID != "" {
		sb.WriteString("Đã gửi lệnh crawl xuống Chrome Extension đang online.\n\n")
	} else {
		sb.WriteString("Đã nhận lệnh crawl và đưa vào hàng đợi xử lý.\n\n")
	}
	sb.WriteString("Mục tiêu: ")
	sb.WriteString(strings.TrimSpace(stripDashboardContext(prompt)))
	sb.WriteString("\nNguồn: ")
	sb.WriteString(sourceLabel)
	sb.WriteString("\n")
	if jobID != "" {
		sb.WriteString("Job: #")
		sb.WriteString(jobID)
		sb.WriteString("\n")
	}
	if commandID != "" {
		sb.WriteString("Connector command: #")
		sb.WriteString(commandID)
		sb.WriteString("\n")
	}
	if taskID != "" {
		sb.WriteString("Task: ")
		sb.WriteString(taskID)
		sb.WriteString("\n")
	}
	sb.WriteString("\nAutomation 24/7: hệ thống ghi nhớ nhu cầu này thành lịch crawl định kỳ 30 phút cho workspace. Các vòng sau dùng scheduler và Chrome Extension, không gọi AI lại nếu không cần phân tích/ngôn ngữ.")
	if commandID != "" {
		sb.WriteString("\nChrome Extension sẽ thao tác trên Facebook thật của account đã ghép, thu dữ liệu từ nguồn bạn đưa, lọc tín hiệu theo business profile và lưu leads đủ điều kiện về dashboard.")
	} else {
		sb.WriteString("\nHệ thống đã tạo job nền. Nếu workspace dùng Chrome Extension mà không thấy Connector command, cần kiểm tra account/session routing.")
	}
	return sb.String()
}
