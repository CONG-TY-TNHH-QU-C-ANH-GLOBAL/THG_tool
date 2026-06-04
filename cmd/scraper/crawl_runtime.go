package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/thg/scraper/internal/browsergateway"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/textutil"
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
	extras := map[string]any{}
	if gate, ok := args["market_signal_gate"]; ok && gate != nil {
		extras["market_signal_gate"] = gate
	}
	if up := strings.TrimSpace(argString(args, "user_prompt")); up != "" {
		extras["user_prompt"] = up
	}
	task := &jobs.Task{
		SchemaVersion: "1",
		TaskID:        openCrawlTaskID(intent, sources, args),
		OrgID:         orgID,
		AccountID:     accountID,
		IntentID:      argInt64(args, "_intent_id"),
		Intent:        intent,
		Keywords:      keywords,
		CrawlPlan: jobs.CrawlPlan{
			Sources:          sources,
			MaxItems:         maxItems,
			BatchSize:        20,
			CursorLastPostID: strings.TrimSpace(argString(args, "_cursor_last_post_id")),
			CursorLastPostAt: parseRFC3339OrZero(argString(args, "_cursor_last_post_at")),
			SinceRunAt:       parseRFC3339OrZero(argString(args, "_since_run_at")),
		},
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
		Extras:              extras,
	}
	if db != nil && !argBool(args, "_recurring_run") && argInt64(args, "interval_minutes") > 0 {
		rememberRecurringCrawlIntents(ctx, db, task, args)
	}
	payload, err := json.Marshal(task)
	if err != nil {
		return "", err
	}
	if db != nil {
		if result, routed, err := submitConnectorCrawl(ctx, db, task, string(payload)); routed {
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
	screen, err := db.Connectors().GetLatestConnectorScreenshot(orgID, 0)
	if err != nil {
		return 0, err
	}
	if screen != nil &&
		screen.AccountID > 0 &&
		screen.AgentID > 0 &&
		strings.EqualFold(strings.TrimSpace(screen.StreamStatus), browsergateway.StreamFacebookLoggedIn) &&
		time.Since(screen.UpdatedAt) <= 5*time.Minute {
		return screen.AccountID, nil
	}
	accounts, err := db.Identities().GetAllAccounts(orgID)
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

func submitConnectorCrawl(ctx context.Context, db *store.Store, task *jobs.Task, payload string) (string, bool, error) {
	if task == nil || task.OrgID <= 0 || task.AccountID <= 0 {
		return "", false, nil
	}
	screen, err := db.Connectors().GetLatestConnectorScreenshot(task.OrgID, task.AccountID)
	if err != nil {
		return "", true, err
	}
	if screen != nil && screen.AgentID > 0 && strings.EqualFold(strings.TrimSpace(screen.StreamStatus), browsergateway.StreamFacebookLoggedIn) && time.Since(screen.UpdatedAt) <= 5*time.Minute {
		result, err := enqueueConnectorCrawlCommand(ctx, db, task, payload, screen.AgentID)
		return result, true, err
	}
	// connectorReason carries the PRECISE per-connector rejection detail that
	// pickOnlineConnectorForCrawl computed (assigned to a different account /
	// fb_user_id mismatch / status=facebook_login_required / none paired). It is
	// logged AND surfaced in the operator-facing error below — without this the
	// operator only saw the generic "not online" and could not tell "log in" from
	// "you're on the wrong FB account" from "this connector belongs to account #X".
	connectorReason := ""
	if agentID, reason := pickOnlineConnectorForCrawl(db, task); agentID > 0 {
		result, err := enqueueConnectorCrawlCommand(ctx, db, task, payload, agentID)
		return result, true, err
	} else if reason != "" {
		connectorReason = reason
		log.Printf("[ConnectorCrawl] no heartbeat-routable connector org=%d account=%d: %s", task.OrgID, task.AccountID, reason)
	}

	appStore, err := store.NewAppStore(db)
	if err != nil {
		return "", true, err
	}
	sess, _ := appStore.GetSession(ctx, task.AccountID)
	if sess != nil && sess.CDPPort > 0 && (sess.Status == "idle" || sess.Status == "ready" || sess.Status == "active") {
		return "", false, nil
	}
	msg := fmt.Sprintf("Facebook account #%d is saved, but THG Chrome Extension is not online for this account yet. Open Browser, pair the Chrome Extension, keep a logged-in Facebook tab open, then send the prompt again", task.AccountID)
	if connectorReason != "" {
		// The precise reason names which of the 5 routing gates failed, so the
		// operator knows exactly what to fix (log in / switch FB account / pair).
		msg += " — chi tiết: " + connectorReason
	}
	return "", true, fmt.Errorf("%s", msg)
}

// connectorCrawlEnvelope wraps the full Task JSON with flat top-level hints so
// the Chrome Extension can find the navigation target without deep JSON parsing.
// MarketSignalGate carries Brain's positive/negative signal hints to the
// extension and back to the crawl-result endpoint so AI classification can
// honor org-specific gating without re-reading context.
type connectorCrawlEnvelope struct {
	NavigateTo       string         `json:"navigate_to"`
	SourceType       string         `json:"source_type"`
	UseBackgroundTab bool           `json:"use_background_tab"`
	MarketSignalGate map[string]any `json:"market_signal_gate,omitempty"`
	UserPrompt       string         `json:"user_prompt,omitempty"`
	Task             *jobs.Task     `json:"task"`
	TaskID           string         `json:"task_id,omitempty"`
	Intent           string         `json:"intent,omitempty"`
	Keywords         []string       `json:"keywords,omitempty"`
	CrawlPlan        jobs.CrawlPlan `json:"crawl_plan"`
	Filters          jobs.Filters   `json:"filters,omitempty"`
}

func connectorCrawlEnvelopeForTask(task *jobs.Task) (connectorCrawlEnvelope, error) {
	if task == nil {
		return connectorCrawlEnvelope{}, fmt.Errorf("crawl task is required")
	}
	// Refuse early when no concrete navigation target exists. Without this,
	// the extension receives navigate_to="" and silently falls back to the
	// Facebook newsfeed instead of the intended group/post URL.
	if len(task.CrawlPlan.Sources) == 0 || strings.TrimSpace(task.CrawlPlan.Sources[0].URL) == "" {
		return connectorCrawlEnvelope{}, fmt.Errorf("crawl task has no concrete source URL; refusing to dispatch (prevents newsfeed fallback)")
	}

	env := connectorCrawlEnvelope{
		UseBackgroundTab: true,
		Task:             task,
		NavigateTo:       strings.TrimSpace(task.CrawlPlan.Sources[0].URL),
		SourceType:       task.CrawlPlan.Sources[0].Type,
		TaskID:           task.TaskID,
		Intent:           task.Intent,
		Keywords:         task.Keywords,
		CrawlPlan:        task.CrawlPlan,
		Filters:          task.Filters,
	}
	if gate, ok := task.Extras["market_signal_gate"].(map[string]any); ok && len(gate) > 0 {
		env.MarketSignalGate = gate
	}
	if up, ok := task.Extras["user_prompt"].(string); ok {
		env.UserPrompt = strings.TrimSpace(up)
	}
	return env, nil
}

func enqueueConnectorCrawlCommand(ctx context.Context, db *store.Store, task *jobs.Task, _ string, agentID int64) (string, error) {
	if agentID <= 0 {
		return "", fmt.Errorf("Chrome Extension connector id is required")
	}
	appStore, err := store.NewAppStore(db)
	if err != nil {
		return "", err
	}
	_ = appStore.CreateTask(ctx, task.TaskID, task.OrgID, task.Intent)
	_ = appStore.StartTask(ctx, task.TaskID)

	env, err := connectorCrawlEnvelopeForTask(task)
	if err != nil {
		_ = appStore.FailTask(ctx, task.TaskID, err.Error())
		return "", err
	}
	envPayload, envErr := json.Marshal(env)
	if envErr != nil {
		return "", fmt.Errorf("marshal connector envelope: %w", envErr)
	}
	log.Printf("[ConnectorCrawl] enqueue navigate_to=%s task=%s org=%d account=%d", env.NavigateTo, task.TaskID, task.OrgID, task.AccountID)
	cmdID, err := db.Connectors().CreateConnectorCommand(task.OrgID, task.AccountID, agentID, 0, "crawl", string(envPayload))
	if err != nil {
		_ = appStore.FailTask(ctx, task.TaskID, err.Error())
		return "", err
	}
	return fmt.Sprintf("da tao Chrome Extension crawler command #%d task=%s intent=%s mode=chrome_extension", cmdID, task.TaskID, task.Intent), nil
}

func pickOnlineConnectorForCrawl(db *store.Store, task *jobs.Task) (int64, string) {
	connectors, err := db.Connectors().ListLocalConnectors(task.OrgID)
	if err != nil {
		return 0, err.Error()
	}
	if len(connectors) == 0 {
		return 0, "no Chrome Extension connector paired"
	}
	acc, _ := db.Identities().GetAccountForOrg(task.AccountID, task.OrgID)
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
		if !strings.EqualFold(status, browsergateway.StreamFacebookLoggedIn) {
			reasons = append(reasons, fmt.Sprintf("connector #%d status=%s", conn.ID, textutil.FirstNonEmpty(status, "unknown")))
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
