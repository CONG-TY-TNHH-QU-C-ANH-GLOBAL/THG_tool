package orchestrator

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
)

// checkInboxReplies loops over all active conversation threads, scrapes each for new
// inbound messages, stores them, then generates and sends an AI follow-up reply.
func (o *Orchestrator) checkInboxReplies(ctx context.Context) error {
	threads, err := o.db.GetActiveThreads(100)
	if err != nil {
		return fmt.Errorf("get threads: %w", err)
	}
	if len(threads) == 0 {
		o.safeNotify("📭 Chưa có conversations nào đang mở.")
		return nil
	}

	var accountID int64 = -1
	if o.accountMgr != nil {
		if acc, err := o.accountMgr.GetNextAccount(models.PlatformFacebook); err == nil {
			accountID = acc.ID
		}
	}
	if accountID < 0 {
		return fmt.Errorf("no active Facebook account for scraping")
	}

	newReplies := 0
	for _, thread := range threads {
		if ctx.Err() != nil {
			break
		}

		msgs, err := o.autoCommenter.ScrapeConversation(ctx, &thread, accountID)
		if err != nil {
			log.Printf("[Orchestrator] ScrapeConversation failed thread #%d (%s): %v", thread.ID, thread.ProfileName, err)
			jitterSleep(ctx, 5*time.Second, 10*time.Second)
			continue
		}

		existing, _ := o.db.GetThreadMessages(thread.ID)
		existingSet := make(map[string]bool, len(existing))
		for _, m := range existing {
			existingSet[m.Content] = true
		}

		var newInbound []models.ConversationMessage
		for _, m := range msgs {
			if m.Direction == "inbound" && !existingSet[m.Content] {
				newInbound = append(newInbound, m)
			}
		}
		if len(newInbound) == 0 {
			log.Printf("[Orchestrator] Thread #%d (%s): no new replies", thread.ID, thread.ProfileName)
			jitterSleep(ctx, 3*time.Second, 7*time.Second)
			continue
		}

		for _, m := range newInbound {
			_ = o.db.AddThreadMessage(thread.ID, "inbound", m.Content, false)
			newReplies++
		}
		_ = o.db.UpdateThreadStatus(thread.ID, "replied")

		log.Printf("[Orchestrator] Thread #%d (%s): %d new replies found", thread.ID, thread.ProfileName, len(newInbound))

		if o.msgGen == nil {
			continue
		}

		allMsgs, _ := o.db.GetThreadMessages(thread.ID)
		var histLines []string
		for _, m := range allMsgs {
			role := "Chúng tôi"
			if m.Direction == "inbound" {
				role = thread.ProfileName
			}
			histLines = append(histLines, fmt.Sprintf("%s: %s", role, m.Content))
		}
		histStr := strings.Join(histLines, "\n")

		bizCtx := o.fetchBusinessContext(thread.Niche)
		followUp, err := o.msgGen.GenerateFollowUp(ctx, histStr, thread.ProfileName, bizCtx, thread.Niche)
		if err != nil {
			log.Printf("[Orchestrator] GenerateFollowUp failed thread #%d: %v", thread.ID, err)
			continue
		}

		msg := &models.OutboundMessage{
			Type:       "inbox",
			Platform:   thread.Platform,
			AccountID:  accountID,
			TargetURL:  thread.ProfileURL,
			TargetName: thread.ProfileName,
			Content:    followUp,
			Status:     models.OutboundApproved,
			AIModel:    "gpt-4o",
		}
		msgID, insertErr := o.db.InsertOutboundMessage(msg)
		if insertErr != nil {
			log.Printf("[Orchestrator] Insert follow-up outbound failed: %v", insertErr)
			continue
		}
		msg.ID = msgID

		if sendErr := o.retryInbox(ctx, msg); sendErr != nil {
			log.Printf("[Orchestrator] Follow-up send failed %s: %v", thread.ProfileName, sendErr)
			_ = o.db.UpdateOutboundStatus(msg.ID, models.OutboundFailed)
			o.safeNotify(fmt.Sprintf("❌ Follow-up thất bại\n👤 %s\n%v", thread.ProfileName, sendErr))
		} else {
			_ = o.db.AddThreadMessage(thread.ID, "outbound", followUp, true)
			_ = o.db.UpdateThreadStatus(thread.ID, "follow_up_sent")
			o.safeNotify(fmt.Sprintf("↩️ Đã trả lời!\n👤 %s\n💬 %s", thread.ProfileName, truncateStr(followUp, 120)))
		}

		jitterSleep(ctx, 10*time.Second, 20*time.Second)
	}

	if newReplies > 0 {
		o.safeNotify(fmt.Sprintf("✅ Check xong: %d threads | %d tin nhắn mới", len(threads), newReplies))
	} else {
		o.safeNotify(fmt.Sprintf("📭 Đã check %d conversations — chưa có ai nhắn mới.", len(threads)))
	}
	return nil
}

