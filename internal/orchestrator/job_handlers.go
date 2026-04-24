package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
)

func (o *Orchestrator) handleScrapePostsJob(ctx context.Context, job models.Job) error {
	startTime := time.Now()

	// Find or create group
	groups, _ := o.db.GetActiveGroups(models.Platform(job.Platform))
	var targetGroup models.Group
	found := false
	for _, g := range groups {
		if g.URL == job.Target {
			targetGroup = g
			found = true
			break
		}
	}

	if !found {
		// Auto-create group entry
		targetGroup = models.Group{
			Platform:  job.Platform,
			Name:      "Auto-detected",
			URL:       job.Target,
			Active:    true,
			JoinState: "none",
		}
		id, err := o.db.AddGroup(&targetGroup)
		if err != nil {
			return fmt.Errorf("add group: %w", err)
		}
		targetGroup.ID = id
	}

	// Scrape posts
	posts, err := o.fbScraper.ScrapeGroup(ctx, targetGroup)
	if err != nil {
		if o.bot != nil {
			o.bot.Notify(fmt.Sprintf("❌ Scan lỗi: %s\n%v", targetGroup.Name, err))
		}
		return err
	}

	// Classify with AI
	var leads []models.Lead
	if o.ai != nil && len(posts) > 0 {
		// Batch classify (max 10 per batch)
		for i := 0; i < len(posts); i += 10 {
			end := i + 10
			if end > len(posts) {
				end = len(posts)
			}
			batch := posts[i:end]
			batchLeads, err := o.ai.ClassifyBatch(ctx, batch)
			if err != nil {
				log.Printf("[Orchestrator] AI classify error: %v", err)
			} else {
				leads = append(leads, batchLeads...)
			}
		}
	}

	// For recruitment businesses: scan comments on all posts for job-seeker candidates.
	// Detected from business profile industry, not hardcoded niche string.
	scanProfile := ai.LoadProfile(o.db)
	isRecruitmentScan := strings.Contains(strings.ToLower(scanProfile.Industry), "recruit")
	if !isRecruitmentScan {
		if activeNiche, _ := o.db.GetContext("active_niche"); strings.EqualFold(activeNiche, "tuyen_dung") {
			isRecruitmentScan = true
		}
	}
	if isRecruitmentScan && o.accountMgr != nil && len(posts) > 0 {
		var commentScanAccountID int64 = -1
		if acc, err := o.accountMgr.GetNextAccount(models.PlatformFacebook); err == nil {
			commentScanAccountID = acc.ID
		}
		if commentScanAccountID >= 0 {
			go func(scanPosts []models.Post, accID int64) {
				scanCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
				defer cancel()
				for _, p := range scanPosts {
					if p.URL == "" {
						continue
					}
					o.scanCommentsForCandidates(scanCtx, p, accID)
					jitterSleep(scanCtx, 5*time.Second, 10*time.Second)
				}
			}(posts, commentScanAccountID)
		}
	}

	duration := time.Since(startTime)

	scanLog := &models.ScanLog{
		Platform:   targetGroup.Platform,
		GroupCount: 1,
		PostCount:  len(posts),
		LeadCount:  len(leads),
		Duration:   int(duration.Seconds()),
	}
	_ = o.db.InsertScanLog(scanLog)

	if o.bot != nil {
		hotCount := 0
		for _, l := range leads {
			if l.Score == models.LeadHot {
				hotCount++
			}
		}
		summary := fmt.Sprintf("✅ *Scan hoàn tất: %s*\n📝 %d posts quét | 🎯 %d leads phù hợp | 🔥 %d hot\n⏱️ %s",
			targetGroup.Name, len(posts), len(leads), hotCount, duration.Round(time.Second))
		o.bot.Notify(summary)
		if len(leads) == 0 {
			o.bot.Notify("ℹ️ Không tìm thấy bài viết nào phù hợp với yêu cầu tìm kiếm")
		}
	}

	hasImages := o.db.CountCompanyImages() > 0

	var defaultAccountID int64 = -1
	if o.accountMgr != nil {
		if acc, err := o.accountMgr.GetNextAccount(models.PlatformFacebook); err == nil {
			defaultAccountID = acc.ID
		}
	}

	queuedCount := 0
	skippedCount := 0

	for i, lead := range leads {
		content := lead.Content
		content = strings.ReplaceAll(content, "*", "")
		content = strings.ReplaceAll(content, "_", "")
		content = strings.ReplaceAll(content, "`", "")

		authorDisplay := lead.Author
		if authorDisplay == "" {
			authorDisplay = "Ẩn danh"
		}

		scoreEmoji := "🟡"
		switch lead.Score {
		case models.LeadHot:
			scoreEmoji = "🔥"
		case models.LeadCold:
			scoreEmoji = "🔵"
		}

		var commentContent string
		var imagePath string

		profile := ai.LoadProfile(o.db)
		bizCtx := profile.ToPromptBlock()

		// isRecruitment is ONLY true when the lead author is explicitly a job-seeker (candidate).
		// Sellers/buyers in logistics/POD groups must not receive recruitment-style comments
		// even if the business profile also handles HR.
		isRecruitment := lead.AuthorRole == "candidate" &&
			(strings.Contains(strings.ToLower(profile.Industry), "recruit") ||
				strings.EqualFold(lead.Niche, "tuyen_dung"))

		if isRecruitment {
			if o.msgGen != nil {
				jobsCtx := o.buildJobsContext()
				var genErr error
				commentContent, genErr = o.msgGen.GenerateRecruitmentComment(ctx, lead.Content, lead.Content, lead.Author, jobsCtx, bizCtx)
				if genErr != nil || commentContent == "" {
					commentContent = fmt.Sprintf("Chào %s, chúng tôi đang có vị trí phù hợp với bạn. Bạn vui lòng nhắn tin để được tư vấn nhé!", lead.Author)
				}
			} else {
				commentContent = fmt.Sprintf("Chào %s, chúng tôi đang có vị trí phù hợp với bạn. Bạn vui lòng nhắn tin để được tư vấn nhé!", lead.Author)
			}
			if img, err := o.db.GetImageForCareerJob(commentContent); err == nil {
				imagePath = img.LocalPath
				_ = o.db.IncrementImageUseCount(img.ID)
			}
		} else {
			if o.msgGen != nil {
				var genErr error
				commentContent, genErr = o.msgGen.GenerateCommentWithService(ctx, lead.Content, lead.Author, bizCtx, lead.ServiceMatch, "")
				if genErr != nil || commentContent == "" {
					commentContent = fmt.Sprintf("Chào %s, bạn có thể liên hệ để được tư vấn thêm về dịch vụ phù hợp nhé!", lead.Author)
				}
			} else {
				commentContent = fmt.Sprintf("Chào %s, bạn có thể liên hệ để được tư vấn thêm về dịch vụ phù hợp nhé!", lead.Author)
			}
			if !strings.Contains(commentContent, "thgfulfill.com") {
				commentContent += "\n" + ai.PickServiceURL(lead.ServiceMatch)
			}
			if hasImages {
				contentWords := strings.Fields(strings.ToLower(lead.Content))
				if img, err := o.db.GetImageForService(lead.ServiceMatch, contentWords...); err == nil {
					imagePath = img.LocalPath
					_ = o.db.IncrementImageUseCount(img.ID)
				} else {
					catalogURL := ai.PickCatalogURL(lead.ServiceMatch)
					if !strings.Contains(commentContent, catalogURL) {
						commentContent += "\nXem thêm sản phẩm: " + catalogURL
					}
				}
			}
		}

		teleMsg := fmt.Sprintf("%s *LEAD %d/%d: %s*\n👤 *%s*\n💬 %s\n🏷️ Dịch vụ: %s\n📌 Vai trò: %s",
			scoreEmoji, i+1, len(leads), lead.Score, authorDisplay, content, lead.ServiceMatch, lead.AuthorRole)
		if lead.SourceURL != "" {
			teleMsg += fmt.Sprintf("\n🔗 %s", lead.SourceURL)
		}
		teleMsg += fmt.Sprintf("\n\n🤖 *AI Comment queued:* %s", commentContent)
		if imagePath != "" {
			teleMsg += fmt.Sprintf("\n🖼 *Kèm ảnh:* %s", filepath.Base(imagePath))
		}
		if o.bot != nil {
			o.bot.Notify(teleMsg)
		}

		if o.autoCommenter == nil || defaultAccountID < 0 || lead.SourceURL == "" {
			skippedCount++
			continue
		}
		if o.db.HasSentComment(lead.SourceURL) {
			skippedCount++
			continue
		}

		msg := &models.OutboundMessage{
			Type:       "comment",
			Platform:   lead.Platform,
			AccountID:  defaultAccountID,
			TargetURL:  lead.SourceURL,
			TargetName: lead.Author,
			Content:    commentContent,
			Context:    lead.Content,
			ImagePath:  imagePath,
			Status:     models.OutboundApproved,
			AIModel:    "gpt-4o-mini",
		}
		if _, err := o.db.InsertOutboundMessage(msg); err == nil {
			queuedCount++
		}
	}

	if queuedCount > 0 {
		_, _ = o.queue.Submit(models.Job{
			Type:     models.JobAutoComment,
			Platform: models.PlatformFacebook,
			Target:   "auto",
		})
		o.safeNotify(fmt.Sprintf("📬 Đã xếp hàng %d comments — xử lý tuần tự (bỏ qua: %d)", queuedCount, skippedCount))
	}

	return nil
}

