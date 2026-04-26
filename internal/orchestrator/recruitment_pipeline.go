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

// PipelineResult holds structured output from RunFullRecruitmentPipeline.
type PipelineResult struct {
	JobsProcessed   int
	CandidatesFound int
	UniqueMatches   int
	CommentsQueued  int
	InboxesQueued   int
	PostsCreated    int
	Skipped         int
	Matches         []MatchSummary
	Duration        time.Duration
}

// MatchSummary is a single scored candidate-job pairing for reporting.
type MatchSummary struct {
	CandidateName string
	JobTitle      string
	Score         float64
	Reason        string
	Outreach      string // "comment+inbox" | "comment"
}

// commentThreshold is the minimum AI score to send a comment reply.
func commentThreshold(priority string) float64 {
	switch strings.ToLower(priority) {
	case "high":
		return 0.35
	case "low":
		return 0.70
	default: // medium
		return 0.55
	}
}

// inboxThreshold is the minimum AI score to also send an inbox DM.
func inboxThreshold(priority string) float64 {
	switch strings.ToLower(priority) {
	case "high":
		return 0.55
	case "low":
		return 0.82
	default:
		return 0.67
	}
}

// jobKW pairs a career job with its extracted search keywords.
type jobKW struct {
	job      models.CareerJob
	keywords []string
}

