package skills

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/thg/scraper/internal/jobs"
)

// TaskExecutor resolves the target account, submits a skill_run job,
// and returns the job ID immediately (async execution per spec).
type TaskExecutor struct {
	registry       *Registry
	jobStore       *jobs.Store
	defaultAccount int64
}

func NewTaskExecutor(reg *Registry, jobStore *jobs.Store, defaultAccountID int64) *TaskExecutor {
	return &TaskExecutor{
		registry:       reg,
		jobStore:       jobStore,
		defaultAccount: defaultAccountID,
	}
}

// SkillPayload is the JSON payload stored in the jobs table for skill_run jobs.
type SkillPayload struct {
	SkillName string         `json:"skill"`
	AccountID int64          `json:"account_id"`
	Params    map[string]any `json:"params"`
}

// Execute submits a skill_run job and returns the task_id for polling.
// The job is idempotent: duplicate Telegram messages with the same params
// do not double-execute (same task_id via deterministic hash).
func (e *TaskExecutor) Execute(ctx context.Context, skillName string, params map[string]any, accountID int64) (taskID string, err error) {
	skill := e.registry.Get(skillName)
	if skill == nil {
		return "", fmt.Errorf("executor: unknown skill %q", skillName)
	}

	if accountID == 0 {
		accountID = e.defaultAccount
	}

	payload := SkillPayload{SkillName: skillName, AccountID: accountID, Params: params}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("executor: marshal payload: %w", err)
	}

	// Deterministic task_id: skill_run:account:<id>:<skill>:<hash(params)[:8]>
	taskID = skillRunTaskID(skillName, accountID, params)

	task := &jobs.Task{
		TaskID:    taskID,
		AccountID: accountID,
		Intent:    fmt.Sprintf("skill_run:%s (account %d)", skillName, accountID),
		OrgID:     0,
		CrawlPlan: jobs.CrawlPlan{
			Sources: []jobs.Source{{Type: "skill", URL: skillName}},
		},
	}

	job, err := e.jobStore.Submit(ctx, task, string(payloadJSON))
	if err != nil {
		return "", fmt.Errorf("executor: submit job: %w", err)
	}

	log.Printf("[skills] Submitted skill_run job: task_id=%s job_id=%d skill=%s account=%d",
		taskID, job.ID, skillName, accountID)
	return taskID, nil
}

func skillRunTaskID(skillName string, accountID int64, params map[string]any) string {
	// Sort params for determinism
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	fmt.Fprintf(h, "skill_run:account:%d:%s:", accountID, skillName)
	for _, k := range keys {
		fmt.Fprintf(h, "%s=%v,", k, params[k])
	}
	// Include UTC day so same command next day is a new job
	fmt.Fprintf(h, "day=%s", time.Now().UTC().Format("2006-01-02"))
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}
