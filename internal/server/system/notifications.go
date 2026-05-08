package system

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

type crawlProgressState struct {
	lastNotify time.Time
	lastFetch  int
}

var (
	crawlProgressMu    sync.Mutex
	crawlProgressTrack = map[string]*crawlProgressState{}
)

const (
	crawlProgressMinInterval = 30 * time.Second
	crawlProgressMinDelta    = 25
)

// ShouldEmitCrawlProgress returns true when the heartbeat should produce a
// user-facing notification for the given task.
func ShouldEmitCrawlProgress(taskID string, fetched int, done bool) bool {
	crawlProgressMu.Lock()
	defer crawlProgressMu.Unlock()
	state, ok := crawlProgressTrack[taskID]
	if !ok {
		crawlProgressTrack[taskID] = &crawlProgressState{lastNotify: time.Now(), lastFetch: fetched}
		return true
	}
	if done {
		delete(crawlProgressTrack, taskID)
		return true
	}
	if time.Since(state.lastNotify) >= crawlProgressMinInterval || (fetched-state.lastFetch) >= crawlProgressMinDelta {
		state.lastNotify = time.Now()
		state.lastFetch = fetched
		return true
	}
	return false
}

func RecordDashboardAutomationEvent(db *store.Store, orgID, accountID int64, message, action, args string, success bool) {
	if db == nil || orgID <= 0 {
		return
	}
	if err := db.InsertSystemPromptLog(orgID, accountID, message, action, args, success); err != nil {
		log.Printf("[AutomationEvent] could not record dashboard event org=%d account=%d action=%s: %v", orgID, accountID, action, err)
	}
}

// Vietnamese label dictionaries used when composing user-facing notifier text.
// English log lines stay untouched so dev/ops grep pipelines keep working.
var stageVN = map[string]string{
	"started":  "đã bắt đầu",
	"scraping": "đang quét",
	"finished": "đã hoàn tất",
	"queued":   "đã vào hàng đợi",
	"failed":   "đã thất bại",
}

func stageLabelVN(stage string) string {
	if v, ok := stageVN[strings.ToLower(strings.TrimSpace(stage))]; ok {
		return v
	}
	return stage
}

const notifierPrefix = "[Trợ lý THG]"

func NotifyOutboundQueued(db *store.Store, notifier func(string), orgID, accountID, id int64, typ string, status models.OutboundStatus) {
	stateEN := "draft waiting for approval"
	stateVN := "bản nháp chờ duyệt"
	if status == models.OutboundApproved {
		stateEN = "approved for Chrome Extension execution"
		stateVN = "đã duyệt và chờ Chrome Extension thực thi"
	}
	labelEN := "Facebook outbound"
	labelVN := "Hành động Facebook"
	switch typ {
	case "comment":
		labelEN = "Facebook comment"
		labelVN = "Bình luận Facebook"
	case "inbox":
		labelEN = "Facebook inbox"
		labelVN = "Tin nhắn Facebook"
	case "group_post":
		labelEN = "Facebook posting"
		labelVN = "Bài đăng Facebook"
	}
	logMsg := fmt.Sprintf("[THG Agent] %s #%d queued as %s. Org #%d, account #%d.", labelEN, id, stateEN, orgID, accountID)
	userMsg := fmt.Sprintf("%s %s #%d %s. Org #%d, account #%d.", notifierPrefix, labelVN, id, stateVN, orgID, accountID)
	log.Printf("[Outbound] %s", logMsg)
	RecordDashboardAutomationEvent(db, orgID, accountID, userMsg, "system_outbound_queued", fmt.Sprintf(`{"id":%d,"type":%q,"status":%q}`, id, typ, status), true)
	if notifier != nil {
		notifier(userMsg)
	}
}

func NotifyOutboundStatus(db *store.Store, notifier func(string), orgID, id int64, status models.OutboundStatus) {
	NotifyOutboundStatusDetail(db, notifier, orgID, id, status, "")
}

func NotifyOutboundStatusDetail(db *store.Store, notifier func(string), orgID, id int64, status models.OutboundStatus, detail string) {
	if db == nil {
		return
	}
	msg, err := db.GetOutboundForOrg(orgID, id)
	if err != nil || msg == nil {
		return
	}
	detail = strings.TrimSpace(detail)
	if len(detail) > 240 {
		detail = detail[:240]
	}
	logText := fmt.Sprintf("[THG Agent] Facebook %s #%d status: %s. Target: %s", msg.Type, msg.ID, status, msg.TargetName)
	userText := fmt.Sprintf("%s Facebook %s #%d trạng thái: %s. Đối tượng: %s", notifierPrefix, msg.Type, msg.ID, status, msg.TargetName)
	if detail != "" {
		logText += fmt.Sprintf(". Detail: %s", detail)
		userText += fmt.Sprintf(". Chi tiet: %s", detail)
	}
	log.Printf("[Outbound] %s", logText)
	RecordDashboardAutomationEvent(db, orgID, msg.AccountID, userText, "system_outbound_status", fmt.Sprintf(`{"id":%d,"type":%q,"status":%q,"detail":%q}`, msg.ID, msg.Type, status, detail), status != models.OutboundFailed)
	if notifier != nil {
		notifier(userText)
	}
}

func crawlExitReasonLabel(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "maxitems":
		return "đã đạt số bài yêu cầu"
	case "no_progress":
		return "Facebook không tải thêm bài sau nhiều lần cuộn"
	case "no_new_items_after_scroll":
		return "đã cuộn tiếp nhưng không thấy bài mới"
	case "pass_exhausted":
		return "đã hết số vòng cuộn an toàn"
	default:
		return strings.TrimSpace(reason)
	}
}

