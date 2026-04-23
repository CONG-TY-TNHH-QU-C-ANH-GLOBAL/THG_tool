package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"path/filepath"
	"strings"
	"time"

	"github.com/thg/scraper/internal/accounts"
	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/browser"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/queue"
	"github.com/thg/scraper/internal/scraper"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/telegram"
)

// Orchestrator coordinates all system components.
type Orchestrator struct {
	db             *store.Store
	pool           *browser.Pool
	queue          *queue.Queue
	bot            *telegram.Bot
	ai             *ai.Classifier
	msgGen         *ai.MessageGenerator
	pricer         *ai.PriceExtractor
	autoCommenter  *scraper.AutoCommenter
	imgScraper     *scraper.ImageScraper
	accountMgr     *accounts.Manager
	fbScraper      *scraper.FacebookScraper
	careersScraper *scraper.CareersScraper
	stopCh         chan struct{}
}

// New creates a new orchestrator.
func New(db *store.Store, pool *browser.Pool, q *queue.Queue, bot *telegram.Bot, classifier *ai.Classifier, msgGen *ai.MessageGenerator, accountMgr *accounts.Manager, pricer *ai.PriceExtractor) *Orchestrator {
	o := &Orchestrator{
		db:         db,
		pool:       pool,
		queue:      q,
		bot:        bot,
		ai:         classifier,
		msgGen:     msgGen,
		pricer:     pricer,
		accountMgr: accountMgr,
		autoCommenter: func() *scraper.AutoCommenter {
			ac := scraper.NewAutoCommenter(db, accountMgr, pool)
			if msgGen != nil {
				ac.SetSelectorAI(ai.NewSelectorAI(msgGen))
			}
			return ac
		}(),
		imgScraper:     scraper.NewImageScraper(pool, db),
		fbScraper:      scraper.NewFacebookScraper(pool, db),
		careersScraper: scraper.NewCareersScraper(pool, db),
		stopCh:         make(chan struct{}),
	}

	// Register job handlers
	q.Register(models.JobScrapePost, o.handleScrapePostsJob)
	q.Register(models.JobScrapeComment, o.handleScrapeCommentsJob)
	q.Register(models.JobAutoComment, o.handleAutoCommentJob)

	return o
}

// Start begins the orchestrator (scheduled scanning, job processing).
func (o *Orchestrator) Start(ctx context.Context, scanInterval time.Duration) {
	// Ensure Facebook login before starting workers
	if err := o.fbScraper.EnsureLoggedIn(ctx); err != nil {
		log.Printf("[Orchestrator] ⚠️ Facebook login check failed: %v", err)
		if o.bot != nil {
			o.bot.Notify("⚠️ Facebook chưa login! Vui lòng login trong cửa sổ Chrome và restart.")
		}
	}

	// Start job queue workers
	o.queue.Start(ctx)

	// Start scheduled scanning
	if scanInterval > 0 {
		go o.scheduledScan(ctx, scanInterval)
	}

	log.Println("[Orchestrator] Started")
}

// Stop gracefully shuts down the orchestrator.
func (o *Orchestrator) Stop() {
	close(o.stopCh)
	o.queue.Stop()
	log.Println("[Orchestrator] Stopped")
}

func (o *Orchestrator) scheduledScan(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-o.stopCh:
			return
		case <-ticker.C:
			o.triggerFullScan(ctx)
		}
	}
}