// commentBatchSize controls how many comments are processed per job invocation.
// Each comment takes up to ~2 min (90s browser + retry), so 5 × 2min = 10min < 20min timeout.
const commentBatchSize = 5

// handleAutoCommentJob executes a batch of approved outbound comments with retry.
// When the batch is full it chains another job so the queue keeps draining without timeout risk.
func (o *Orchestrator) handleAutoCommentJob(ctx context.Context, job models.Job) error {
	if o.autoCommenter == nil {
		return fmt.Errorf("auto commenter not configured")
	}

	msgs, err := o.db.GetOutboundByStatus("approved", commentBatchSize)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return nil
	}

	sent, failed := 0, 0
	for _, msg := range msgs {
		m := msg
		if err := o.retryComment(ctx, &m); err != nil {
			action := "Comment"
			if m.Type == "group_post" {
				action = "Đăng bài"
			}
			log.Printf("[Orchestrator] %s failed #%d after retries: %v", action, m.ID, err)
			_ = o.db.UpdateOutboundStatus(m.ID, models.OutboundFailed)
			o.safeNotify(fmt.Sprintf("❌ %s thất bại\n🔗 %s\n%v", action, truncateStr(m.TargetURL, 60), err))
			failed++
		} else {
			if m.Type == "group_post" {
				o.safeNotify(fmt.Sprintf("✅ Đăng bài thành công!\n📋 %s\n🔗 %s",
					truncateStr(m.Content, 80), m.TargetURL))
			} else {
				o.safeNotify(fmt.Sprintf("✅ Comment đã gửi!\n👤 %s\n🔗 %s\n💬 %s",
					m.TargetName, m.TargetURL, truncateStr(m.Content, 80)))
			}
			sent++
		}
		jitterSleep(ctx, 8*time.Second, 18*time.Second)
	}

	log.Printf("[Orchestrator] Comment batch done: ✅ %d sent | ❌ %d failed", sent, failed)

	// Chain: if batch was full there may be more approved messages waiting
	if len(msgs) == commentBatchSize {
		_, _ = o.queue.Submit(models.Job{
			Type:     models.JobAutoComment,
			Platform: models.PlatformFacebook,
			Target:   "auto",
		})
	}

	return nil
}

func (o *Orchestrator) handleScrapeCommentsJob(ctx context.Context, job models.Job) error {
	var target struct {
		PostURL string `json:"post_url"`
		PostID  int64  `json:"post_id"`
	}
	if err := json.Unmarshal([]byte(job.Target), &target); err != nil {
		target.PostURL = job.Target
	}

	comments, err := o.fbScraper.ScrapeComments(ctx, target.PostURL, target.PostID)
	if err != nil {
		return err
	}

	if o.bot != nil {
		o.bot.Notify(fmt.Sprintf("💬 Cào được %d comments", len(comments)))
	}

	return nil
}