// RunFullRecruitmentPipeline executes the complete end-to-end recruitment pipeline:
//
//	Load jobs → extract keywords → crawl groups → scrape candidate comments →
//	score + domain-match → deduplicate → comment + inbox → post JDs → report
func (o *Orchestrator) RunFullRecruitmentPipeline(ctx context.Context) (*PipelineResult, error) {
	startTime := time.Now()
	result := &PipelineResult{}

	// ── Phase 1: Load jobs sorted by priority ──────────────────────────────────
	jobs, err := o.db.GetCareerJobsByPriority()
	if err != nil || len(jobs) == 0 {
		return nil, fmt.Errorf("no active career jobs in database — run 'crawl_careers' first")
	}
	result.JobsProcessed = len(jobs)
	o.safeNotify(fmt.Sprintf("🚀 Pipeline tuyển dụng bắt đầu\n📋 %d vị trí:\n%s", len(jobs), formatJobList(jobs)))

	// ── Phase 2: Extract keywords for each job ─────────────────────────────────
	jobKWs := make([]jobKW, 0, len(jobs))
	for _, j := range jobs {
		kws := o.msgGen.ExtractJobKeywords(j)
		jobKWs = append(jobKWs, jobKW{job: j, keywords: kws})
		log.Printf("[Pipeline] %q → keywords: %v", j.Title, kws)
	}

	// ── Phase 3: Auto-discover groups per domain, score, filter ──────────────
	groups, _ := o.db.GetActiveGroups(models.PlatformFacebook)

	// 3a. Collect unique domains from jobs + build search queries per domain
	domainQueries := make(map[string]string) // domain → search query
	for _, jkw := range jobKWs {
		domain := ai.JobDomainCategory(jkw.job)
		if _, exists := domainQueries[domain]; exists {
			continue
		}
		// Build domain-specific search queries
		switch domain {
		case "tech":
			domainQueries[domain] = "tuyển dụng IT developer engineer AI HCM"
		case "sales":
			domainQueries[domain] = "tuyển dụng sales kinh doanh logistics HCM"
		case "ops":
			domainQueries[domain] = "tuyển dụng vận hành operations logistics ecommerce HCM"
		case "finance":
			domainQueries[domain] = "tuyển dụng kế toán accountant finance HCM"
		default:
			domainQueries[domain] = "tuyển dụng " + domain + " HCM"
		}
	}

	// 3b. Search Facebook for groups in EACH domain
	if o.fbScraper != nil {
		for domain, query := range domainQueries {
			if ctx.Err() != nil {
				break
			}
			o.safeNotify(fmt.Sprintf("🔍 Tìm groups cho domain [%s]: %q", domain, query))
			searchCtx, searchCancel := context.WithTimeout(ctx, 3*time.Minute)
			found, err := o.fbScraper.SearchGroups(searchCtx, query)
			searchCancel()
			if err != nil {
				log.Printf("[Pipeline] SearchGroups failed for domain %s: %v", domain, err)
				continue
			}
			added := 0
			for _, ng := range found {
				clean := strings.Split(ng.URL, "?")[0]
				if o.db.GroupExistsByURL(clean) {
					continue
				}
				g := &models.Group{Platform: models.PlatformFacebook, Name: ng.Name, URL: clean, Active: true, JoinState: "none"}
				if id, err := o.db.AddGroup(g); err == nil {
					g.ID = id
					groups = append(groups, *g)
					added++
				}
			}
			log.Printf("[Pipeline] Domain [%s]: found %d groups, added %d new", domain, len(found), added)
		}
	}

	if len(groups) == 0 {
		return nil, fmt.Errorf("không có groups nào để cào — hãy kiểm tra kết nối Facebook")
	}

	// 3c. Score unscored groups (NLP quality scoring)
	unscoredCount := 0
	for _, g := range groups {
		if ctx.Err() != nil {
			break
		}
		if _, err := o.db.GetGroupQuality(g.ID); err == nil {
			continue // already scored
		}
		unscoredCount++
		if o.msgGen != nil {
			scoreCtx, scoreCancel := context.WithTimeout(ctx, 20*time.Second)
			quality, err := o.msgGen.ScoreGroupQuality(scoreCtx, g.Name, "", "", "general")
			scoreCancel()
			if err == nil && quality != nil {
				quality.GroupID = g.ID
				_ = o.db.UpsertGroupQuality(quality)
				log.Printf("[Pipeline] Scored group %q: %.2f (%s) → %s",
					g.Name, quality.FinalScore, quality.Category, quality.Decision)
			}
		}
	}
	if unscoredCount > 0 {
		o.safeNotify(fmt.Sprintf("📊 Scored %d new groups", unscoredCount))
	}

	// 3d. Build domain → group mapping (only use SCORED groups with matching domain)
	domainGroups := make(map[string][]models.Group)
	for _, g := range groups {
		q, err := o.db.GetGroupQuality(g.ID)
		if err != nil {
			log.Printf("[Pipeline] Skipping unscored group %q (ID %d)", g.Name, g.ID)
			continue
		}
		if q.Blacklist {
			continue
		}
		if q.FinalScore >= 0.4 {
			domainGroups[q.Category] = append(domainGroups[q.Category], g)
		}
	}

	// 3e. Map jobs → groups by domain
	type jobWithGroups struct {
		job    models.CareerJob
		groups []models.Group
	}
	var jobGroupPairs []jobWithGroups
	for _, jkw := range jobKWs {
		domain := ai.JobDomainCategory(jkw.job)
		matched := domainGroups[domain]
		if len(matched) == 0 {
			log.Printf("[Pipeline] No groups for domain %q (job: %s) — this job will be skipped", domain, jkw.job.Title)
		}
		jobGroupPairs = append(jobGroupPairs, jobWithGroups{job: jkw.job, groups: dedupeGroups(matched)})
	}

	totalGroups := 0
	seenGroups := make(map[int64]bool)
	for _, pair := range jobGroupPairs {
		for _, g := range pair.groups {
			if !seenGroups[g.ID] {
				seenGroups[g.ID] = true
				totalGroups++
			}
		}
	}
	o.safeNotify(fmt.Sprintf("📂 %d jobs → %d unique groups (by domain)", len(jobGroupPairs), totalGroups))

	// ── Phase 4–7: Crawl → match → deduplicate → queue outreach ───────────────
	contacted := make(map[string]bool) // authorURL → already reached out this run

	var defaultAccountID int64 = -1
	if o.accountMgr != nil {
		if acc, err := o.accountMgr.GetNextAccount(models.PlatformFacebook); err == nil {
			defaultAccountID = acc.ID
		}
	}

	for _, pair := range jobGroupPairs {
		for gi, group := range pair.groups {
			if ctx.Err() != nil {
				break
			}
			log.Printf("[Pipeline] [%s] [%d/%d] Scraping group: %s", pair.job.Title, gi+1, len(pair.groups), group.Name)

			scrapeCtx, scrapeCancel := context.WithTimeout(ctx, 3*time.Minute)
			posts, err := o.fbScraper.ScrapeGroup(scrapeCtx, group)
			scrapeCancel()
			if err != nil {
				log.Printf("[Pipeline] ScrapeGroup error %s: %v", group.Name, err)
				continue
			}

			for _, post := range posts {
				if post.URL == "" || ctx.Err() != nil {
					continue
				}

				commentCtx, commentCancel := context.WithTimeout(ctx, 2*time.Minute)
				candidates, err := o.fbScraper.ScrapeJobSeekerComments(commentCtx, post.URL)
				commentCancel()
				if err != nil || len(candidates) == 0 {
					continue
				}
				result.CandidatesFound += len(candidates)

				for _, c := range candidates {
					if ctx.Err() != nil {
						break
					}
					if c.AuthorURL == "" || contacted[c.AuthorURL] {
						result.Skipped++
						continue
					}

					// Score against all jobs — pick best match
					bestJob, bestScore, bestReason := o.scoreBestJobMatch(ctx, c.Content, post.Content, jobs)
					if bestJob == nil || bestScore < commentThreshold(bestJob.Priority) {
						result.Skipped++
						// Learning loop: spam yield
						_ = o.db.UpdateGroupYield(group.ID, 0, 1)
						continue
					}

					contacted[c.AuthorURL] = true
					outreachType := "comment"

					// Learning loop: candidate yield
					_ = o.db.UpdateGroupYield(group.ID, 1, 0)

					// Generate @mention comment reply
					jobsCtx := o.buildJobsContext()
					pipelineBizCtx := ai.LoadProfile(o.db).ToPromptBlock()
					var commentContent string
					if o.msgGen != nil {
						genCtx, genCancel := context.WithTimeout(ctx, 20*time.Second)
						commentContent, _ = o.msgGen.GenerateRecruitmentComment(genCtx, post.Content, c.Content, c.Author, jobsCtx, pipelineBizCtx)
						genCancel()
					}
					if commentContent == "" {
						commentContent = fmt.Sprintf("@%s Chúng tôi đang tuyển %s. Vui lòng nhắn tin để ứng tuyển!", c.Author, bestJob.Title)
					}

					// JD image attachment
					var imgPath string
					if img, err := o.db.GetImageForCareerJob(bestJob.Title); err == nil {
						imgPath = img.LocalPath
						_ = o.db.IncrementImageUseCount(img.ID)
					}

					// Cross-run dedup: check if we already contacted this candidate
					alreadyContacted := o.db.HasContactedCandidate(c.AuthorURL)

					if defaultAccountID >= 0 && !alreadyContacted {
						// Encode author URL in context so HasContactedCandidate can find it cross-run
						ctx2 := fmt.Sprintf("author_url=%s|%s", c.AuthorURL, c.Content)
						msg := &models.OutboundMessage{
							Type: "comment_reply", Platform: models.PlatformFacebook,
							AccountID: defaultAccountID, TargetURL: post.URL,
							TargetName: c.Author, Content: commentContent,
							Context: ctx2, ImagePath: imgPath,
							Status: models.OutboundApproved, AIModel: "gpt-4o-mini",
						}
						if _, err := o.db.InsertOutboundMessage(msg); err == nil {
							result.CommentsQueued++
						}
					}

					// Inbox DM for high-score matches
					if bestScore >= inboxThreshold(bestJob.Priority) && !alreadyContacted && !o.db.HasSentInbox(c.AuthorURL) {
						var inboxContent string
						if o.msgGen != nil {
							genCtx, genCancel := context.WithTimeout(ctx, 20*time.Second)
							inboxContent, _ = o.msgGen.GeneratePersonalizedInbox(genCtx, c.Content, c.Author, *bestJob)
							genCancel()
						}
						if inboxContent == "" {
							inboxContent = fmt.Sprintf("Chào %s, mình thấy bạn đang tìm việc. THG đang tuyển %s — vui lòng gửi CV về %s để trao đổi thêm.", c.Author, bestJob.Title, bestJob.Email)
						}
						ctx2 := fmt.Sprintf("author_url=%s|%s", c.AuthorURL, c.Content)
						inbox := &models.OutboundMessage{
							Type: "inbox", Platform: models.PlatformFacebook,
							AccountID: defaultAccountID, TargetURL: c.AuthorURL,
							TargetName: c.Author, Content: inboxContent,
							Context: ctx2, ImagePath: imgPath,
							Status: models.OutboundApproved, AIModel: "gpt-4o",
						}
						if _, err := o.db.InsertOutboundMessage(inbox); err == nil {
							result.InboxesQueued++
							outreachType = "comment+inbox"
						}
					}

					result.UniqueMatches++
					result.Matches = append(result.Matches, MatchSummary{
						CandidateName: c.Author,
						JobTitle:      bestJob.Title,
						Score:         bestScore,
						Reason:        bestReason,
						Outreach:      outreachType,
					})

					o.safeNotify(fmt.Sprintf(
						"🎯 Match: %s → %s (score %.2f)\n💬 %s\n📤 %s",
						c.Author, bestJob.Title, bestScore, bestReason, outreachType,
					))
					// Human-like delay between candidate contacts: 30s–3min
					jitterSleep(ctx, 30*time.Second, 3*time.Minute)
				}
			}

			// Human-like delay between groups: 15s–90s
			jitterSleep(ctx, 15*time.Second, 90*time.Second)
		}
	}

	// ── Phase 8: Start processing queued outreach ──────────────────────────────
	if result.CommentsQueued+result.InboxesQueued > 0 {
		_, _ = o.queue.Submit(models.Job{Type: models.JobAutoComment, Platform: models.PlatformFacebook, Target: "auto"})
	}

	// ── Phase 9: Post JDs to domain-matched groups (professional HR style) ────
	// Each JD gets posted to up to 2 matched groups (anti-spam).
	// Candidates will comment on these posts → pipeline Phase 4 picks them up next run.
	if o.msgGen != nil && defaultAccountID >= 0 {
		postCtx, postCancel := context.WithTimeout(ctx, 5*time.Minute)
		defer postCancel()

		// Track groups used in this phase — each group posted to at most once per run
		// to prevent within-run quota exhaustion across different jobs.
		phaseUsedGroups := make(map[int64]bool)
		phaseRunStart := time.Now()

		for _, pair := range jobGroupPairs {
			if postCtx.Err() != nil || strings.ToLower(pair.job.Priority) == "low" {
				continue
			}
			if len(pair.groups) == 0 {
				log.Printf("[Pipeline] No groups to post JD for %q — skipping", pair.job.Title)
				continue
			}

			// Generate professional JD post
			content, err := o.msgGen.GenerateJobPost(postCtx, pair.job.Title, pair.job.Description,
				pair.job.Requirements, pair.job.Benefits, pair.job.Salary, pair.job.Email)
			if err != nil || content == "" {
				log.Printf("[Pipeline] GenerateJobPost failed for %q: %v", pair.job.Title, err)
				continue
			}

			// Attach JD card image
			var imgPath string
			if img, err := o.db.GetImageForCareerJob(pair.job.Title); err == nil {
				imgPath = img.LocalPath
			}

			// Post to up to 2 best groups with proper anti-spam:
			//   1. Skip groups already used for another job this run.
			//   2. Skip groups posted to within the last 3 days from a PREVIOUS run.
			posted := 0
			for _, group := range pair.groups {
				if posted >= 2 {
					break
				}
				if phaseUsedGroups[group.ID] {
					continue
				}
				if q, err := o.db.GetGroupQuality(group.ID); err == nil && q != nil {
					if !q.LastPostAt.IsZero() && q.LastPostAt.Before(phaseRunStart) {
						cooldown := groupCooldown(q.FinalScore, pair.job.Priority)
						if time.Since(q.LastPostAt) < cooldown {
							log.Printf("[Pipeline] Cooldown: %q (score=%.2f, priority=%s) → %.0fh remaining",
								group.Name, q.FinalScore, pair.job.Priority, (cooldown - time.Since(q.LastPostAt)).Hours())
							continue
						}
					}
				}

				msg := &models.OutboundMessage{
					Type:       "group_post",
					Platform:   models.PlatformFacebook,
					AccountID:  defaultAccountID,
					TargetURL:  group.URL,
					TargetName: pair.job.Title,
					Content:    content,
					ImagePath:  imgPath,
					Context:    fmt.Sprintf("JD: %s | Group: %s | Priority: %s", pair.job.Title, group.Name, pair.job.Priority),
					Status:     models.OutboundApproved,
					AIModel:    "gpt-4o-mini",
				}
				if _, err := o.db.InsertOutboundMessage(msg); err == nil {
					result.PostsCreated++
					phaseUsedGroups[group.ID] = true
					_ = o.db.UpdateGroupLastPost(group.ID)
					posted++
					log.Printf("[Pipeline] Queued JD post: %q → group %q", pair.job.Title, group.Name)
				}
			}
		}

		// Trigger autocommenter to process the queued group_posts
		if result.PostsCreated > 0 {
			_, _ = o.queue.Submit(models.Job{Type: models.JobAutoComment, Platform: models.PlatformFacebook, Target: "auto"})
			o.safeNotify(fmt.Sprintf("📝 Đã queue %d bài JD để đăng vào groups tuyển dụng — HR Agent sẽ đăng tuần tự", result.PostsCreated))
		}
	}

	result.Duration = time.Since(startTime)

	// ── Final summary ──────────────────────────────────────────────────────────
	summary := buildPipelineSummary(result)
	o.safeNotify(summary)
	return result, nil
}

