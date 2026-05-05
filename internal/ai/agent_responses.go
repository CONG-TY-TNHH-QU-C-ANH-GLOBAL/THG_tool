package ai

import (
	"regexp"
	"strings"
)

func facebookActionNotExecutedMessage() string {
	return "Minh hieu day la yeu cau can hanh dong tren Facebook, nhung chua du dieu kien de chuyen thanh lenh thuc thi an toan.\n\n" +
		"Hay bo sung link group/post Facebook, account Facebook muon dung, hoac hanh dong cu the nhu crawl, loc leads, comment, inbox hay posting. Khi du dieu kien, Copilot se tao command that cho Browser/Chrome Extension thay vi chi tra loi dang tu van."
}

func facebookScopeGuardMessage() string {
	return "Minh la Facebook AI Copilot cua workspace nay, nen chi xu ly cac viec lien quan den Facebook: tim nguon/group, crawl bai viet, loc leads, phan tich fanpage/profile, comment, inbox, posting, cham soc khach hang va automation qua Browser/Telegram.\n\n" +
		"Cau hoi hien tai nam ngoai pham vi Facebook nen minh se khong dung token de tra loi. Ban hay dua nhu cau ve boi canh Facebook, vi du: \"tim tep khach trong group nay\", \"phan tich fanpage nay\", \"comment cho cac leads hot\", hoac \"len lich cham soc profile Facebook\"."
}

func polishActionResponse(action, raw, prompt string) string {
	switch action {
	case "scrape_group", "scrape_comments":
		return crawlerQueuedMessage(raw, prompt, "group/post Facebook da chon")
	case "search_groups":
		return crawlerQueuedMessage(raw, prompt, "tim nguon Facebook phu hop")
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
		strings.Contains(lower, "loi") ||
		strings.Contains(raw, "❌")
}

func crawlerFailureMessage(raw string) string {
	detail := strings.TrimSpace(raw)
	if detail == "" {
		detail = "Action handler did not return a crawler command or background job id."
	}
	return "Chua the khoi dong crawler cho lenh nay.\n\n" +
		"Chi tiet backend: " + detail + "\n\n" +
		"Hay kiem tra Browser dashboard: Chrome Extension phai online, tab Facebook da dang nhap phai san sang, roi gui lai lenh. Neu thong bao van lap lai, day la loi routing that can xu ly chu khong phai phan hoi demo."
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
		sb.WriteString("Da gui lenh crawl xuong Chrome Extension dang online.\n\n")
	} else {
		sb.WriteString("Da nhan lenh crawl va dua vao hang doi xu ly.\n\n")
	}
	sb.WriteString("Muc tieu: ")
	sb.WriteString(strings.TrimSpace(stripDashboardContext(prompt)))
	sb.WriteString("\nNguon: ")
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
	sb.WriteString("\nAutomation 24/7: he thong ghi nho nhu cau nay thanh lich crawl dinh ky 30 phut cho workspace. Cac vong sau dung scheduler va Chrome Extension, khong goi AI lai neu khong can phan tich/ngon ngu.")
	if commandID != "" {
		sb.WriteString("\nTrang thai nay moi xac nhan Chrome Extension da nhan command. Sau khi crawl xong, he thong moi ghi fetched / qualified / filtered ve dashboard va Telegram.")
	} else {
		sb.WriteString("\nHe thong da tao job nen. Neu workspace dung Chrome Extension ma khong thay Connector command, can kiem tra account/session routing.")
	}
	return sb.String()
}
