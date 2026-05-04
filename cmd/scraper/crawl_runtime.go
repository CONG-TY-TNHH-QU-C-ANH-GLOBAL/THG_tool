package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

func submitOpenCrawl(ctx context.Context, db *store.Store, jobStore *jobs.Store, intent string, sources []jobs.Source, args map[string]any) (string, error) {
	if len(sources) == 0 {
		return "", fmt.Errorf("crawler requires at least one source")
	}
	maxItems := int(argInt64(args, "max_items"))
	if maxItems <= 0 {
		maxItems = int(argInt64(args, "limit"))
	}
	if maxItems <= 0 {
		maxItems = maxItemsFromPrompt(argString(args, "user_prompt"))
	}
	if maxItems <= 0 {
		maxItems = 50
	}
	keywords := splitKeywords(argString(args, "keywords"))
	if len(keywords) == 0 {
		keywords = splitKeywords(promptKeywordFallback(argString(args, "user_prompt")))
	}
	orgID := argInt64(args, "org_id")
	accountID := argInt64(args, "account_id")
	if accountID <= 0 && orgID > 0 && db != nil {
		if pickedAccountID, err := pickReadyFacebookAccountIDForCrawl(db, orgID); err == nil && pickedAccountID > 0 {
			accountID = pickedAccountID
			args["account_id"] = pickedAccountID
		}
	}
	task := &jobs.Task{
		SchemaVersion: "1",
		TaskID:        openCrawlTaskID(intent, sources, args),
		OrgID:         orgID,
		AccountID:     accountID,
		Intent:        intent,
		Keywords:      keywords,
		CrawlPlan:     jobs.CrawlPlan{Sources: sources, MaxItems: maxItems, BatchSize: 20},
		Filters:       jobs.Filters{Keywords: keywords, MinContentLength: 20, KeywordMinScore: 0},
		ScoringConfig: jobs.ScoringConfig{
			HotThreshold:  70,
			WarmThreshold: 40,
			Weights: jobs.ScoringWeights{
				KeywordRelevance: 0.4,
				Engagement:       0.2,
				ContentQuality:   0.4,
			},
		},
		RetryPolicy:         jobs.RetryPolicy{MaxAttempts: 3, BackoffMs: 1000},
		ExecutionMode:       "async",
		OutputSchema:        "open_crawler_v1",
		OutputSchemaVersion: "1",
	}
	if db != nil && !argBool(args, "_recurring_run") {
		rememberRecurringCrawlIntents(ctx, db, task, args)
	}
	payload, err := json.Marshal(task)
	if err != nil {
		return "", err
	}
	if db != nil {
		if result, routed, err := submitLocalRuntimeCrawl(ctx, db, task, string(payload)); routed {
			return result, err
		}
	}
	job, err := jobStore.Submit(ctx, task, string(payload))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("da tao crawler job #%d task=%s intent=%s", job.ID, job.TaskID, intent), nil
}

func pickReadyFacebookAccountIDForCrawl(db *store.Store, orgID int64) (int64, error) {
	screen, err := db.GetLatestConnectorScreenshot(orgID, 0)
	if err != nil {
		return 0, err
	}
	if screen != nil &&
		screen.AccountID > 0 &&
		screen.AgentID > 0 &&
		strings.EqualFold(strings.TrimSpace(screen.StreamStatus), "facebook_logged_in") &&
		time.Since(screen.UpdatedAt) <= 5*time.Minute {
		return screen.AccountID, nil
	}
	accounts, err := db.GetAllAccounts(orgID)
	if err != nil {
		return 0, err
	}
	for _, acc := range accounts {
		if acc.Platform == models.PlatformFacebook &&
			acc.BrowserLoggedIn &&
			acc.Status == models.AccountActive &&
			strings.TrimSpace(acc.FBUserID) != "" {
			return acc.ID, nil
		}
	}
	return 0, nil
}