func (o *Orchestrator) triggerFullScan(_ context.Context) {
	groups, err := o.db.GetActiveGroups(models.PlatformFacebook)
	if err != nil {
		log.Printf("[Orchestrator] Failed to get groups: %v", err)
		return
	}

	if len(groups) == 0 {
		return
	}

	log.Printf("[Orchestrator] Scheduled scan: %d groups", len(groups))
	if o.bot != nil {
		o.bot.Notify(fmt.Sprintf("🔄 Bắt đầu scan tự động: %d groups", len(groups)))
	}

	for _, g := range groups {
		job := models.Job{
			Type:     models.JobScrapePost,
			Platform: g.Platform,
			Target:   g.URL,
		}
		if _, err := o.queue.Submit(job); err != nil {
			log.Printf("[Orchestrator] Submit job error: %v", err)
		}
	}
}

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

	// For recruitment businesses: scan comments on all posts for job-seeker candidates
	// Detected from business profile industry, not hardcoded niche string
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

	// Log scan
	scanLog := &models.ScanLog{
		Platform:   targetGroup.Platform,
		GroupCount: 1,
		PostCount:  len(posts),
		LeadCount:  len(leads),
		Duration:   int(duration.Seconds()),
	}
	_ = o.db.InsertScanLog(scanLog)

	// Notify Summary via Telegram
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
		// Telegram: clean markdown special chars
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

		// 1. Generate AI comment — profile-driven, works for any business
		var commentContent string
		var imagePath string

		profile := ai.LoadProfile(o.db)
		bizCtx := profile.ToPromptBlock()
		isRecruitment := strings.Contains(strings.ToLower(profile.Industry), "recruit") ||
			strings.EqualFold(lead.Niche, "tuyen_dung")

		if isRecruitment {
			// Recruitment: personalized outreach comment driven by open JDs
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
			// Attach matching JD card image if available
			if img, err := o.db.GetImageForCareerJob(commentContent); err == nil {
				imagePath = img.LocalPath
				_ = o.db.IncrementImageUseCount(img.ID)
			}
		} else {
			// Universal: service-specific comment driven by business profile
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

			// 2. Pick image — logistics only
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

		// 3. Notify Telegram
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

		// 4. Queue outbound message — KHÔNG execute ngay, để job worker xử lý
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

	// Submit dedicated comment job — tách biệt khỏi scrape job, có timeout riêng
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
		// Real jitter delay — not predictable by FB bot detection
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
	// Parse job target (expected: JSON with post_url and post_id)
	var target struct {
		PostURL string `json:"post_url"`
		PostID  int64  `json:"post_id"`
	}
	if err := json.Unmarshal([]byte(job.Target), &target); err != nil {
		// Treat target as plain URL
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

// HandleAgentAction executes an action requested by the AI Agent via function calling.
func (o *Orchestrator) HandleAgentAction(action string, args map[string]any) (string, error) {
	switch action {
	case "scrape_group":
		url, _ := args["url"].(string)
		if url == "" {
			return "", fmt.Errorf("missing url parameter")
		}
		job := models.Job{
			Type:     models.JobScrapePost,
			Platform: models.PlatformFacebook,
			Target:   url,
		}
		jobID, err := o.queue.Submit(job)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Đã tạo job #%d để cào group: %s", jobID, url), nil

	case "search_groups":
		query, _ := args["query"].(string)
		if query == "" {
			return "", fmt.Errorf("missing query parameter")
		}

		// If niche is provided, save as last search intent for context
		if niche, ok := args["niche"].(string); ok && niche != "" {
			_ = o.db.SetContext("last_search_intent", niche)
			log.Printf("[Orchestrator] search_groups: intent = %s", niche)
		}

		// Use the correct scraper (fb) to search
		if o.fbScraper == nil {
			return "", fmt.Errorf("facebook scraper not available")
		}

		groups, err := o.fbScraper.SearchGroups(context.Background(), query)
		if err != nil {
			return "", fmt.Errorf("lỗi tìm kiếm FB: %v", err)
		}

		if len(groups) == 0 {
			return fmt.Sprintf("Không tìm thấy group Public nào (1K+ members) cho từ khóa: %q", query), nil
		}

		addedCount := 0
		for _, g := range groups {
			// Clean URL for db
			cleanURL := strings.Split(g.URL, "?")[0]

			// Skip duplicates early
			if o.db.GroupExistsByURL(cleanURL) {
				continue
			}

			// Add to DB
			dbGroup := &models.Group{
				Platform:  models.PlatformFacebook,
				Name:      g.Name,
				URL:       cleanURL,
				Active:    true,
				JoinState: "none",
			}
			if _, err := o.db.AddGroup(dbGroup); err == nil {
				addedCount++

				// Auto submit scrape job for the new group
				job := models.Job{
					Type:     models.JobScrapePost,
					Platform: models.PlatformFacebook,
					Target:   cleanURL,
				}
				_, _ = o.queue.Submit(job)
			}
		}

		return fmt.Sprintf("🔍 Đã tìm thấy %d public groups cho %q. Đã thêm mới %d groups vào danh sách theo dõi và bắt đầu cào auto.", len(groups), query, addedCount), nil

	case "scrape_all":
		platform, _ := args["platform"].(string)
		if platform == "" {
			platform = "facebook"
		}
		groups, err := o.db.GetActiveGroups(models.Platform(platform))
		if err != nil {
			return "", err
		}
		if len(groups) == 0 {
			return "⚠️ Chưa có group nào trong database! Hãy gửi URL group trước, ví dụ:\nhttps://www.facebook.com/groups/tên-group\nSau khi cào lần đầu, group sẽ được lưu lại để lần sau dùng 'quét tất cả'.", nil
		}
		count := 0
		for _, g := range groups {
			job := models.Job{
				Type:     models.JobScrapePost,
				Platform: g.Platform,
				Target:   g.URL,
			}
			if _, err := o.queue.Submit(job); err == nil {
				count++
			}
		}
		return fmt.Sprintf("🚀 Đã tạo %d jobs cho %d groups (%s)", count, len(groups), platform), nil

	case "scrape_comments":
		postURL, _ := args["post_url"].(string)
		if postURL == "" {
			return "", fmt.Errorf("missing post_url parameter")
		}
		job := models.Job{
			Type:     models.JobScrapeComment,
			Platform: models.PlatformFacebook,
			Target:   postURL,
		}
		jobID, err := o.queue.Submit(job)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Đã tạo job #%d để cào comments", jobID), nil

	case "check_inbox":
		return "📬 Đã lên lịch check inbox (cần account cookies)", nil

	case "add_group":
		url, _ := args["url"].(string)
		name, _ := args["name"].(string)
		if url == "" {
			return "", fmt.Errorf("missing url parameter")
		}
		if name == "" {
			name = "New Group"
		}
		group := &models.Group{
			Platform:  models.PlatformFacebook,
			Name:      name,
			URL:       url,
			Active:    true,
			JoinState: "none",
		}
		id, err := o.db.AddGroup(group)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ Đã thêm group #%d: %s", id, name), nil

	case "get_stats":
		stats, err := o.db.GetStats()
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("📊 Groups: %d active | Posts: %d (hôm nay: %d) | Leads: %d (hot: %d) | Jobs running: %d | Accounts: %d",
			stats.ActiveGroups, stats.TotalPosts, stats.TodayPosts, stats.TotalLeads, stats.HotLeads, stats.RunningJobs, stats.ActiveAccounts), nil

	case "classify_leads":
		return "🧠 Đã bắt đầu classify leads mới nhất", nil

	case "auto_comment":
		postURL, _ := args["post_url"].(string)
		if postURL == "" {
			return "", fmt.Errorf("missing post_url parameter")
		}
		targetName, _ := args["target_name"].(string)
		postContext, _ := args["context"].(string)
		accountID := int64(0)
		if aid, ok := args["account_id"].(float64); ok {
			accountID = int64(aid)
		}
		msg := &models.OutboundMessage{
			Type:       "comment",
			Platform:   models.PlatformFacebook,
			AccountID:  accountID,
			TargetURL:  postURL,
			TargetName: targetName,
			Content:    "[AI sẽ soạn nội dung khi duyệt]",
			Context:    postContext,
			Status:     models.OutboundDraft,
			AIModel:    "gpt-4o-mini",
		}
		id, err := o.db.InsertOutboundMessage(msg)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("✏️ Đã tạo draft comment #%d → Vào Dashboard 📤 Outbox để duyệt trước khi gửi", id), nil

	case "comment_all_leads":
		template, _ := args["template"].(string)
		scoreFilter, _ := args["score_filter"].(string)
		withImage, _ := args["with_image"].(bool)
		if scoreFilter == "all" {
			scoreFilter = ""
		}

		// Fetch leads
		leads, err := o.db.GetLeads(scoreFilter, 100, 0)
		if err != nil {
			return "", fmt.Errorf("get leads: %w", err)
		}

		if len(leads) == 0 {
			return "⚠️ Không có leads nào để comment", nil
		}

		hasImages := o.db.CountCompanyImages() > 0
		_ = withImage // images always used when available

		var defaultAccountID int64 = -1
		if o.accountMgr != nil {
			if acc, err := o.accountMgr.GetNextAccount(models.PlatformFacebook); err == nil {
				defaultAccountID = acc.ID
			}
		}

		// Context với timeout cho toàn bộ vòng lặp generate AI (không block vô hạn)
		genCtx, genCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer genCancel()

		created := 0
		skipped := 0

		for _, lead := range leads {
			if lead.SourceURL == "" {
				skipped++
				continue
			}
			if o.db.HasSentComment(lead.SourceURL) {
				skipped++
				continue
			}

			// Generate AI comment text
			var commentContent string
			var genErr error
			if template != "" && o.msgGen != nil {
				commentContent, genErr = o.msgGen.GenerateCommentFromTemplate(genCtx, template, lead.Content, lead.Author)
			} else if o.msgGen != nil {
				bzCtx := o.fetchBusinessContext(lead.Niche)
				commentContent, genErr = o.msgGen.GenerateCommentWithService(genCtx, lead.Content, lead.Author, bzCtx, lead.ServiceMatch, lead.Niche)
			}
			if genErr != nil || commentContent == "" {
				commentContent = template
			}
			if commentContent == "" {
				commentContent = fmt.Sprintf("Chào %s, bạn có thể liên hệ để được tư vấn thêm về dịch vụ phù hợp nhé!", lead.Author)
			}
			if !strings.Contains(commentContent, "thgfulfill.com") {
				commentContent += "\n" + ai.PickServiceURL(lead.ServiceMatch)
			}

			var imagePath string
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
				created++
			}
		}

		// Submit comment job — worker sẽ execute với retry + jitter, không block Telegram
		if created > 0 {
			_, _ = o.queue.Submit(models.Job{
				Type:     models.JobAutoComment,
				Platform: models.PlatformFacebook,
				Target:   "auto",
			})
		}

		result := fmt.Sprintf("🚀 Đã xếp hàng %d comments (AI đã soạn sẵn) — sẽ gửi tuần tự và thông báo từng kết quả", created)
		if skipped > 0 {
			result += fmt.Sprintf(" | bỏ qua %d (thiếu link/đã comment)", skipped)
		}
		if hasImages {
			result += " | 🖼️ có kèm ảnh thực tế"
		}
		if defaultAccountID < 0 {
			result = fmt.Sprintf("⚠️ Không có account active. Đã soạn %d comments (draft, chưa gửi).", created)
		}
		return result, nil

	case "auto_inbox":
		targetURL, _ := args["target_url"].(string)
		if targetURL == "" {
			return "", fmt.Errorf("missing target_url parameter")
		}
		targetName, _ := args["target_name"].(string)
		inboxContext, _ := args["context"].(string)
		accountID := int64(0)
		if aid, ok := args["account_id"].(float64); ok {
			accountID = int64(aid)
		}

		// Generate inbox message immediately with AI
		var inboxContent string
		if o.msgGen != nil {
			inboxBizCtx := ai.LoadProfile(o.db).ToPromptBlock()
			inboxContent, _ = o.msgGen.GenerateInboxMessage(context.Background(), inboxContext, targetName, inboxBizCtx, "")
		}
		if inboxContent == "" {
			inboxContent = fmt.Sprintf("Chào %s, mình thấy bài viết của bạn rất phù hợp với dịch vụ của chúng tôi. Bạn có muốn mình tư vấn thêm không?", targetName)
		}

		msg := &models.OutboundMessage{
			Type:       "inbox",
			Platform:   models.PlatformFacebook,
			AccountID:  accountID,
			TargetURL:  targetURL,
			TargetName: targetName,
			Content:    inboxContent,
			Context:    inboxContext,
			Status:     models.OutboundApproved,
			AIModel:    "gpt-4o",
		}
		id, err := o.db.InsertOutboundMessage(msg)
		if err != nil {
			return "", err
		}
		msg.ID = id

		// Auto-execute inbox trong goroutine riêng — có timeout và retry
		if o.autoCommenter != nil && accountID >= 0 {
			go func(m models.OutboundMessage) {
				singleCtx, singleCancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer singleCancel()
				if execErr := o.retryInbox(singleCtx, &m); execErr != nil {
					log.Printf("[Orchestrator] Inbox failed for %s: %v", m.TargetURL, execErr)
					_ = o.db.UpdateOutboundStatus(m.ID, models.OutboundFailed)
					o.safeNotify(fmt.Sprintf("❌ Inbox thất bại: %s\n%v", m.TargetName, execErr))
				} else {
					o.safeNotify(fmt.Sprintf("✅ Inbox đã gửi!\n👤 %s\n💬 %s", m.TargetName, truncateStr(m.Content, 100)))
				}
			}(*msg)
		}
		return fmt.Sprintf("🚀 Đang gửi inbox tới %s (#%d)", targetName, id), nil

	case "inbox_all_leads":
		scoreFilter, _ := args["score_filter"].(string)
		if scoreFilter == "all" {
			scoreFilter = ""
		}
		if scoreFilter == "" {
			scoreFilter = "hot" // mặc định chỉ inbox hot leads
		}

		leads, err := o.db.GetLeads(scoreFilter, 200, 0)
		if err != nil {
			return "", fmt.Errorf("get leads: %w", err)
		}
		if len(leads) == 0 {
			return fmt.Sprintf("⚠️ Không có leads nào (filter: %s)", scoreFilter), nil
		}

		// Phân loại: có profile URL (inbox được) vs ẩn danh (bỏ qua inbox)
		type inboxTarget struct {
			lead models.Lead
		}
		var targets []inboxTarget
		anonymousCount := 0
		alreadySentCount := 0

		for _, lead := range leads {
			if isAnonymousLead(lead) {
				anonymousCount++
				log.Printf("[Orchestrator] Skip inbox (ẩn danh): %q authorURL=%q", lead.Author, lead.AuthorURL)
				continue
			}
			if o.db.HasSentInbox(lead.AuthorURL) {
				alreadySentCount++
				log.Printf("[Orchestrator] Skip inbox (đã inbox): %s", lead.AuthorURL)
				continue
			}
			targets = append(targets, inboxTarget{lead: lead})
		}

		if len(targets) == 0 {
			summary := fmt.Sprintf("⚠️ Không có leads nào để inbox (ẩn danh: %d, đã inbox rồi: %d)", anonymousCount, alreadySentCount)
			return summary, nil
		}

		// Context is fetched dynamically based on niche inside the worker

		var defaultAccountID int64 = -1
		if o.accountMgr != nil {
			if acc, err := o.accountMgr.GetNextAccount(models.PlatformFacebook); err == nil {
				defaultAccountID = acc.ID
			}
		}

		// Thông báo ngay rồi xử lý nền — không block Telegram
		o.safeNotify(fmt.Sprintf(
			"📬 Bắt đầu inbox %d leads (bỏ qua: %d ẩn danh, %d đã inbox)\nFilter: %s | Model: gpt-4o",
			len(targets), anonymousCount, alreadySentCount, scoreFilter,
		))

		// Tạo context riêng cho inbox batch — timeout 45 phút, không phụ thuộc request context
		inboxCtx, inboxCancel := context.WithTimeout(context.Background(), 45*time.Minute)

		go func(tgts []inboxTarget, accID int64) {
			defer inboxCancel()
			sent, failed := 0, 0

			for _, t := range tgts {
				lead := t.lead

				// Generate tin nhắn riêng từng lead bằng gpt-4o
				var inboxContent string
				if o.msgGen != nil {
					bizCtx := o.fetchBusinessContext(lead.Niche)
					inboxContent, _ = o.msgGen.GenerateInboxMessage(inboxCtx, lead.Content, lead.Author, bizCtx, lead.Niche)
				}
				if inboxContent == "" {
					inboxContent = fmt.Sprintf("Chào %s, mình thấy bài viết của bạn rất phù hợp với dịch vụ THG. Bạn có muốn mình tư vấn thêm không?", lead.Author)
				}

				msg := &models.OutboundMessage{
					Type:       "inbox",
					Platform:   lead.Platform,
					AccountID:  accID,
					TargetURL:  lead.AuthorURL,
					TargetName: lead.Author,
					Content:    inboxContent,
					Context:    lead.Content,
					Status:     models.OutboundApproved,
					AIModel:    "gpt-4o",
				}
				msgID, err := o.db.InsertOutboundMessage(msg)
				if err != nil {
					log.Printf("[Orchestrator] Insert inbox msg failed: %v", err)
					failed++
					continue
				}
				msg.ID = msgID

				if o.autoCommenter == nil {
					log.Printf("[Orchestrator] autoCommenter nil, skip PostInbox for %s", lead.AuthorURL)
					failed++
					continue
				}

				if execErr := o.retryInbox(inboxCtx, msg); execErr != nil {
					log.Printf("[Orchestrator] Inbox failed %s: %v", lead.AuthorURL, execErr)
					_ = o.db.UpdateOutboundStatus(msg.ID, models.OutboundFailed)
					o.safeNotify(fmt.Sprintf("❌ Inbox thất bại\n👤 %s\n%v", lead.Author, execErr))
					failed++
				} else {
					o.safeNotify(fmt.Sprintf("✅ Đã inbox!\n👤 %s\n💬 %s", lead.Author, truncateStr(inboxContent, 120)))
					sent++
				}

				jitterSleep(inboxCtx, 10*time.Second, 25*time.Second)
			}

			o.safeNotify(fmt.Sprintf("🏁 Inbox hoàn tất: ✅ %d gửi thành công | ❌ %d thất bại", sent, failed))
		}(targets, defaultAccountID)

		return fmt.Sprintf("🚀 Đang inbox %d leads trong nền (filter: %s) — sẽ thông báo từng kết quả qua đây", len(targets), scoreFilter), nil

	case "crawl_catalog":
		catalogURL, _ := args["url"].(string)
		if catalogURL == "" {
			return "", fmt.Errorf("missing url parameter")
		}
		if o.imgScraper == nil {
			return "", fmt.Errorf("image scraper not initialized (browser pool required)")
		}

		if o.bot != nil {
			o.bot.Notify(fmt.Sprintf("🕷️ Đang crawl ảnh từ: %s\n⏳ Vui lòng chờ...", catalogURL))
		}

		saved, err := o.imgScraper.CrawlCatalog(context.Background(), catalogURL)
		if err != nil {
			return "", fmt.Errorf("crawl catalog: %w", err)
		}

		total := o.db.CountCompanyImages()
		result := fmt.Sprintf("✅ Đã lưu %d ảnh từ catalog vào database!\n🖼️ Tổng ảnh: %d\n💡 AI sẽ tự dùng các ảnh này khi comment leads", saved, total)
		if o.bot != nil {
			o.bot.Notify(result)
		}
		return result, nil

	case "crawl_careers":
		url, _ := args["url"].(string)
		if url == "" {
			return "", fmt.Errorf("missing url parameter")
		}
		if o.careersScraper == nil {
			return "", fmt.Errorf("careers scraper not initialized")
		}

		if o.bot != nil {
			o.bot.Notify(fmt.Sprintf("🕷️ Đang cập nhật tin tuyển dụng từ: %s\n⏳ Vui lòng chờ...", url))
		}

		if err := o.careersScraper.CrawlCareers(context.Background(), url); err != nil {
			return "", fmt.Errorf("crawl careers error: %w", err)
		}

		jobs, _ := o.db.GetActiveCareerJobs()
		result := fmt.Sprintf("✅ Đã cập nhật thành công %d vị trí tuyển dụng đang mở!\n💡 Cứ khi nào cào tuyển dụng, HR AI sẽ dùng thông tin list Jobs này để tư vấn comment cho ứng viên.", len(jobs))
		if o.bot != nil {
			o.bot.Notify(result)
		}

		// Auto-chain: screenshot each job card modal and save as career_job images
		// so HR agent can attach visual JD cards when commenting on candidates
		if len(jobs) > 0 {
			careersURL := url
			go func() {
				o.safeNotify(fmt.Sprintf("📸 Đang chụp ảnh JD từ %d vị trí — sẽ thông báo khi xong...", len(jobs)))
				imgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				defer cancel()
				saved, err := o.careersScraper.CrawlCareersImages(imgCtx, careersURL)
				if err != nil {
					log.Printf("[Orchestrator] CrawlCareersImages error: %v", err)
					o.safeNotify(fmt.Sprintf("⚠️ Chụp ảnh JD gặp lỗi: %v", err))
					return
				}
				o.safeNotify(fmt.Sprintf("🖼️ Đã lưu %d ảnh JD — HR Agent sẽ tự đính kèm khi comment ứng viên.", saved))
			}()
		}
		return result, nil

	case "update_price_list":
		text, _ := args["text"].(string)
		if text == "" {
			return "", fmt.Errorf("missing text parameter")
		}
		if o.pricer == nil || !o.pricer.Available() {
			return "", fmt.Errorf("price extractor not available (OPENAI_API_KEY required)")
		}
		replace, _ := args["replace"].(bool)
		if replace {
			_ = o.db.ClearPriceItems()
		}
		items, err := o.pricer.ExtractFromText(context.Background(), text)
		if err != nil {
			return "", fmt.Errorf("extract price: %w", err)
		}
		if len(items) == 0 {
			return "⚠️ Không tìm thấy bảng giá trong text. Hãy gửi với format rõ hơn, ví dụ:\nGói A: 100.000đ/tháng\nGói B: 200.000đ/tháng", nil
		}
		saved, err := o.db.InsertPriceItems(items, "text")
		if err != nil {
			return "", fmt.Errorf("save prices: %w", err)
		}
		result := fmt.Sprintf("✅ Đã học *%d mục giá*! AI sẽ tư vấn đúng giá khi comment/inbox khách.\nDùng /price trong Telegram để xem bảng giá.", saved)
		if o.bot != nil {
			o.bot.Notify(result)
		}
		return result, nil

	case "set_context":
		key, _ := args["key"].(string)
		value, _ := args["value"].(string)
		if key == "" || value == "" {
			return "", fmt.Errorf("cần có key và value")
		}
		if err := o.db.SetContext(key, value); err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ Đã lưu cấu hình: %s = %s\nAI sẽ dùng thông tin này để cào và phân loại chính xác hơn.", key, value), nil

	case "recruit_from_database":
		jobs, err := o.db.GetActiveCareerJobs()
		if err != nil || len(jobs) == 0 {
			return "📭 Chưa có vị trí tuyển dụng nào trong database. Hãy gửi link trang careers để crawl_careers() trước.", nil
		}

		// 1. Save search intent for context (profile already drives AI behavior)
		_ = o.db.SetContext("last_search_intent", "find candidates for open positions")
		log.Printf("[Orchestrator] recruit_from_database: starting candidate search")

		// 2. Generate search keywords from job titles
		var keywords []string
		for _, j := range jobs {
			keywords = append(keywords, j.Title)
		}
		// Build 2-3 search queries from the titles
		queryParts := []string{"tuyển dụng"}
		for i, kw := range keywords {
			if i < 5 { // max 5 job titles in query
				queryParts = append(queryParts, kw)
			}
		}
		searchQuery := strings.Join(queryParts, " ")

		o.safeNotify(fmt.Sprintf("🚀 Auto-recruit: tìm thấy %d vị trí trong DB\n🔍 Đang tìm groups với keywords: %s", len(jobs), searchQuery))

		// 3. Search groups
		if o.fbScraper == nil {
			return "", fmt.Errorf("facebook scraper not available")
		}

		groups, err := o.fbScraper.SearchGroups(context.Background(), searchQuery)
		if err != nil {
			return "", fmt.Errorf("search groups: %w", err)
		}

		if len(groups) == 0 {
			return fmt.Sprintf("Không tìm thấy group tuyển dụng nào cho: %q", searchQuery), nil
		}

		// 4. Add groups + auto scrape
		addedCount := 0
		for _, g := range groups {
			cleanURL := strings.Split(g.URL, "?")[0]
			if o.db.GroupExistsByURL(cleanURL) {
				continue
			}
			dbGroup := &models.Group{
				Platform:  models.PlatformFacebook,
				Name:      g.Name,
				URL:       cleanURL,
				Active:    true,
				JoinState: "none",
			}
			if _, err := o.db.AddGroup(dbGroup); err == nil {
				addedCount++
				_, _ = o.queue.Submit(models.Job{
					Type:     models.JobScrapePost,
					Platform: models.PlatformFacebook,
					Target:   cleanURL,
				})
			}
		}

		// 5. Build result summary
		var sb strings.Builder
		sb.WriteString("✅ Auto-recruit hoàn tất!\n")
		sb.WriteString(fmt.Sprintf("📋 Vị trí trong DB: %d\n", len(jobs)))
		for i, j := range jobs {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, j.Title))
		}
		sb.WriteString(fmt.Sprintf("\n🔍 Tìm thấy: %d groups | ✅ Thêm mới: %d\n", len(groups), addedCount))
		sb.WriteString("🤖 Đã tự động submit cào tất cả groups mới — AI HR sẽ dùng thông tin jobs để comment chuyên nghiệp.")

		if o.bot != nil {
			o.bot.Notify(sb.String())
		}
		return sb.String(), nil

	case "list_career_jobs":
		jobs, err := o.db.GetActiveCareerJobs()
		if err != nil {
			return "", fmt.Errorf("get career jobs: %w", err)
		}
		if len(jobs) == 0 {
			return "📭 Chưa có vị trí tuyển dụng nào trong database. Hãy gửi link trang careers để crawl_careers() trước.", nil
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("📋 Có %d vị trí tuyển dụng đang mở:\n\n", len(jobs)))
		for i, j := range jobs {
			desc := j.Description
			if len(desc) > 200 {
				desc = desc[:200] + "..."
			}
			sb.WriteString(fmt.Sprintf("%d. **%s** (%s)\n   📧 %s\n   📝 %s\n\n", i+1, j.Title, j.Location, j.Email, desc))
		}
		sb.WriteString("💡 Dùng search_groups(query=...) với keywords từ danh sách trên để tìm group tuyển dụng phù hợp.")
		return sb.String(), nil

	case "recruit_all_candidates":
		scoreFilter, _ := args["score_filter"].(string)
		if scoreFilter == "all" {
			scoreFilter = ""
		}

		// Load candidate leads — filter by role, not hardcoded niche string
		allLeads, err := o.db.GetLeads(scoreFilter, 200, 0)
		if err != nil {
			return "", fmt.Errorf("get leads: %w", err)
		}

		var candidates []models.Lead
		for _, l := range allLeads {
			if l.AuthorRole != "candidate" {
				continue
			}
			if l.SourceURL == "" || o.db.HasSentComment(l.SourceURL) {
				continue
			}
			candidates = append(candidates, l)
		}

		if len(candidates) == 0 {
			return "⚠️ Không có ứng viên nào chưa được tiếp cận (role=candidate) trong leads.", nil
		}

		// Check JDs exist
		jobs, _ := o.db.GetActiveCareerJobs()
		if len(jobs) == 0 {
			return "⚠️ Chưa có JD nào trong DB. Hãy cào trang tuyển dụng trước: 'Cào thông tin từ [URL careers]'", nil
		}
		jobsCtx := o.buildJobsContext()

		var defaultAccountID int64 = -1
		if o.accountMgr != nil {
			if acc, err := o.accountMgr.GetNextAccount(models.PlatformFacebook); err == nil {
				defaultAccountID = acc.ID
			}
		}

		genCtx, genCancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer genCancel()

		recruitBizCtx := ai.LoadProfile(o.db).ToPromptBlock()
		created := 0
		for _, lead := range candidates {
			var commentContent string
			if o.msgGen != nil {
				var genErr error
				commentContent, genErr = o.msgGen.GenerateRecruitmentComment(genCtx, lead.Content, lead.Content, lead.Author, jobsCtx, recruitBizCtx)
				if genErr != nil || commentContent == "" {
					commentContent = fmt.Sprintf("Chào %s, chúng tôi đang tuyển dụng vị trí phù hợp với bạn. Vui lòng nhắn tin để được tư vấn nhé!", lead.Author)
				}
			} else {
				commentContent = fmt.Sprintf("Chào %s, chúng tôi đang tuyển dụng vị trí phù hợp với bạn. Vui lòng nhắn tin để được tư vấn nhé!", lead.Author)
			}

			// Attach matching JD image if available
			var imgPath string
			if img, err := o.db.GetImageForCareerJob(commentContent); err == nil {
				imgPath = img.LocalPath
				_ = o.db.IncrementImageUseCount(img.ID)
			}

			msg := &models.OutboundMessage{
				Type:       "comment",
				Platform:   lead.Platform,
				AccountID:  defaultAccountID,
				TargetURL:  lead.SourceURL,
				TargetName: lead.Author,
				Content:    commentContent,
				Context:    lead.Content,
				ImagePath:  imgPath,
				Status:     models.OutboundApproved,
				AIModel:    "gpt-4o-mini",
			}
			if _, err := o.db.InsertOutboundMessage(msg); err == nil {
				created++
			}
		}

		if created > 0 {
			_, _ = o.queue.Submit(models.Job{
				Type:     models.JobAutoComment,
				Platform: models.PlatformFacebook,
				Target:   "auto",
			})
		}

		return fmt.Sprintf("🚀 Đã xếp hàng %d outreach comments cho ứng viên — sẽ comment tuần tự và thông báo từng kết quả | JDs: %d vị trí", created, len(jobs)), nil

	case "create_job_post":
		title, _ := args["title"].(string)
		if title == "" {
			return "", fmt.Errorf("missing job title")
		}
		description, _ := args["description"].(string)
		requirements, _ := args["requirements"].(string)
		benefits, _ := args["benefits"].(string)
		salary, _ := args["salary"].(string)
		email, _ := args["email"].(string)
		if email == "" {
			// Try to read from stored context (set via describe_business or set_context)
			email, _ = o.db.GetContext("career_email")
		}

		var postContent string
		if o.msgGen != nil {
			genCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			var err error
			postContent, err = o.msgGen.GenerateJobPost(genCtx, title, description, requirements, benefits, salary, email)
			if err != nil {
				log.Printf("[Orchestrator] GenerateJobPost failed: %v", err)
			}
		}
		if postContent == "" {
			postContent = fmt.Sprintf("🏢 THG TUYỂN DỤNG: %s\n\n%s\n\nYêu cầu: %s\nLương: %s\n\nGửi CV: %s",
				title, description, requirements, salary, email)
		}

		// Save as a draft outbound post (type="post" for future posting, or store as comment draft)
		var defaultAccountID int64 = -1
		if o.accountMgr != nil {
			if acc, err := o.accountMgr.GetNextAccount(models.PlatformFacebook); err == nil {
				defaultAccountID = acc.ID
			}
		}
		msg := &models.OutboundMessage{
			Type:       "post",
			Platform:   models.PlatformFacebook,
			AccountID:  defaultAccountID,
			TargetURL:  "",
			TargetName: title,
			Content:    postContent,
			Context:    fmt.Sprintf("Job post: %s | Salary: %s | Email: %s", title, salary, email),
			Status:     models.OutboundDraft,
			AIModel:    "gpt-4o-mini",
		}
		id, err := o.db.InsertOutboundMessage(msg)
		if err != nil {
			return "", fmt.Errorf("save job post draft: %w", err)
		}

		preview := postContent
		if len(preview) > 300 {
			preview = preview[:300] + "..."
		}
		o.safeNotify(fmt.Sprintf("📋 Đã soạn bài tuyển dụng #%d — *%s*\n\n%s\n\n💡 Vào Dashboard → Outbox để xem và copy bài đăng.", id, title, preview))
		return fmt.Sprintf("✅ Đã soạn bài tuyển dụng #%d cho vị trí *%s* — xem tại Dashboard Outbox", id, title), nil

	case "run_full_recruitment_pipeline":
		jobs, _ := o.db.GetCareerJobsByPriority()
		if len(jobs) == 0 {
			return "⚠️ Chưa có JD nào trong DB. Hãy cào trang careers trước: 'Cào thông tin từ [URL]'", nil
		}
		o.safeNotify(fmt.Sprintf("🤖 Khởi động Full Recruitment Pipeline — %d jobs, tự động crawl + match + outreach + post JDs", len(jobs)))
		go func() {
			pipelineCtx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
			defer cancel()
			result, err := o.RunFullRecruitmentPipeline(pipelineCtx)
			if err != nil {
				log.Printf("[Orchestrator] Pipeline error: %v", err)
				o.safeNotify(fmt.Sprintf("❌ Pipeline lỗi: %v", err))
				return
			}
			log.Printf("[Orchestrator] Pipeline done: %+v", result)
		}()
		return fmt.Sprintf("🚀 Đang chạy Full Recruitment Pipeline (%d jobs) trong nền — sẽ thông báo từng bước và tổng kết khi xong", len(jobs)), nil

	case "post_jds_to_groups":
		jobs, _ := o.db.GetCareerJobsByPriority()
		if len(jobs) == 0 {
			return "⚠️ Chưa có JD nào trong DB. Hãy cào trang careers trước: 'Cào thông tin từ [URL]'", nil
		}
		// Parse optional positions filter
		positionsFilter, _ := args["positions"].(string)
		positionsFilter = strings.TrimSpace(positionsFilter)

		var filteredJobs []models.CareerJob
		if positionsFilter != "" {
			// User specified specific positions — filter by name
			requestedPositions := strings.Split(positionsFilter, ",")
			for _, job := range jobs {
				jobLower := strings.ToLower(strings.TrimSpace(job.Title))
				for _, pos := range requestedPositions {
					posLower := strings.ToLower(strings.TrimSpace(pos))
					if posLower == "" {
						continue
					}
					if strings.Contains(jobLower, posLower) || strings.Contains(posLower, jobLower) {
						filteredJobs = append(filteredJobs, job)
						break
					}
				}
			}
			if len(filteredJobs) == 0 {
				return fmt.Sprintf("⚠️ Không tìm thấy vị trí nào khớp với: %s\nCác vị trí hiện có: %s",
					positionsFilter, func() string {
						var names []string
						for _, j := range jobs {
							names = append(names, j.Title)
						}
						return strings.Join(names, ", ")
					}()), nil
			}
			log.Printf("[Orchestrator] PostJDs filtered to %d/%d positions: %s", len(filteredJobs), len(jobs), positionsFilter)
		} else {
			filteredJobs = jobs
		}

		go func() {
			postCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			posted, err := o.PostJDsToExistingGroups(postCtx, filteredJobs)
			if err != nil {
				log.Printf("[Orchestrator] PostJDs error: %v", err)
				o.safeNotify(fmt.Sprintf("❌ Lỗi tạo bài: %v", err))
				return
			}
			log.Printf("[Orchestrator] PostJDs done: %d posts created", posted)
		}()
		if positionsFilter != "" {
			var names []string
			for _, j := range filteredJobs {
				names = append(names, j.Title)
			}
			return fmt.Sprintf("📝 Đang tạo bài viết tuyển dụng cho %d vị trí: %s", len(filteredJobs), strings.Join(names, ", ")), nil
		}
		return fmt.Sprintf("📝 Đang tạo bài viết tuyển dụng cho %d vị trí — sử dụng groups có sẵn", len(filteredJobs)), nil

	case "scan_own_jd_posts":
		sentPosts, err := o.db.GetSentGroupPosts(7)
		if err != nil || len(sentPosts) == 0 {
			return "⚠️ Chưa có bài JD nào được đăng. Hãy tạo bài viết tuyển dụng trước.", nil
		}
		go func() {
			scanCtx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()
			scanned, err := o.ScanOwnJDPosts(scanCtx)
			if err != nil {
				log.Printf("[Orchestrator] ScanJDPosts error: %v", err)
				o.safeNotify(fmt.Sprintf("❌ Lỗi quét bài JD: %v", err))
				return
			}
			log.Printf("[Orchestrator] ScanJDPosts done: scanned %d posts", scanned)
		}()
		return fmt.Sprintf("🔍 Đang quét %d bài JD đã đăng để tìm ứng viên comments — leads sẽ hiện trong tab Tuyển dụng", len(sentPosts)), nil

	case "crawl_careers_images":
		url, _ := args["url"].(string)
		if url == "" {
			return "", fmt.Errorf("missing url parameter")
		}
		if o.careersScraper == nil {
			return "", fmt.Errorf("careers scraper not initialized")
		}
		o.safeNotify(fmt.Sprintf("📸 Đang chụp ảnh JD từ: %s\n⏳ Quá trình mở từng modal và screenshot có thể mất vài phút...", url))
		go func() {
			imgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			saved, err := o.careersScraper.CrawlCareersImages(imgCtx, url)
			if err != nil {
				o.safeNotify(fmt.Sprintf("❌ Chụp ảnh JD thất bại: %v", err))
				return
			}
			total := o.db.CountCompanyImages()
			o.safeNotify(fmt.Sprintf("✅ Đã lưu %d ảnh JD từ trang tuyển dụng!\n🖼️ Tổng ảnh trong DB: %d\n💡 HR Agent sẽ tự đính kèm ảnh JD phù hợp khi comment ứng viên.", saved, total))
		}()
		return "📸 Đang chụp ảnh JD trong nền — sẽ thông báo khi hoàn tất", nil

	case "check_inbox_replies":
		if o.autoCommenter == nil {
			return "", fmt.Errorf("auto commenter not initialized")
		}
		o.safeNotify("🔍 Đang kiểm tra replies từ tất cả conversations...")
		go func() {
			checkCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()
			if err := o.checkInboxReplies(checkCtx); err != nil {
				log.Printf("[Orchestrator] checkInboxReplies error: %v", err)
				o.safeNotify(fmt.Sprintf("❌ Check replies lỗi: %v", err))
			}
		}()
		return "🔍 Đang kiểm tra replies trong nền — sẽ thông báo khi có kết quả", nil

	case "score_groups":
		allGroups, _ := o.db.GetActiveGroups(models.PlatformFacebook)
		unscored := 0
		for _, g := range allGroups {
			if _, err := o.db.GetGroupQuality(g.ID); err == nil {
				continue
			}
			unscored++
		}
		if unscored == 0 {
			return "✅ Tất cả groups đã được score rồi. Xem dashboard để biết chi tiết.", nil
		}
		o.safeNotify(fmt.Sprintf("📊 Bắt đầu NLP scoring %d groups chưa đánh giá...", unscored))
		go func() {
			scoreCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()
			scored, rejected, monitored := 0, 0, 0
			for _, g := range allGroups {
				if scoreCtx.Err() != nil {
					break
				}
				if _, err := o.db.GetGroupQuality(g.ID); err == nil {
					continue
				}
				if o.msgGen == nil {
					break
				}
				q, err := o.msgGen.ScoreGroupQuality(scoreCtx, g.Name, "", "", "general")
				if err != nil {
					log.Printf("[score_groups] Failed to score %q: %v", g.Name, err)
					continue
				}
				q.GroupID = g.ID
				_ = o.db.UpsertGroupQuality(q)
				switch q.Decision {
				case "use":
					scored++
				case "reject":
					rejected++
				default:
					monitored++
				}
				log.Printf("[score_groups] %q → %.2f (%s)", g.Name, q.FinalScore, q.Decision)
			}
			o.safeNotify(fmt.Sprintf("✅ Hoàn thành scoring!\n✅ Dùng được: %d\n👀 Theo dõi: %d\n❌ Loại: %d", scored, monitored, rejected))
		}()
		return fmt.Sprintf("📊 Đang NLP score %d groups trong nền — sẽ thông báo kết quả", unscored), nil

	case "discover_groups_for_jobs":
		jobs, _ := o.db.GetCareerJobsByPriority()
		if len(jobs) == 0 {
			return "⚠️ Chưa có jobs trong DB — hãy cào careers trước", nil
		}
		if o.fbScraper == nil {
			return "", fmt.Errorf("Facebook scraper not initialized")
		}
		o.safeNotify(fmt.Sprintf("🔍 Khám phá groups cho %d job domains...", len(jobs)))
		go func() {
			discCtx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()
			seenDomains := make(map[string]bool)
			totalAdded := 0
			for _, job := range jobs {
				domain := ai.JobDomainCategory(job)
				if seenDomains[domain] {
					continue
				}
				seenDomains[domain] = true
				queries := o.msgGen.ExtractJobKeywords(job)
				for _, q := range queries[:min(len(queries), 3)] {
					if discCtx.Err() != nil {
						return
					}
					found, err := o.fbScraper.SearchGroups(discCtx, q)
					if err != nil {
						continue
					}
					for _, ng := range found {
						clean := strings.Split(ng.URL, "?")[0]
						if o.db.GroupExistsByURL(clean) {
							continue
						}
						g := &models.Group{Platform: models.PlatformFacebook, Name: ng.Name, URL: clean, Active: true, JoinState: "none"}
						if id, err := o.db.AddGroup(g); err == nil {
							g.ID = id
							totalAdded++
							// Auto-score new group
							if quality, err := o.msgGen.ScoreGroupQuality(discCtx, ng.Name, "", "", domain); err == nil {
								quality.GroupID = g.ID
								_ = o.db.UpsertGroupQuality(quality)
							}
						}
					}
				}
			}
			o.safeNotify(fmt.Sprintf("✅ Đã khám phá và thêm %d groups mới vào hệ thống", totalAdded))
		}()
		return "🔍 Đang khám phá groups theo domain trong nền — sẽ thông báo khi hoàn tất", nil

	case "seed_quality_groups":
		// Curated high-quality Vietnamese recruitment group names by domain
		seeds := map[string][]string{
			"tech":    {"Vietnam Developers Community", "Frontend Vietnam", "Fullstack Vietnam", "Vietnam AI Community", "IT Jobs Vietnam", "Cộng đồng Lập trình viên Việt Nam"},
			"sales":   {"Sales & Marketing Vietnam", "Cộng đồng Sales Việt Nam", "Vietnam Business Network", "Logistics & Supply Chain Vietnam", "Cộng đồng Kinh doanh HCM"},
			"ops":     {"Vietnam Logistics Community", "Cộng đồng Vận hành & Kho bãi", "Ecommerce Operations Vietnam", "Supply Chain Việt Nam"},
			"finance": {"Kế toán Kiểm toán Việt Nam", "Vietnam Finance Network", "Cộng đồng Tài chính Kế toán"},
		}
		if o.fbScraper == nil {
			return "", fmt.Errorf("Facebook scraper not initialized")
		}
		total := 0
		for _, names := range seeds {
			total += len(names)
		}
		o.safeNotify(fmt.Sprintf("🌱 Seeding %d groups chất lượng cao theo domain...", total))
		go func() {
			seedCtx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()
			added := 0
			for domain, names := range seeds {
				for _, name := range names {
					if seedCtx.Err() != nil {
						return
					}
					found, err := o.fbScraper.SearchGroups(seedCtx, name)
					if err != nil || len(found) == 0 {
						continue
					}
					ng := found[0]
					clean := strings.Split(ng.URL, "?")[0]
					if o.db.GroupExistsByURL(clean) {
						continue
					}
					g := &models.Group{Platform: models.PlatformFacebook, Name: ng.Name, URL: clean, Active: true, JoinState: "none"}
					if id, err := o.db.AddGroup(g); err == nil {
						g.ID = id
						added++
						if quality, err := o.msgGen.ScoreGroupQuality(seedCtx, ng.Name, "", "", domain); err == nil {
							quality.GroupID = g.ID
							_ = o.db.UpsertGroupQuality(quality)
							log.Printf("[seed_groups] Added %q → score %.2f (%s)", ng.Name, quality.FinalScore, quality.Decision)
						}
					}
				}
			}
			o.safeNotify(fmt.Sprintf("✅ Seed hoàn tất! Đã thêm %d groups chất lượng cao vào hệ thống", added))
		}()
		return fmt.Sprintf("🌱 Đang seed %d groups chất lượng trong nền...", total), nil

	case "describe_business":
		// AI extracts a structured BusinessProfile from any free-form description
		if o.msgGen == nil {
			return "", fmt.Errorf("AI not initialized")
		}
		desc, _ := args["description"].(string)
		if strings.TrimSpace(desc) == "" {
			return "⚠️ Vui lòng mô tả doanh nghiệp của bạn (ngành, dịch vụ, khách hàng mục tiêu...)", nil
		}
		descCtx, descCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer descCancel()
		profile, err := o.msgGen.ExtractProfile(descCtx, desc)
		if err != nil {
			return "", fmt.Errorf("extract profile: %w", err)
		}
		if err := profile.Save(o.db); err != nil {
			return "", fmt.Errorf("save profile: %w", err)
		}
		var summary strings.Builder
		summary.WriteString("✅ Đã cập nhật hồ sơ doanh nghiệp:\n")
		if profile.Name != "" {
			fmt.Fprintf(&summary, "🏢 Tên: %s\n", profile.Name)
		}
		if profile.Industry != "" {
			fmt.Fprintf(&summary, "🏭 Ngành: %s\n", profile.Industry)
		}
		if profile.Description != "" {
			fmt.Fprintf(&summary, "📋 Mô tả: %s\n", profile.Description)
		}
		if profile.Services != "" {
			fmt.Fprintf(&summary, "🛒 Dịch vụ: %s\n", profile.Services)
		}
		if profile.Targets != "" {
			fmt.Fprintf(&summary, "🎯 Khách hàng mục tiêu: %s\n", profile.Targets)
		}
		if profile.Location != "" {
			fmt.Fprintf(&summary, "📍 Địa điểm: %s\n", profile.Location)
		}
		if profile.USP != "" {
			fmt.Fprintf(&summary, "⭐ Điểm khác biệt: %s\n", profile.USP)
		}
		summary.WriteString("\nAI sẽ dùng thông tin này cho tất cả hoạt động (phân loại, comment, inbox, tìm groups).")
		return summary.String(), nil

	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

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

		// Scrape the live Messenger conversation
		msgs, err := o.autoCommenter.ScrapeConversation(ctx, &thread, accountID)
		if err != nil {
			log.Printf("[Orchestrator] ScrapeConversation failed thread #%d (%s): %v", thread.ID, thread.ProfileName, err)
			jitterSleep(ctx, 5*time.Second, 10*time.Second)
			continue
		}

		// Build set of already-stored message contents
		existing, _ := o.db.GetThreadMessages(thread.ID)
		existingSet := make(map[string]bool, len(existing))
		for _, m := range existing {
			existingSet[m.Content] = true
		}

		// Detect new inbound messages
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

		// Persist new inbound messages
		for _, m := range newInbound {
			_ = o.db.AddThreadMessage(thread.ID, "inbound", m.Content, false)
			newReplies++
		}
		_ = o.db.UpdateThreadStatus(thread.ID, "replied")

		log.Printf("[Orchestrator] Thread #%d (%s): %d new replies found", thread.ID, thread.ProfileName, len(newInbound))

		// Skip follow-up if AI not available
		if o.msgGen == nil {
			continue
		}

		// Build full conversation history string for AI context
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

		// Send the follow-up via Messenger
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

		// Save as a lead so it appears in the dashboard
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

		// Look up a matching JD card image to attach
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
