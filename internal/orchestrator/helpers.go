package orchestrator

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
)

// buildJobsContext formats active career jobs into a compact string for AI comment generation.
func (o *Orchestrator) buildJobsContext() string {
	jobs, err := o.db.GetActiveCareerJobs()
	if err != nil || len(jobs) == 0 {
		return "THG đang tuyển dụng nhiều vị trí trong lĩnh vực logistics, kho vận và sales."
	}
	var lines []string
	for _, j := range jobs {
		line := "- " + j.Title
		if j.Location != "" {
			line += " (" + j.Location + ")"
		}
		if j.Requirements != "" {
			req := j.Requirements
			if len(req) > 150 {
				req = req[:150]
			}
			line += ": " + req
		}
		if j.Email != "" {
			line += " | CV: " + j.Email
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// fetchBusinessContext returns the structured business profile block for AI prompts.
// Reads from BusinessProfile (profile-driven), appending price list and open jobs when relevant.
func (o *Orchestrator) fetchBusinessContext(niche string) string {
	profile := ai.LoadProfile(o.db)
	ctx := profile.ToPromptBlock()

	if priceText := o.db.GetPriceListText(); priceText != "" {
		ctx += "\n\n" + priceText
	}

	isRecruitment := strings.Contains(strings.ToLower(profile.Industry), "recruit") ||
		strings.EqualFold(niche, "tuyen_dung")
	if isRecruitment {
		if jobs, err := o.db.GetActiveCareerJobs(); err == nil && len(jobs) > 0 {
			ctx += "\n\n--- OPEN JOB POSITIONS ---\n"
			for _, j := range jobs {
				ctx += fmt.Sprintf("- %s (%s): %s | Yêu cầu: %s | Quyền lợi: %s | Link: %s\n",
					j.Title, j.Location, j.Description, j.Requirements, j.Benefits, j.URL)
			}
		}
	}

	return ctx
}

// safeNotify gửi Telegram notification, recover nếu bot đã bị stop (tránh panic "send on closed channel").
func (o *Orchestrator) safeNotify(msg string) {
	if o.bot == nil {
		return
	}
	defer func() { recover() }()
	o.bot.Notify(msg)
}

// isAnonymousLead returns true if the lead has no usable author profile URL,
// meaning we cannot send them a direct inbox message.
func isAnonymousLead(lead models.Lead) bool {
	return lead.AuthorURL == "" || lead.Author == "" || lead.Author == "Ẩn danh"
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// retryComment executes a comment with up to 3 attempts and exponential backoff.
func (o *Orchestrator) retryComment(ctx context.Context, msg *models.OutboundMessage) error {
	delays := []time.Duration{0, 20 * time.Second, 45 * time.Second}
	var lastErr error
	for attempt, delay := range delays {
		if delay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
		if err := o.autoCommenter.Execute(ctx, msg); err != nil {
			lastErr = err
			action := "Comment"
			if msg.Type == "group_post" {
				action = "Post"
			}
			log.Printf("[Orchestrator] %s attempt %d/%d failed for #%d: %v", action, attempt+1, len(delays), msg.ID, err)
			continue
		}
		return nil
	}
	return lastErr
}

// retryInbox sends an inbox message with up to 3 attempts and backoff.
func (o *Orchestrator) retryInbox(ctx context.Context, msg *models.OutboundMessage) error {
	delays := []time.Duration{0, 25 * time.Second, 50 * time.Second}
	var lastErr error
	for attempt, delay := range delays {
		if delay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
		if err := o.autoCommenter.PostInbox(ctx, msg); err != nil {
			lastErr = err
			log.Printf("[Orchestrator] Inbox attempt %d/%d failed for #%d: %v", attempt+1, len(delays), msg.ID, err)
			continue
		}
		return nil
	}
	return lastErr
}

// jitterSleep sleeps for a uniformly random duration between min and max,
// or returns early if ctx is cancelled.
func jitterSleep(ctx context.Context, min, max time.Duration) {
	if max <= min {
		max = min + time.Second
	}
	d := min + time.Duration(rand.Int63n(int64(max-min)))
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