func NotifyCrawlSummary(db *store.Store, notifier func(string), orgID, accountID int64, taskID, intent string, totalItems, fetched, inserted int, sourceURL, exitReason string) {
	label := strings.TrimSpace(intent)
	if label == "" {
		label = "facebook_crawl"
	}
	sourceURL = strings.TrimSpace(sourceURL)
	sourceVN := sourceURL
	if sourceURL == "" {
		sourceURL = "Facebook source selected by the workspace"
		sourceVN = "Nguồn Facebook do workspace chọn"
	}
	rejected := fetched - inserted
	if rejected < 0 {
		rejected = 0
	}
	skipped := totalItems - fetched
	if skipped < 0 {
		skipped = 0
	}
	outcomeEN := fmt.Sprintf("%d raw items, %d analyzable posts, %d qualified leads saved, %d filtered by Market Signal Gate, %d skipped", totalItems, fetched, inserted, rejected, skipped)
	outcomeVN := fmt.Sprintf("%d bài thô, %d bài phân tích được, %d leads đủ điều kiện đã lưu, %d bị Bộ lọc tín hiệu thị trường loại, %d bỏ qua", totalItems, fetched, inserted, rejected, skipped)
	if inserted == 0 {
		outcomeEN = fmt.Sprintf("%d raw items, %d analyzable posts, but 0 leads passed Market Signal Gate (%d filtered, %d skipped)", totalItems, fetched, rejected, skipped)
		outcomeVN = fmt.Sprintf("%d bài thô, %d bài phân tích được, nhưng không có lead nào qua Bộ lọc tín hiệu thị trường (%d bị loại, %d bỏ qua)", totalItems, fetched, rejected, skipped)
	}
	exitReason = strings.TrimSpace(exitReason)
	exitReasonVN := crawlExitReasonLabel(exitReason)
	if exitReason != "" {
		outcomeEN = fmt.Sprintf("%s. Exit reason: %s", outcomeEN, exitReason)
		outcomeVN = fmt.Sprintf("%s. Lý do dừng: %s", outcomeVN, exitReasonVN)
	}
	logText := fmt.Sprintf("[THG Agent] Crawl %s completed. Task %s. Org #%d, account #%d. %s. Source: %s", label, taskID, orgID, accountID, outcomeEN, sourceURL)
	userText := fmt.Sprintf("%s Crawl %s đã hoàn tất. Tác vụ %s. Org #%d, account #%d. %s. Nguồn: %s", notifierPrefix, label, taskID, orgID, accountID, outcomeVN, sourceVN)
	log.Printf("[ConnectorCrawl] %s", logText)
	RecordDashboardAutomationEvent(db, orgID, accountID, userText, "system_crawl_summary", fmt.Sprintf(`{"task_id":%q,"intent":%q,"raw_items":%d,"fetched":%d,"qualified":%d,"filtered":%d,"skipped":%d,"source_url":%q,"exit_reason":%q}`, taskID, label, totalItems, fetched, inserted, rejected, skipped, sourceURL, exitReason), true)
	if notifier != nil {
		notifier(userText)
	}
}

func NotifyCrawlProgress(db *store.Store, notifier func(string), orgID, accountID int64, taskID, intent, stage string, fetched, max int, sourceURL string) {
	label := strings.TrimSpace(intent)
	if label == "" {
		label = "facebook_crawl"
	}
	stage = strings.TrimSpace(stage)
	if stage == "" {
		stage = "scraping"
	}
	progress := fmt.Sprintf("%d", fetched)
	if max > 0 {
		progress = fmt.Sprintf("%d/%d", fetched, max)
	}
	source := strings.TrimSpace(sourceURL)
	sourceVN := source
	if source == "" {
		source = "(source not reported)"
		sourceVN = "(không báo cáo nguồn)"
	}
	logText := fmt.Sprintf("[THG Agent] Crawl %s in progress. Task %s. Org #%d, account #%d. Stage: %s. Progress: %s posts. Source: %s",
		label, taskID, orgID, accountID, stage, progress, source)
	userText := fmt.Sprintf("%s Crawl %s đang chạy. Tác vụ %s. Org #%d, account #%d. Trạng thái: %s. Tiến độ: %s bài. Nguồn: %s",
		notifierPrefix, label, taskID, orgID, accountID, stageLabelVN(stage), progress, sourceVN)
	log.Printf("[ConnectorCrawl] %s", logText)
	RecordDashboardAutomationEvent(db, orgID, accountID, userText, "system_crawl_progress",
		fmt.Sprintf(`{"task_id":%q,"intent":%q,"stage":%q,"fetched":%d,"max":%d,"source_url":%q}`, taskID, label, stage, fetched, max, source), true)
	if notifier != nil {
		notifier(userText)
	}
}

func NotifyCrawlFailure(db *store.Store, notifier func(string), orgID, accountID int64, taskID, reason string) {
	reason = strings.TrimSpace(reason)
	reasonVN := reason
	if reason == "" {
		reason = "Chrome Extension crawl failed without an explicit error"
		reasonVN = "Crawl qua Chrome Extension thất bại nhưng không có thông báo lỗi cụ thể"
	}
	logText := fmt.Sprintf("[THG Agent] Crawl task %s failed. Org #%d, account #%d. Reason: %s", taskID, orgID, accountID, reason)
	userText := fmt.Sprintf("%s Tác vụ crawl %s thất bại. Org #%d, account #%d. Lý do: %s", notifierPrefix, taskID, orgID, accountID, reasonVN)
	log.Printf("[ConnectorCrawl] %s", logText)
	RecordDashboardAutomationEvent(db, orgID, accountID, userText, "system_crawl_failure", fmt.Sprintf(`{"task_id":%q,"reason":%q}`, taskID, reason), false)
	if notifier != nil {
		notifier(userText)
	}
}
