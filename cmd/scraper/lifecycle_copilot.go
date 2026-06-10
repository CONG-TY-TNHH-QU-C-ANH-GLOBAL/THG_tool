package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// Lifecycle-aware copilot wording (spec: specs/LEAD_LIFECYCLE_WORK_QUEUE.md, PR-5). When
// "comment N leads" finds nothing eligible, the copilot degrades honestly: it reports what
// the org DOES have (waiting-for-reply, follow-up-due, archived) and suggests a next step
// (crawl more / enable follow-up / view archived) instead of a dead-end "0 queued".

// noEligibleCommentMessage builds the "0 queued" reply, enriched with a lifecycle-aware
// inventory + suggestion. Falls back to the bare line if the summary can't be read.
func noEligibleCommentMessage(ctx context.Context, db *store.Store, orgID int64, scanned int, skipNote string) string {
	base := fmt.Sprintf("Không tìm được lead hợp lệ để comment sau khi quét %d lead.%s", scanned, skipNote)
	sum, err := db.Leads().LeadLifecycleSummary(ctx, orgID)
	if err != nil {
		return base
	}
	return base + " " + lifecycleSuggestion(sum)
}

// lifecycleSuggestion is the pure inventory + suggestion text from a LifecycleSummary.
func lifecycleSuggestion(sum models.LifecycleSummary) string {
	inventory := []string{}
	if sum.WaitingReply > 0 {
		inventory = append(inventory, fmt.Sprintf("%d lead đang chờ phản hồi", sum.WaitingReply))
	}
	if sum.FollowupDue > 0 {
		inventory = append(inventory, fmt.Sprintf("%d lead đến hạn follow-up", sum.FollowupDue))
	}
	if sum.Archived > 0 {
		inventory = append(inventory, fmt.Sprintf("%d lead đã lưu trữ", sum.Archived))
	}

	msg := "Không còn lead mới đủ điều kiện."
	if len(inventory) > 0 {
		msg += " Có " + joinVietnamese(inventory) + "."
	}

	suggestions := []string{}
	if sum.Active == 0 {
		suggestions = append(suggestions, "cào thêm lead mới")
	}
	if sum.FollowupDue > 0 {
		suggestions = append(suggestions, "bật follow-up cho lead đến hạn")
	}
	if sum.Archived > 0 {
		suggestions = append(suggestions, "xem mục đã lưu trữ")
	}
	if len(suggestions) == 0 {
		suggestions = append(suggestions, "cào thêm lead mới")
	}
	return msg + " Gợi ý: " + strings.Join(suggestions, " / ") + "."
}

// joinVietnamese joins phrases with ", " and " và " before the last, e.g.
// ["a", "b", "c"] → "a, b và c".
func joinVietnamese(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " và " + parts[len(parts)-1]
}