// scoreBestJobMatch scores the candidate against all jobs and returns the best fitting one.
// Returns nil if no job scored above 0.
func (o *Orchestrator) scoreBestJobMatch(ctx context.Context, candidateContent, postContext string, jobs []models.CareerJob) (*models.CareerJob, float64, string) {
	if o.msgGen == nil {
		return nil, 0, ""
	}
	var bestJob *models.CareerJob
	bestScore := 0.0
	bestReason := ""

	for i := range jobs {
		scoreCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		score, reason, err := o.msgGen.ScoreCandidateMatch(scoreCtx, candidateContent, postContext, jobs[i])
		cancel()
		if err != nil {
			log.Printf("[Pipeline] ScoreCandidateMatch failed for %q: %v", jobs[i].Title, err)
			continue
		}
		if score > bestScore {
			bestScore = score
			bestReason = reason
			j := jobs[i]
			bestJob = &j
		}
	}
	return bestJob, bestScore, bestReason
}

func formatJobList(jobs []models.CareerJob) string {
	lines := make([]string, 0, len(jobs))
	for _, j := range jobs {
		p := j.Priority
		if p == "" {
			p = "medium"
		}
		lines = append(lines, fmt.Sprintf("• %s [%s, score %d]", j.Title, p, j.UrgencyScore))
	}
	return strings.Join(lines, "\n")
}

