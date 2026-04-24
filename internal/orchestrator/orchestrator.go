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
