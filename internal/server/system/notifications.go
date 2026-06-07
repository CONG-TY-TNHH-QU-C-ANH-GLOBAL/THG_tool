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
	if err := db.Prompts().InsertSystemPromptLog(orgID, accountID, message, action, args, success); err != nil {
		log.Printf("[AutomationEvent] could not record dashboard event org=%d account=%d action=%s: %v", orgID, accountID, action, err)
	}
}

// recordAutomationForAccount routes an account-scoped automation event (crawl
// progress / summary / failure) to the OWNING member's private copilot chat, so
// they can track everything happening on their own account — without leaking it
// to other members (consistent with PR-M5 account privacy). Falls back to the
// shared system feed only when the account has no owner (legacy / unassigned).
func recordAutomationForAccount(db *store.Store, orgID, accountID int64, message, action, args string, success bool) {
	if db == nil || orgID <= 0 {
		return
	}
	var ownerID int64
	if accountID > 0 {
		if acc, err := db.Identities().GetAccountForOrg(accountID, orgID); err == nil && acc != nil {
			ownerID = acc.AssignedUserID
		}
	}
	if ownerID > 0 {
		if err := db.Prompts().InsertUserAutomationLog(orgID, accountID, ownerID, message, action, args, success); err != nil {
			log.Printf("[AutomationEvent] user automation log failed org=%d account=%d action=%s: %v", orgID, accountID, action, err)
		}
		return
	}
	RecordDashboardAutomationEvent(db, orgID, accountID, message, action, args, success)
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

func NotifyOutboundQueued(db *store.Store, notifier func(string), orgID, accountID, id int64, typ string, state models.ExecutionState) {
	stateEN := "planned for autonomous execution"
	stateVN := "đã lên kế hoạch thực thi tự động"
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
	logMsg := fmt.Sprintf("[THG Agent] %s #%d %s. Org #%d, account #%d.", labelEN, id, stateEN, orgID, accountID)
	userMsg := fmt.Sprintf("%s %s #%d %s. Org #%d, account #%d.", notifierPrefix, labelVN, id, stateVN, orgID, accountID)
	log.Printf("[Outbound] %s", logMsg)
	// AUTONOMOUS-VERIFIED-EXECUTION (project goal, May-2026): emit
	// the four-event taxonomy that distinguishes planned / started /
	// verified / failed. Pre-this-change there were only two event
	// names — system_outbound_queued at queue and system_outbound_status
	// at terminal — and "status" lumped success and failure together.
	// The new vocabulary lets dashboards and the AI planner project
	// "what actually happened to this customer" without re-parsing
	// payloads.
	RecordDashboardAutomationEvent(db, orgID, accountID, userMsg, models.ExecutionEventPlanned, fmt.Sprintf(`{"id":%d,"type":%q,"execution_state":%q}`, id, typ, state), true)
	if notifier != nil {
		notifier(userMsg)
	}
}

// NotifyExecutionStarted is emitted when the Chrome Extension claims
// an outbound row and begins the execute path. Distinct from
// execution_planned: planned == "intent recorded"; started ==
// "extension is now mutating the live DOM".
//
// callers: agentGetOutbox right after ClaimPlannedOutboundForOrg
// succeeds.
func NotifyExecutionStarted(db *store.Store, orgID, accountID, outboundID int64, executionID string, typ string) {
	if db == nil {
		return
	}
	logMsg := fmt.Sprintf("[THG Agent] outbound #%d execution_started org=%d account=%d exec=%s", outboundID, orgID, accountID, executionID)
	userMsg := fmt.Sprintf("%s Hành động Facebook #%d bắt đầu thực thi. Org #%d, account #%d.", notifierPrefix, outboundID, orgID, accountID)
	log.Printf("[Outbound] %s", logMsg)
	RecordDashboardAutomationEvent(db, orgID, accountID, userMsg, models.ExecutionEventStarted,
		fmt.Sprintf(`{"id":%d,"type":%q,"execution_id":%q}`, outboundID, typ, executionID), true)
}

func NotifyOutboundStatus(db *store.Store, notifier func(string), orgID, id int64, state models.ExecutionState, outcome models.VerificationOutcome) {
	NotifyOutboundStatusDetail(db, notifier, orgID, id, state, outcome, "")
}

func NotifyOutboundStatusDetail(db *store.Store, notifier func(string), orgID, id int64, state models.ExecutionState, outcome models.VerificationOutcome, detail string) {
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
	// PR-2 V2: the emission splits on the (state, outcome) pair so
	// dashboards can project "did the customer hear from us?" without
	// parsing the payload. verified == finished + verified_success,
	// computed via the single-source-of-truth predicate
	// IsVerifiedSuccess.
	verified := models.IsVerifiedSuccess(state, outcome)
	statusLabel := string(state)
	if outcome != "" {
		statusLabel = string(state) + "/" + string(outcome)
	}
	logText := fmt.Sprintf("[THG Agent] Facebook %s #%d status: %s. Target: %s", msg.Type, msg.ID, statusLabel, msg.TargetName)
	userText := fmt.Sprintf("%s Facebook %s #%d trạng thái: %s. Đối tượng: %s", notifierPrefix, msg.Type, msg.ID, statusLabel, msg.TargetName)
	if detail != "" {
		logText += fmt.Sprintf(". Detail: %s", detail)
		userText += fmt.Sprintf(". Chi tiet: %s", detail)
	}
	log.Printf("[Outbound] %s", logText)
	eventName := models.ExecutionEventFailed
	if verified {
		eventName = models.ExecutionEventVerified
	}
	argsJSON := fmt.Sprintf(`{"id":%d,"type":%q,"execution_state":%q,"verification_outcome":%q,"detail":%q,"verified":%t}`, msg.ID, msg.Type, state, outcome, detail, verified)
	// PR-M5.1: attribute the outbound result to the member who initiated it so
	// "comment #X failed: <reason>" lands in THEIR private copilot chat (they need
	// to see why their own comment failed). Falls back to the shared system feed
	// only when the initiator is unknown (legacy rows). Crawl progress stays
	// system-scoped and out of the chat.
	if msg.CreatedBy > 0 {
		if err := db.Prompts().InsertUserAutomationLog(orgID, msg.AccountID, msg.CreatedBy, userText, eventName, argsJSON, verified); err != nil {
			log.Printf("[Outbound] record user automation event failed org=%d outbound=%d: %v", orgID, msg.ID, err)
		}
	} else {
		RecordDashboardAutomationEvent(db, orgID, msg.AccountID, userText, eventName, argsJSON, verified)
	}
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
	case "time_budget_exhausted":
		return "đã hết thời gian quy định cho phiên crawl (5 phút)"
	case "cursor_match":
		return "đã gặp lại bài viết của lượt trước (cursor)"
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
	recordAutomationForAccount(db, orgID, accountID, userText, "system_crawl_summary", fmt.Sprintf(`{"task_id":%q,"intent":%q,"raw_items":%d,"fetched":%d,"qualified":%d,"filtered":%d,"skipped":%d,"source_url":%q,"exit_reason":%q}`, taskID, label, totalItems, fetched, inserted, rejected, skipped, sourceURL, exitReason), true)
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
	recordAutomationForAccount(db, orgID, accountID, userText, "system_crawl_progress",
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
	recordAutomationForAccount(db, orgID, accountID, userText, "system_crawl_failure", fmt.Sprintf(`{"task_id":%q,"reason":%q}`, taskID, reason), false)
	if notifier != nil {
		notifier(userText)
	}
}