func buildPipelineSummary(r *PipelineResult) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "✅ Pipeline hoàn tất! (%s)\n\n", r.Duration.Round(time.Second))
	fmt.Fprintf(&sb, "📋 Jobs xử lý: %d\n", r.JobsProcessed)
	fmt.Fprintf(&sb, "👥 Ứng viên tìm thấy: %d\n", r.CandidatesFound)
	fmt.Fprintf(&sb, "🎯 Matches hợp lệ: %d\n", r.UniqueMatches)
	fmt.Fprintf(&sb, "💬 Comments queued: %d\n", r.CommentsQueued)
	fmt.Fprintf(&sb, "📩 Inbox DMs queued: %d\n", r.InboxesQueued)
	fmt.Fprintf(&sb, "📄 Job posts drafted: %d\n", r.PostsCreated)
	fmt.Fprintf(&sb, "⏭ Bỏ qua: %d (domain mismatch / đã liên hệ)\n", r.Skipped)
	if len(r.Matches) > 0 {
		fmt.Fprintf(&sb, "\n🏆 Top matches:\n")
		shown := r.Matches
		if len(shown) > 8 {
			shown = shown[:8]
		}
		for _, m := range shown {
			fmt.Fprintf(&sb, "  • %s → %s (%.2f) [%s]\n", m.CandidateName, m.JobTitle, m.Score, m.Outreach)
		}
	}
	return sb.String()
}

