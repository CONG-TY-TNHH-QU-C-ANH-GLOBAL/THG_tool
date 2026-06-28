package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/thg/scraper/internal/browsergateway"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/connectors"
)

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

// pickOnlineConnectorForCrawl resolves the connector that will run a crawl for
// task.AccountID. It delegates the eligibility decision to the SHARED
// connectors.PickReadyConnector so the run-time picker and the create-time
// mission preflight (readiness.EvaluateCrawlAccountReadiness) never diverge.
// Returns (connectorID, "") on success, or (0, typed-reason) otherwise.
func pickOnlineConnectorForCrawl(db *store.Store, task *jobs.Task) (int64, string) {
	conns, err := db.Connectors().ListLocalConnectors(task.OrgID)
	if err != nil {
		return 0, err.Error()
	}
	expectedFB := ""
	if acc, _ := db.Identities().GetAccountForOrg(task.AccountID, task.OrgID); acc != nil {
		expectedFB = acc.FBUserID
	}
	policy, _ := db.Connectors().GetExtensionPolicy()
	id, reason := connectors.PickReadyConnector(conns, task.AccountID, expectedFB, policy)
	if reason == connectors.ConnReady {
		return id, ""
	}
	return 0, reason
}
