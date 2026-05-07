package facebookcrawl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/filter"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/leadingest"
	"github.com/thg/scraper/internal/livesession"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/output"
	"github.com/thg/scraper/internal/runtime"
	"github.com/thg/scraper/internal/scoring"
	"github.com/thg/scraper/internal/session"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/textutil"
)

// Handler implements jobs.Handler for all crawl-based intents.
// Filters AND scoring run inline per item — nothing is accumulated before evaluation.
type Handler struct {
	rt        runtime.Runtime // optional mock/runtime fallback for development only
	fe        *filter.Engine
	scorer    *scoring.Scorer
	jobStore  *jobs.Store
	appStore  *store.AppStore
	allocator *session.Allocator
	lsFactory *livesession.LiveSessionFactory
	ctxStore  *store.Store
	aiClass   *ai.MessageGenerator
}

func New(rt runtime.Runtime, scorer *scoring.Scorer, jobStore *jobs.Store, appStore *store.AppStore) *Handler {
	return &Handler{
		rt:       rt,
		fe:       filter.New(),
		scorer:   scorer,
		jobStore: jobStore,
		appStore: appStore,
	}
}

// SetAllocator wires in the session allocator + live session factory.
// When set, Handle() acquires a real browser session per job.
func (h *Handler) SetAllocator(a *session.Allocator, f *livesession.LiveSessionFactory) {
	h.allocator = a
	h.lsFactory = f
}

// SetUniversalClassifier enables prompt/business-profile driven lead
// classification. It is optional; without it the deterministic scorer remains
// the fallback.
func (h *Handler) SetUniversalClassifier(ctxStore *store.Store, mg *ai.MessageGenerator) {
	h.ctxStore = ctxStore
	h.aiClass = mg
}

