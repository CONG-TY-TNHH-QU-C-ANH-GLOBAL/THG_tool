package orchestrator

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/scraper"
)

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
		if niche, ok := args["niche"].(string); ok && niche != "" {
			_ = o.db.SetContext("last_search_intent", niche)
			log.Printf("[Orchestrator] search_groups: intent = %s", niche)
		}
		if o.fbScraper == nil {
			return "", fmt.Errorf("facebook scraper not available")
		}
		searchAccountID := int64(0)
		if aid, ok := args["account_id"].(float64); ok && aid > 0 {
			searchAccountID = int64(aid)
		}
		searchScraper := o.fbScraper
		if searchAccountID > 0 && o.accountMgr != nil {
			if acc, accErr := o.accountMgr.GetAccountByID(searchAccountID); accErr == nil {
				if accPool, poolErr := o.accountMgr.GetPoolForAccount(acc); poolErr == nil {
					searchScraper = scraper.NewFacebookScraper(accPool, o.db)
					defer accPool.Shutdown()
					log.Printf("[Orchestrator] search_groups: using account %s (ID=%d)", acc.Name, acc.ID)
				} else {
					log.Printf("[Orchestrator] search_groups: GetPoolForAccount failed for ID=%d: %v", searchAccountID, poolErr)
				}
			}
		}
		groups, err := searchScraper.SearchGroups(context.Background(), query)
		if err != nil {
			return "", fmt.Errorf("lỗi tìm kiếm FB: %v", err)
		}
		if len(groups) == 0 {
			return fmt.Sprintf("Không tìm thấy group Public nào (1K+ members) cho từ khóa: %q", query), nil
		}
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
				job := models.Job{
					Type:     models.JobScrapePost,
					Platform: models.PlatformFacebook,
					Target:   cleanURL,
				}
				_, _ = o.queue.Submit(job)
			}
		}
		accountSuffix := ""
		if searchAccountID > 0 && o.accountMgr != nil {
			if acc, aErr := o.accountMgr.GetAccountByID(searchAccountID); aErr == nil {
				accountSuffix = fmt.Sprintf(" (dùng account: %s)", acc.Name)
			}
		}
		return fmt.Sprintf("🔍 Đã tìm thấy %d public groups cho %q. Đã thêm mới %d groups vào danh sách theo dõi và bắt đầu cào auto%s.", len(groups), query, addedCount, accountSuffix), nil

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
		if aid, ok := args["account_id"].(float64); ok && aid > 0 {
			defaultAccountID = int64(aid)
		} else if o.accountMgr != nil {
			if acc, err := o.accountMgr.GetNextAccount(models.PlatformFacebook); err == nil {
				defaultAccountID = acc.ID
			}
		}
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
			isLeadRecruitment := lead.AuthorRole == "candidate" &&
				(strings.Contains(strings.ToLower(lead.Niche), "recruit") ||
					strings.EqualFold(lead.Niche, "tuyen_dung"))
			var commentContent string
			var genErr error
			if template != "" && o.msgGen != nil {
				commentContent, genErr = o.msgGen.GenerateCommentFromTemplate(genCtx, template, lead.Content, lead.Author)
			} else if o.msgGen != nil {
				bzCtx := o.fetchBusinessContext(lead.Niche)
				if isLeadRecruitment {
					jobsCtx := o.buildJobsContext()
					commentContent, genErr = o.msgGen.GenerateRecruitmentComment(genCtx, lead.Content, lead.Content, lead.Author, jobsCtx, bzCtx)
				} else {
					commentContent, genErr = o.msgGen.GenerateCommentWithService(genCtx, lead.Content, lead.Author, bzCtx, lead.ServiceMatch, lead.Niche)
				}
			}
			if genErr != nil || commentContent == "" {
				commentContent = template
			}
			if commentContent == "" {
				if isLeadRecruitment {
					commentContent = fmt.Sprintf("Chào %s, chúng tôi đang có vị trí phù hợp với bạn. Vui lòng nhắn tin để được tư vấn nhé!", lead.Author)
				} else {
					commentContent = fmt.Sprintf("Chào %s, bạn có thể liên hệ để được tư vấn thêm về dịch vụ phù hợp nhé!", lead.Author)
				}
			}
			var imagePath string
			if isLeadRecruitment {
				if img, err := o.db.GetImageForCareerJob(commentContent); err == nil {
					imagePath = img.LocalPath
					_ = o.db.IncrementImageUseCount(img.ID)
				}
			} else {
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
			scoreFilter = "hot"
		}
		leads, err := o.db.GetLeads(scoreFilter, 200, 0)
		if err != nil {
			return "", fmt.Errorf("get leads: %w", err)
		}
		if len(leads) == 0 {
			return fmt.Sprintf("⚠️ Không có leads nào (filter: %s)", scoreFilter), nil
		}
		type inboxTarget struct{ lead models.Lead }
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
			return fmt.Sprintf("⚠️ Không có leads nào để inbox (ẩn danh: %d, đã inbox rồi: %d)", anonymousCount, alreadySentCount), nil
		}
		var defaultAccountID int64 = -1
		if aid, ok := args["account_id"].(float64); ok && aid > 0 {
			defaultAccountID = int64(aid)
		} else if o.accountMgr != nil {
			if acc, err := o.accountMgr.GetNextAccount(models.PlatformFacebook); err == nil {
				defaultAccountID = acc.ID
			}
		}
		o.safeNotify(fmt.Sprintf(
			"📬 Bắt đầu inbox %d leads (bỏ qua: %d ẩn danh, %d đã inbox)\nFilter: %s | Model: gpt-4o",
			len(targets), anonymousCount, alreadySentCount, scoreFilter,
		))
		inboxCtx, inboxCancel := context.WithTimeout(context.Background(), 45*time.Minute)
		go func(tgts []inboxTarget, accID int64) {
			defer inboxCancel()
			sent, failed := 0, 0
			for _, t := range tgts {
				lead := t.lead
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
		_ = o.db.SetContext("last_search_intent", "find candidates for open positions")
		log.Printf("[Orchestrator] recruit_from_database: starting candidate search")
		var keywords []string
		for _, j := range jobs {
			keywords = append(keywords, j.Title)
		}
		queryParts := []string{"tuyển dụng"}
		for i, kw := range keywords {
			if i < 5 {
				queryParts = append(queryParts, kw)
			}
		}
		searchQuery := strings.Join(queryParts, " ")
		o.safeNotify(fmt.Sprintf("🚀 Auto-recruit: tìm thấy %d vị trí trong DB\n🔍 Đang tìm groups với keywords: %s", len(jobs), searchQuery))
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
		jobs, _ := o.db.GetActiveCareerJobs()
		if len(jobs) == 0 {
			return "⚠️ Chưa có JD nào trong DB. Hãy cào trang tuyển dụng trước: 'Cào thông tin từ [URL careers]'", nil
		}
		jobsCtx := o.buildJobsContext()
		var defaultAccountID int64 = -1
		if aid, ok := args["account_id"].(float64); ok && aid > 0 {
			defaultAccountID = int64(aid)
		} else if o.accountMgr != nil {
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
		positionsFilter, _ := args["positions"].(string)
		positionsFilter = strings.TrimSpace(positionsFilter)
		var filteredJobs []models.CareerJob
		if positionsFilter != "" {
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
				var names []string
				for _, j := range jobs {
					names = append(names, j.Title)
				}
				return fmt.Sprintf("⚠️ Không tìm thấy vị trí nào khớp với: %s\nCác vị trí hiện có: %s",
					positionsFilter, strings.Join(names, ", ")), nil
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