// dedupeGroups removes duplicate groups by ID.
func dedupeGroups(groups []models.Group) []models.Group {
	seen := make(map[int64]bool)
	var out []models.Group
	for _, g := range groups {
		if !seen[g.ID] {
			seen[g.ID] = true
			out = append(out, g)
		}
	}
	return out
}

// normalizeGroupCategory maps AI-scored categories to standard domain keys.
// The AI scorer may return various category names — normalize to: tech, sales, ops, finance.
// Uses word-padded matching to avoid substring collisions (e.g. "it" in "recruitment", "ai" in "chain").
func normalizeGroupCategory(cat string) string {
	c := " " + strings.ToLower(strings.TrimSpace(cat)) + " "
	switch {
	// "it" and "ai" must be whole words — pad with spaces to avoid "recruitment"/"chain" collisions
	case strings.Contains(c, "tech") || strings.Contains(c, " it ") || strings.Contains(c, "develop") ||
		strings.Contains(c, "engineer") || strings.Contains(c, " ai ") || strings.Contains(c, "data") ||
		strings.Contains(c, "software") || strings.Contains(c, "lập trình"):
		return "tech"
	case strings.Contains(c, "sale") || strings.Contains(c, "kinh doanh") || strings.Contains(c, "marketing") ||
		strings.Contains(c, "business") || strings.Contains(c, "commercial"):
		return "sales"
	case strings.Contains(c, "ops") || strings.Contains(c, "operation") || strings.Contains(c, "logistics") ||
		strings.Contains(c, "warehouse") || strings.Contains(c, "ecommerce") || strings.Contains(c, "vận hành") ||
		strings.Contains(c, "shipping") || strings.Contains(c, "kho"):
		return "ops"
	case strings.Contains(c, "finance") || strings.Contains(c, "accounting") || strings.Contains(c, "kế toán") ||
		strings.Contains(c, "tài chính"):
		return "finance"
	case strings.Contains(c, "hr") || strings.Contains(c, "recruit") || strings.Contains(c, "tuyển dụng") ||
		strings.Contains(c, "nhân sự"):
		// HR/Recruitment groups are useful for ALL domains
		return "general"
	default:
		return "general"
	}
}