// scanCommentsForCandidates scrapes all comments on a post, finds job-seekers by intent signals,
// creates Lead records for each, generates @mention recruitment replies, and queues them.
// Only used for tuyen_dung niche — targets recruiter/unknown posts that attract job-seeker comments.
func (o *Orchestrator) scanCommentsForCandidates(ctx context.Context, post models.Post, accountID int64) {
	candidates, err := o.fbScraper.ScrapeJobSeekerComments(ctx, post.URL)
	if err != nil {
		log.Printf("[Orchestrator] ScrapeJobSeekerComments failed for %s: %v", post.URL, err)
		return
	}
	if len(candidates) == 0 {
		return
	}

	log.Printf("[Orchestrator] Found %d candidate comments on %s", len(candidates), post.URL)
	jobsCtx := o.buildJobsContext()

	queued := 0
	for _, c := range candidates {
		if c.AuthorURL == "" || o.db.HasSentComment(c.AuthorURL) {
			continue
		}

		scanLeadProfile := ai.LoadProfile(o.db)
		lead := models.Lead{
			SourceType:   "comment",
			SourceURL:    post.URL,
			Platform:     models.PlatformFacebook,
			Author:       c.Author,
			AuthorURL:    c.AuthorURL,
			Content:      c.Content,
			Score:        models.LeadWarm,
			AuthorRole:   "candidate",
			ServiceMatch: "recruitment",
			Niche:        scanLeadProfile.Industry,
		}
		_, _ = o.db.InsertLead(&lead)

		scanBizCtx := ai.LoadProfile(o.db).ToPromptBlock()
		var commentContent string
		if o.msgGen != nil {
			genCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			var genErr error
			commentContent, genErr = o.msgGen.GenerateRecruitmentComment(genCtx, post.Content, c.Content, c.Author, jobsCtx, scanBizCtx)
			cancel()
			if genErr != nil || commentContent == "" {
				commentContent = fmt.Sprintf("@%s Chúng tôi đang tuyển dụng vị trí phù hợp với bạn. Vui lòng nhắn tin để được tư vấn nhé!", c.Author)
			}
		} else {
			commentContent = fmt.Sprintf("@%s Chúng tôi đang tuyển dụng vị trí phù hợp với bạn. Vui lòng nhắn tin để được tư vấn nhé!", c.Author)
		}

		var imgPath string
		if img, err := o.db.GetImageForCareerJob(commentContent); err == nil {
			imgPath = img.LocalPath
			_ = o.db.IncrementImageUseCount(img.ID)
		}

		msg := &models.OutboundMessage{
			Type:       "comment_reply",
			Platform:   models.PlatformFacebook,
			AccountID:  accountID,
			TargetURL:  post.URL,
			TargetName: c.Author,
			Content:    commentContent,
			Context:    c.Content,
			ImagePath:  imgPath,
			Status:     models.OutboundApproved,
			AIModel:    "gpt-4o-mini",
		}
		if _, err := o.db.InsertOutboundMessage(msg); err == nil {
			queued++
		}
	}

	if queued > 0 {
		_, _ = o.queue.Submit(models.Job{
			Type:     models.JobAutoComment,
			Platform: models.PlatformFacebook,
			Target:   "auto",
		})
		o.safeNotify(fmt.Sprintf("💬 Tìm thấy %d ứng viên comment trên %s — đã queue %d @mention replies", len(candidates), post.URL, queued))
	}
}