func submitLocalRuntimeCrawl(ctx context.Context, db *store.Store, task *jobs.Task, payload string) (string, bool, error) {
	if task == nil || task.OrgID <= 0 || task.AccountID <= 0 {
		return "", false, nil
	}
	screen, err := db.GetLatestConnectorScreenshot(task.OrgID, task.AccountID)
	if err != nil {
		return "", true, err
	}
	if screen != nil && screen.AgentID > 0 && strings.EqualFold(strings.TrimSpace(screen.StreamStatus), "facebook_logged_in") && time.Since(screen.UpdatedAt) <= 5*time.Minute {
		result, err := enqueueLocalRuntimeCrawlCommand(ctx, db, task, payload, screen.AgentID)
		return result, true, err
	}
	if agentID, reason := pickOnlineRuntimeAgentForCrawl(db, task); agentID > 0 {
		result, err := enqueueLocalRuntimeCrawlCommand(ctx, db, task, payload, agentID)
		return result, true, err
	} else if reason != "" {
		log.Printf("[LocalCrawl] no heartbeat-routable runtime org=%d account=%d: %s", task.OrgID, task.AccountID, reason)
	}

	appStore, err := store.NewAppStore(db)
	if err != nil {
		return "", true, err
	}
	sess, _ := appStore.GetSession(ctx, task.AccountID)
	if sess != nil && sess.CDPPort > 0 && (sess.Status == "idle" || sess.Status == "ready" || sess.Status == "active") {
		return "", false, nil
	}
	return "", true, fmt.Errorf("Facebook account #%d is saved, but THG Local Runtime is not online for this account yet. Open Browser, run THG Local Kit, wait for Facebook local ready, then send the prompt again", task.AccountID)
}

func enqueueLocalRuntimeCrawlCommand(ctx context.Context, db *store.Store, task *jobs.Task, payload string, agentID int64) (string, error) {
	if agentID <= 0 {
		return "", fmt.Errorf("local runtime agent id is required")
	}
	appStore, err := store.NewAppStore(db)
	if err != nil {
		return "", err
	}
	_ = appStore.CreateTask(ctx, task.TaskID, task.OrgID, task.Intent)
	_ = appStore.StartTask(ctx, task.TaskID)
	cmdID, err := db.CreateConnectorCommand(task.OrgID, task.AccountID, agentID, 0, "crawl", payload)
	if err != nil {
		_ = appStore.FailTask(ctx, task.TaskID, err.Error())
		return "", err
	}
	return fmt.Sprintf("da tao local crawler command #%d task=%s intent=%s mode=local_runtime", cmdID, task.TaskID, task.Intent), nil
}

func pickOnlineRuntimeAgentForCrawl(db *store.Store, task *jobs.Task) (int64, string) {
	connectors, err := db.ListLocalConnectors(task.OrgID)
	if err != nil {
		return 0, err.Error()
	}
	if len(connectors) == 0 {
		return 0, "no local connector paired"
	}
	acc, _ := db.GetAccountForOrg(task.AccountID, task.OrgID)
	var reasons []string
	for _, conn := range connectors {
		if !conn.Online {
			reasons = append(reasons, fmt.Sprintf("connector #%d offline", conn.ID))
			continue
		}
		if conn.AssignedAccountID > 0 && conn.AssignedAccountID != task.AccountID {
			reasons = append(reasons, fmt.Sprintf("connector #%d assigned to account #%d", conn.ID, conn.AssignedAccountID))
			continue
		}
		status := strings.TrimSpace(conn.StreamStatus)
		if !strings.EqualFold(status, "facebook_logged_in") {
			reasons = append(reasons, fmt.Sprintf("connector #%d status=%s", conn.ID, firstNonEmpty(status, "unknown")))
			continue
		}
		if strings.TrimSpace(conn.FBUserID) == "" {
			reasons = append(reasons, fmt.Sprintf("connector #%d missing fb_user_id", conn.ID))
			continue
		}
		if acc != nil && strings.TrimSpace(acc.FBUserID) != "" && strings.TrimSpace(conn.FBUserID) != strings.TrimSpace(acc.FBUserID) {
			reasons = append(reasons, fmt.Sprintf("connector #%d fb_user_id mismatch", conn.ID))
			continue
		}
		return conn.ID, ""
	}
	return 0, strings.Join(reasons, "; ")
}

func openCrawlTaskID(intent string, sources []jobs.Source, args map[string]any) string {
	if taskID := argString(args, "_task_id"); strings.HasPrefix(taskID, "autocrawl-") {
		return taskID
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s|day=%s|", intent, time.Now().UTC().Format("2006-01-02"))
	for _, src := range sources {
		fmt.Fprintf(h, "%s:%s|", src.Type, src.URL)
	}
	fmt.Fprintf(h, "org=%d|account=%d", argInt64(args, "org_id"), argInt64(args, "account_id"))
	return fmt.Sprintf("open-crawl-%x", h.Sum(nil))[:27]
}