func (h *Handler) Handle(ctx context.Context, job *jobs.Job) (string, error) {
	var task jobs.Task
	if err := json.Unmarshal([]byte(job.Payload), &task); err != nil {
		return "", fmt.Errorf("unmarshal task payload: %w", err)
	}

	// Attempt to acquire a live browser session for real CDP scraping.
	// h.rt is an optional development fallback; production keeps it nil.
	rt := h.rt
	var acquireErr error
	if h.allocator != nil && h.lsFactory != nil {
		workerID := uuid.New().String()
		accountID := task.AccountID
		policy := session.PolicyAny
		if accountID != 0 {
			policy = session.PolicySticky
		}
		if sess, err := h.allocator.Acquire(ctx, accountID, policy, workerID); err == nil {
			ls, lsErr := h.lsFactory.Wrap(*sess, workerID)
			if lsErr != nil {
				slog.WarnContext(ctx, "wrap live session failed, using fallback runtime",
					"error", lsErr, "job_id", job.ID)
				_ = h.allocator.Release(ctx, sess.AccountID, workerID)
			} else {
				rt = ls.Runtime()
				sessionCtx, sessionCancel := context.WithCancel(ctx)
				defer func() {
					sessionCancel()
					closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_ = ls.Close(closeCtx)
				}()
				// Heartbeat every 30s so the health checker knows the session is in use.
				go func() {
					ticker := time.NewTicker(30 * time.Second)
					defer ticker.Stop()
					for {
						select {
						case <-sessionCtx.Done():
							return
						case <-ticker.C:
							if err := ls.Heartbeat(sessionCtx); err != nil {
								slog.WarnContext(sessionCtx, "session heartbeat failed",
									"account_id", ls.AccountID(), "error", err)
							}
						}
					}
				}()
			}
		} else {
			acquireErr = err
		}
		// ErrNoIdleSession is non-fatal — continue with fallback runtime
	}

	if rt == nil {
		if acquireErr != nil {
			if task.AccountID != 0 {
				return "", fmt.Errorf("facebook account %d has no logged-in idle browser session available for real crawl: %w", task.AccountID, acquireErr)
			}
			return "", fmt.Errorf("no logged-in idle Facebook browser session available for real crawl: %w", acquireErr)
		}
		return "", errors.New("no browser runtime configured; start and log in a Facebook workspace first")
	}

	// Ensure task row exists, then mark running (both calls are idempotent).
	if h.appStore != nil {
		if err := h.appStore.CreateTask(ctx, job.TaskID, task.OrgID, job.Intent); err != nil {
			log.Printf("handler: create app task %s: %v", job.TaskID, err)
		}
		if err := h.appStore.StartTask(ctx, job.TaskID); err != nil {
			log.Printf("handler: start app task %s: %v", job.TaskID, err)
		}
	}

	// Hard cost/time boundary — aborts before any limit is breached.
	budget := runtime.NewBudget(runtime.DefaultBudget)

	scorerCfg := scoringConfig(task.ScoringConfig)
	sc := scoring.New(scorerCfg)
	var businessProfile *ai.BusinessProfile
	var scoreGuidance scoring.Guidance
	if h.ctxStore != nil {
		if p := ai.LoadProfileForOrg(h.ctxStore, task.OrgID); p != nil && p.IsConfigured() {
			scoreGuidance = scoring.Guidance{
				TargetAuthorRole: p.TargetAuthorRole,
				TargetSignals:    ai.SplitSignalPhrases(p.TargetSignals),
				RejectPhrases:    ai.SplitSignalPhrases(strings.Join([]string{p.NegativeSignals, p.RejectRules}, "\n")),
			}
			if h.aiClass != nil && h.aiClass.Available() {
				businessProfile = p
			}
		}
	}
	var gate leadingest.SignalGate
	var userPrompt string
	if task.Extras != nil {
		if raw, ok := task.Extras["market_signal_gate"].(map[string]any); ok {
			gate = leadingest.SignalGateFromMap(raw)
		}
		if up, ok := task.Extras["user_prompt"].(string); ok {
			userPrompt = strings.TrimSpace(up)
		}
	}

	filterCfg := filter.Config{
		Keywords:         task.Filters.Keywords,
		ExcludePhrases:   task.Filters.ExcludePhrases,
		MinContentLength: task.Filters.MinContentLength,
		MinReactions:     task.Filters.MinReactions,
		KeywordMinScore:  task.Filters.KeywordMinScore,
	}

	maxItems := task.CrawlPlan.MaxItems
	if maxItems <= 0 {
		maxItems = 100
	}
	batchSize := task.CrawlPlan.BatchSize
	if batchSize <= 0 {
		batchSize = 20
	}

	estimatedTotal := maxItems * len(task.CrawlPlan.Sources)
	if estimatedTotal <= 0 {
		estimatedTotal = maxItems
	}

	var (
		records []output.Record
		stats   output.Stats
		seen    = map[string]bool{}
	)

	for _, src := range task.CrawlPlan.Sources {
		if stats.TotalReturned >= maxItems {
			break
		}

		offset := 0
		for {
			if stats.TotalReturned >= maxItems {
				break
			}

			// Invariant BUDGET: hard-stop before issuing network/LLM call.
			budget.RecordBatch()
			if err := budget.CheckOrAbort(); err != nil {
				slog.WarnContext(ctx, "budget exceeded — aborting job",
					"job_id", job.ID, "elapsed", budget.Elapsed())
				return buildResult(records, stats)
			}

			rawItems, err := rt.FetchBatch(ctx, src.URL, offset, batchSize)
			if err != nil {
				// Invariant CHECKPOINT: human-verification gate detected — never retry.
				var cdpErr runtime.CDPError
				if errors.As(err, &cdpErr) && runtime.IsBanSignal(err) {
					slog.WarnContext(ctx, "ban signal from Facebook",
						"job_id", job.ID, "code", cdpErr.Code.String(), "src", src.URL)
					if cdpErr.Code == runtime.ErrFacebookCheckpoint {
						return `{"status":"human_required","reason":"facebook_checkpoint"}`, nil
					}
					// Logout / banned — abort, no human intervention queued.
					return `{"status":"aborted","reason":"` + cdpErr.Code.String() + `"}`, nil
				}
				return "", fmt.Errorf("fetch batch (src=%s offset=%d): %w", src.URL, offset, err)
			}
			if len(rawItems) == 0 {
				break
			}

			for _, item := range rawItems {
				stats.TotalFetched++

				if seen[item.ID] {
					stats.TotalDeduped++
					continue
				}
				if src.Type == "facebook_search" {
					seen[item.ID] = true
					if h.ctxStore != nil && item.SourceURL != "" {
						_, _ = h.ctxStore.AddGroup(&models.Group{
							OrgID:     task.OrgID,
							Platform:  models.PlatformFacebook,
							Name:      textutil.FirstNonEmpty(item.AuthorName, item.SourceURL),
							URL:       item.SourceURL,
							Active:    true,
							JoinState: "none",
						})
					}
					records = append(records, output.Record{
						ID:               item.ID,
						Content:          item.Content,
						AuthorName:       item.AuthorName,
						AuthorProfileURL: item.AuthorProfileURL,
						SourceURL:        item.SourceURL,
						Timestamp:        item.Timestamp,
						LeadScore:        50,
						Category:         "source_candidate",
						Signals:          []string{"source_discovery"},
					})
					stats.TotalReturned++
					if stats.TotalReturned >= maxItems {
						break
					}
					continue
				}

				// Stage 1: filter (inline discard)
				fr := h.fe.Evaluate(
					item.Content, item.AuthorProfileURL,
					item.Reactions, item.Timestamp, filterCfg,
				)
				if !fr.Pass {
					stats.TotalFiltered++
					continue
				}

				// Stage 2: classify + persist via shared pipeline so worker
				// and Chrome Extension behave identically (same AI, same
				// gate, same mirror to legacy leads table).
				outcome, err := leadingest.IngestPost(ctx, leadingest.Deps{
					AppStore:        h.appStore,
					LegacyDB:        h.ctxStore,
					Scorer:          sc,
					Guidance:        scoreGuidance,
					BusinessProfile: businessProfile,
					AIClass:         h.aiClass,
					SignalGate:      gate,
					Keywords:        task.Keywords,
					UserPrompt:      userPrompt,
				}, leadingest.Input{
					TaskID:           job.TaskID,
					OrgID:            task.OrgID,
					SourceURL:        item.SourceURL,
					AuthorName:       item.AuthorName,
					AuthorProfileURL: item.AuthorProfileURL,
					Content:          item.Content,
					Reactions:        item.Reactions,
					Comments:         item.Comments,
					Shares:           item.Shares,
				})
				if err != nil {
					log.Printf("handler: ingest post: %v", err)
				}
				if outcome.Skipped == "rejected" || outcome.Skipped == "gate_negative" {
					stats.TotalFiltered++
					continue
				}

				seen[item.ID] = true
				sr := scoring.Result{
					Score:    outcome.Score,
					Category: outcome.Category,
					Signals:  outcome.Signals,
				}
				rec := toRecord(item, fr.Signals, sr)
				records = append(records, rec)
				stats.TotalReturned++

				if stats.TotalReturned >= maxItems {
					break
				}
			}

			// Report progress after each batch.
			if h.jobStore != nil && estimatedTotal > 0 {
				pct := min(99, stats.TotalFetched*100/estimatedTotal)
				if err := h.jobStore.UpdateProgress(ctx, job.ID, pct); err != nil {
					log.Printf("handler: update progress: %v", err)
				}
			}

			offset += len(rawItems)

			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}

	// Mark application task complete.
	if h.appStore != nil {
		if err := h.appStore.CompleteTask(ctx, job.TaskID, stats.TotalFetched, stats.TotalReturned); err != nil {
			log.Printf("handler: complete app task: %v", err)
		}
	}

	return buildResult(records, stats)
}

// ── helpers ───────────────────────────────────────────────────────────────────

// buildResult marshals the accumulated records and stats into the job result JSON.
// Used for early exits (budget exceeded, context cancelled).
func buildResult(records []output.Record, stats output.Stats) (string, error) {
	if records == nil {
		records = []output.Record{}
	}
	ds := output.Dataset{Records: records, Stats: stats, Insights: []any{}}
	b, err := json.Marshal(ds)
	if err != nil {
		return "", fmt.Errorf("marshal dataset: %w", err)
	}
	return string(b), nil
}


func toRecord(item runtime.RawItem, filterSignals []string, sr scoring.Result) output.Record {
	allSignals := make([]string, 0, len(filterSignals)+len(sr.Signals))
	allSignals = append(allSignals, filterSignals...)
	allSignals = append(allSignals, sr.Signals...)

	return output.Record{
		ID:               item.ID,
		Content:          item.Content,
		AuthorName:       item.AuthorName,
		AuthorProfileURL: item.AuthorProfileURL,
		SourceURL:        item.SourceURL,
		Timestamp:        item.Timestamp,
		Reactions:        item.Reactions,
		Comments:         item.Comments,
		Shares:           item.Shares,
		LeadScore:        sr.Score,
		Category:         sr.Category,
		Signals:          allSignals,
		FilterSignals:    filterSignals,
	}
}

func scoringConfig(cfg jobs.ScoringConfig) scoring.Config {
	if cfg.HotThreshold == 0 {
		return scoring.DefaultConfig()
	}
	return scoring.Config{
		HotThreshold:  cfg.HotThreshold,
		WarmThreshold: cfg.WarmThreshold,
		Weights: scoring.Weights{
			KeywordRelevance: cfg.Weights.KeywordRelevance,
			Engagement:       cfg.Weights.Engagement,
			ContentQuality:   cfg.Weights.ContentQuality,
		},
	}
}