// PostJDsToExistingGroups is a LIGHTWEIGHT method that:
// 1. Takes pre-filtered jobs (caller handles filtering by positions)
// 2. Maps to EXISTING scored groups (NO searching/discovering)
// 3. Generates professional JD posts via AI
// 4. Queues them as group_post to be posted on Facebook
// Use this when user says "tạo bài viết tuyển dụng" — no scraping needed.
func (o *Orchestrator) PostJDsToExistingGroups(ctx context.Context, jobs []models.CareerJob) (int, error) {
	if len(jobs) == 0 {
		return 0, fmt.Errorf("không có vị trí tuyển dụng — hãy cào careers trước")
	}

	// Get default account
	var defaultAccountID int64 = -1
	if o.accountMgr != nil {
		if acc, err := o.accountMgr.GetNextAccount(models.PlatformFacebook); err == nil {
			defaultAccountID = acc.ID
		}
	}
	if defaultAccountID < 0 {
		return 0, fmt.Errorf("chưa có tài khoản Facebook nào active")
	}

	if o.msgGen == nil {
		return 0, fmt.Errorf("AI message generator chưa được cấu hình")
	}

	// Load ALL scored groups from DB
	allGroups, _ := o.db.GetActiveGroups(models.PlatformFacebook)
	domainGroups := make(map[string][]models.Group)
	var allQualityGroups []models.Group
	skippedReasons := make(map[string]int)
	for _, g := range allGroups {
		q, err := o.db.GetGroupQuality(g.ID)
		if err != nil {
			skippedReasons["unscored"]++
			continue
		}
		if q.Blacklist {
			skippedReasons["blacklisted"]++
			continue
		}
		if q.FinalScore < 0.1 {
			skippedReasons["low_score"]++
			continue
		}
		// Normalize category → standard domain keys
		cat := normalizeGroupCategory(q.Category)
		domainGroups[cat] = append(domainGroups[cat], g)
		allQualityGroups = append(allQualityGroups, g)
	}

	// Debug log
	var domainCounts []string
	for k, v := range domainGroups {
		domainCounts = append(domainCounts, fmt.Sprintf("%s:%d", k, len(v)))
	}
	log.Printf("[PostJDs] Groups: quality=%d, domains=[%s], skipped=%v",
		len(allQualityGroups), strings.Join(domainCounts, ", "), skippedReasons)

	o.safeNotify(fmt.Sprintf("📝 Tạo bài viết cho %d vị trí → %d groups tuyển dụng phù hợp", len(jobs), len(allQualityGroups)))

	// usedGroupThisRun tracks groups posted to in this run so different jobs don't
	// exhaust the cross-run weekly quota against each other. The DB counter only
	// governs how often we return to the SAME group across separate pipeline runs.
	usedGroupThisRun := make(map[int64]bool)
	runStart := time.Now()

	posted := 0
	var skippedPositions []string
	for _, job := range jobs {
		if ctx.Err() != nil {
			break
		}
		if strings.ToLower(job.Priority) == "low" {
			continue
		}

		domain := ai.JobDomainCategory(job)
		groups := domainGroups[domain]
		// Fallback: tuyển dụng / general groups serve all domains
		if len(groups) == 0 && len(domainGroups["general"]) > 0 {
			groups = domainGroups["general"]
			log.Printf("[PostJDs] Using 'general' groups for %q [%s]", job.Title, domain)
		}
		// Last resort: any scored quality group
		if len(groups) == 0 && len(allQualityGroups) > 0 {
			groups = allQualityGroups
			log.Printf("[PostJDs] Using all-quality fallback for %q", job.Title)
		}

		// If still nothing, try searching Facebook for this domain
		if len(groups) == 0 && o.fbScraper != nil {
			searchQuery := domainSearchQuery(domain, job.Title)
			o.safeNotify(fmt.Sprintf("🔍 Tìm groups mới cho [%s]: %q", domain, searchQuery))
			searchCtx, searchCancel := context.WithTimeout(ctx, 90*time.Second)
			found, err := o.fbScraper.SearchGroups(searchCtx, searchQuery)
			searchCancel()
			if err == nil {
				for _, ng := range found {
					clean := strings.Split(ng.URL, "?")[0]
					if o.db.GroupExistsByURL(clean) {
						continue
					}
					g := &models.Group{Platform: models.PlatformFacebook, Name: ng.Name, URL: clean, Active: true, JoinState: "none"}
					if id, err := o.db.AddGroup(g); err == nil {
						g.ID = id
						groups = append(groups, *g)
						allQualityGroups = append(allQualityGroups, *g)
						log.Printf("[PostJDs] Discovered new group: %q", ng.Name)
					}
				}
			}
		}

		if len(groups) == 0 {
			log.Printf("[PostJDs] No groups available for %q [%s]", job.Title, domain)
			skippedPositions = append(skippedPositions, fmt.Sprintf("%s (không tìm thấy groups tuyển dụng phù hợp)", job.Title))
			continue
		}

		// Generate professional JD post
		genCtx, genCancel := context.WithTimeout(ctx, 30*time.Second)
		content, err := o.msgGen.GenerateJobPost(genCtx, job.Title, job.Description,
			job.Requirements, job.Benefits, job.Salary, job.Email)
		genCancel()
		if err != nil || content == "" {
			log.Printf("[PostJDs] GenerateJobPost failed for %q: %v", job.Title, err)
			skippedPositions = append(skippedPositions, fmt.Sprintf("%s (AI error)", job.Title))
			continue
		}

		// Attach JD card image if available
		var imgPath string
		if img, err := o.db.GetImageForCareerJob(job.Title); err == nil {
			imgPath = img.LocalPath
		}

		// Post to up to 2 groups per job.
		// Anti-spam rules:
		//   1. Each group used at most once per run (prevents exhausting quota across jobs in the same run).
		//   2. Cross-run: dynamic cooldown based on group quality score + job priority.
		jobPosted := 0
		for _, group := range dedupeGroups(groups) {
			if jobPosted >= 2 {
				break
			}
			// Rule 1: skip if already used this run
			if usedGroupThisRun[group.ID] {
				continue
			}
			// Rule 2: dynamic cross-run cooldown
			if q, err := o.db.GetGroupQuality(group.ID); err == nil && q != nil {
				if !q.LastPostAt.IsZero() && q.LastPostAt.Before(runStart) {
					cooldown := groupCooldown(q.FinalScore, job.Priority)
					if time.Since(q.LastPostAt) < cooldown {
						log.Printf("[PostJDs] Cooldown: %q (score=%.2f, priority=%s) → wait %.0fh more",
							group.Name, q.FinalScore, job.Priority, (cooldown - time.Since(q.LastPostAt)).Hours())
						continue
					}
				}
			}

			msg := &models.OutboundMessage{
				Type:       "group_post",
				Platform:   models.PlatformFacebook,
				AccountID:  defaultAccountID,
				TargetURL:  group.URL,
				TargetName: job.Title,
				Content:    content,
				ImagePath:  imgPath,
				Context:    fmt.Sprintf("JD: %s | Group: %s | Domain: %s", job.Title, group.Name, domain),
				Status:     models.OutboundApproved,
				AIModel:    "gpt-4o-mini",
			}
			if _, err := o.db.InsertOutboundMessage(msg); err == nil {
				usedGroupThisRun[group.ID] = true
				_ = o.db.UpdateGroupLastPost(group.ID)
				jobPosted++
				posted++
				log.Printf("[PostJDs] ✅ Queued: %q → %q", job.Title, group.Name)
				preview := content
				if len(preview) > 300 {
					preview = preview[:300] + "..."
				}
				o.safeNotify(fmt.Sprintf("📝 Bài viết tuyển dụng — **%s**\n🏢 Group: %s\n\n%s", job.Title, group.Name, preview))
			}
		}
		if jobPosted == 0 {
			skippedPositions = append(skippedPositions, fmt.Sprintf("%s (tất cả groups đang trong cooldown hoặc đã được dùng)", job.Title))
		}
	}

	// Trigger auto-commenter to actually post them
	if posted > 0 {
		_, _ = o.queue.Submit(models.Job{Type: models.JobAutoComment, Platform: models.PlatformFacebook, Target: "auto"})
		summary := fmt.Sprintf("✅ Đã tạo %d bài JD — HR Agent đang đăng vào groups tuyển dụng...\n💡 Ứng viên sẽ comment lên bài → Pipeline sẽ tự xử lý", posted)
		if len(skippedPositions) > 0 {
			summary += fmt.Sprintf("\n\n⏭️ Bỏ qua %d vị trí:\n• %s", len(skippedPositions), strings.Join(skippedPositions, "\n• "))
		}
		o.safeNotify(summary)
	} else {
		if len(skippedPositions) > 0 {
			o.safeNotify(fmt.Sprintf("⚠️ Không đăng được bài nào:\n• %s\n\n💡 Hãy thêm groups tuyển dụng hoặc chờ hết cooldown 3 ngày", strings.Join(skippedPositions, "\n• ")))
		} else {
			o.safeNotify("⚠️ Không tìm thấy groups phù hợp để đăng bài. Hãy chạy pipeline đầy đủ trước: 'chạy pipeline tuyển dụng'")
		}
	}

	return posted, nil
}

