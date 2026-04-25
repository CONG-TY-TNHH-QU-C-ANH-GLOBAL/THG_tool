package orchestrator

import (
	"context"
	"fmt"
	"log"
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
	pool           browser.Browser
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
func New(db *store.Store, pool browser.Browser, q *queue.Queue, bot *telegram.Bot, classifier *ai.Classifier, msgGen *ai.MessageGenerator, accountMgr *accounts.Manager, pricer *ai.PriceExtractor) *Orchestrator {
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

// ProcessAgentScrapedPosts runs the full AI pipeline on posts scraped by a local agent.
// Called by the server's agent handler after receiving job results.
func (o *Orchestrator) ProcessAgentScrapedPosts(ctx context.Context, groupURL string, posts []models.Post) {
	if len(posts) == 0 {
		return
	}

	// Resolve group for display name
	groupName := groupURL
	groups, _ := o.db.GetActiveGroups(models.PlatformFacebook)
	for _, g := range groups {
		if g.URL == groupURL {
			groupName = g.Name
			break
		}
	}

	var leads []models.Lead
	if o.ai != nil {
		for i := 0; i < len(posts); i += 10 {
			end := i + 10
			if end > len(posts) {
				end = len(posts)
			}
			batch, err := o.ai.ClassifyBatch(ctx, posts[i:end])
			if err != nil {
				log.Printf("[Orchestrator] Agent classify error: %v", err)
			} else {
				leads = append(leads, batch...)
			}
		}
	}

	if o.bot != nil {
		hotCount := 0
		for _, l := range leads {
			if l.Score == models.LeadHot {
				hotCount++
			}
		}
		o.bot.Notify(fmt.Sprintf("🤖 *Agent scan: %s*\n📝 %d posts | 🎯 %d leads | 🔥 %d hot",
			groupName, len(posts), len(leads), hotCount))
	}

	// Queue outbound comments for hot/warm leads (same logic as server scan)
	var defaultAccountID int64 = -1
	if o.accountMgr != nil {
		if acc, err := o.accountMgr.GetNextAccount(models.PlatformFacebook); err == nil {
			defaultAccountID = acc.ID
		}
	}
	queuedCount := 0
	for _, lead := range leads {
		if lead.SourceURL == "" || o.autoCommenter == nil || defaultAccountID < 0 {
			continue
		}
		if o.db.HasSentComment(lead.SourceURL) {
			continue
		}
		commentContent := fmt.Sprintf("Chào %s, bạn có thể liên hệ để được tư vấn thêm nhé!", lead.Author)
		if o.msgGen != nil {
			if c, err := o.msgGen.GenerateCommentWithService(ctx, lead.Content, lead.Author, "", lead.ServiceMatch, ""); err == nil && c != "" {
				commentContent = c
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
			Status:     models.OutboundApproved,
			AIModel:    "agent",
		}
		if _, err := o.db.InsertOutboundMessage(msg); err == nil {
			queuedCount++
		}
	}
	if queuedCount > 0 {
		o.safeNotify(fmt.Sprintf("📬 Agent: queued %d comments", queuedCount))
	}
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