// groupCooldown returns the minimum time between posts to a group, based on
// its quality score and the job's priority. High-quality groups for urgent jobs
// can be reused sooner; low-quality groups need more breathing room.
func groupCooldown(finalScore float64, priority string) time.Duration {
	switch strings.ToLower(priority) {
	case "high":
		if finalScore >= 0.7 {
			return 24 * time.Hour // high quality + urgent job → 1 day
		}
		return 3 * 24 * time.Hour
	case "low":
		return 7 * 24 * time.Hour
	default: // medium
		if finalScore >= 0.7 {
			return 2 * 24 * time.Hour
		}
		return 3 * 24 * time.Hour
	}
}

// domainSearchQuery returns a Facebook group search query for a given job domain.
func domainSearchQuery(domain, _ string) string {
	switch domain {
	case "tech":
		return "tuyển dụng IT developer engineer HCM"
	case "sales":
		return "tuyển dụng sales kinh doanh logistics HCM"
	case "ops":
		return "tuyển dụng vận hành operations logistics ecommerce HCM"
	case "finance":
		return "tuyển dụng kế toán accountant finance HCM"
	default:
		return "tuyển dụng việc làm HCM"
	}
}

// ScanOwnJDPosts scans groups where we recently posted JDs for candidate comments.
// Commenters become leads in "tuyen_dung" niche, and @mention replies are queued.
// This is the feedback loop: Post JDs → candidates comment → leads in dashboard.
func (o *Orchestrator) ScanOwnJDPosts(ctx context.Context) (int, error) {
	if o.fbScraper == nil {
		return 0, fmt.Errorf("Facebook scraper not initialized")
	}

	// Get all group_posts we created in the last 7 days
	sentPosts, err := o.db.GetSentGroupPosts(7)
	if err != nil || len(sentPosts) == 0 {
		return 0, fmt.Errorf("chưa có bài JD nào được đăng — hãy tạo bài viết tuyển dụng trước")
	}

	// Deduplicate by group URL (one group may have multiple JD posts)
	seenGroups := make(map[string]bool)
	var uniqueGroupURLs []string
	var accountID int64
	for _, p := range sentPosts {
		if !seenGroups[p.TargetURL] {
			seenGroups[p.TargetURL] = true
			uniqueGroupURLs = append(uniqueGroupURLs, p.TargetURL)
			accountID = p.AccountID
		}
	}

	o.safeNotify(fmt.Sprintf("🔍 Quét %d groups đã đăng JD để tìm ứng viên...", len(uniqueGroupURLs)))

	totalCandidates := 0
	for _, groupURL := range uniqueGroupURLs {
		if ctx.Err() != nil {
			break
		}

		// Scrape the group for recent posts
		group := models.Group{URL: groupURL, Platform: models.PlatformFacebook}
		scrapeCtx, scrapeCancel := context.WithTimeout(ctx, 3*time.Minute)
		posts, err := o.fbScraper.ScrapeGroup(scrapeCtx, group)
		scrapeCancel()
		if err != nil {
			log.Printf("[ScanJDPosts] ScrapeGroup error %s: %v", groupURL, err)
			continue
		}

		for _, post := range posts {
			if post.URL == "" || ctx.Err() != nil {
				continue
			}
			// Use scanCommentsForCandidates — it creates leads with niche=tuyen_dung
			o.scanCommentsForCandidates(ctx, post, accountID)
			totalCandidates++ // count posts scanned
		}

		jitterSleep(ctx, 3*time.Second, 8*time.Second)
	}

	if totalCandidates > 0 {
		o.safeNotify(fmt.Sprintf("✅ Quét xong %d groups — các ứng viên sẽ hiện trong tab Tuyển dụng", len(uniqueGroupURLs)))
	}

	return totalCandidates, nil
}
