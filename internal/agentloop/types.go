package agentloop

import "time"

// Task is the input to the agent loop.
type Task struct {
	Description    string   `json:"task"`
	Logs           string   `json:"logs,omitempty"`
	AvailableFiles []string `json:"available_files,omitempty"`
	// Domain hint — leave empty to let the Planner decide.
	DomainHint string `json:"domain_hint,omitempty"`
}

// Domain classifies the kind of system the task operates on.
type Domain string

const (
	DomainBrowser  Domain = "browser"
	DomainFrontend Domain = "frontend"
	DomainInfra    Domain = "infra"
	DomainJob      Domain = "job"
	DomainUnknown  Domain = "unknown"
)

// PlannerResult is the Planner's output — classification only, no patches.
type PlannerResult struct {
	Domain     Domain  `json:"domain"`
	Intent     string  `json:"intent"`
	RootCause  string  `json:"root_cause"`
	Confidence float64 `json:"confidence"`
}

// PatchAction describes how a patch modifies a file.
type PatchAction string

const (
	ActionReplaceBlock  PatchAction = "replace_block"  // replace from target string to end of block
	ActionInsertAfter   PatchAction = "insert_after"   // insert content after the target line
	ActionDeleteBlock   PatchAction = "delete_block"   // delete the block starting at target
	ActionPrependImport PatchAction = "prepend_import" // add import if not present
	ActionAppend        PatchAction = "append"         // append to end of file
)

// Patch is one atomic file modification.
type Patch struct {
	File    string      `json:"file"`
	Action  PatchAction `json:"action"`
	Target  string      `json:"target"`  // function/block identifier, or line prefix to match
	Content string      `json:"content"` // replacement or inserted content
	Why     string      `json:"why"`     // architect's rationale (not written to file)
}

// ArchitectResult is the Architect's patch plan.
type ArchitectResult struct {
	Patches    []Patch `json:"patches"`
	Confidence float64 `json:"confidence"`
	Risk       string  `json:"risk"`      // "low" | "medium" | "high"
	Rationale  string  `json:"rationale"` // overall explanation
}

// VerifySignals holds per-signal confidence weights (sum must be ≤ 1.0).
type VerifySignals struct {
	DOM       float64 `json:"dom,omitempty"`
	Stream    float64 `json:"stream,omitempty"`
	HTTP      float64 `json:"http,omitempty"`
	Container float64 `json:"container,omitempty"`
	API       float64 `json:"api,omitempty"`
	Job       float64 `json:"job,omitempty"`
}

// VerifyResult is the outcome of one verification pass.
type VerifyResult struct {
	Pass    bool          `json:"pass"`
	Score   float64       `json:"score"` // 0.0–1.0; Pass requires ≥ VerifyPassThreshold
	Signals VerifySignals `json:"signals"`
	Reason  string        `json:"reason"`
}

// VerifyPassThreshold is the minimum score for a verification to be considered passing.
const VerifyPassThreshold = 0.70

// AgentState represents where the agent is in its lifecycle.
type AgentState string

const (
	StateIdle          AgentState = "IDLE"
	StatePlanning      AgentState = "PLANNING"
	StatePatching      AgentState = "PATCHING"
	StateVerifying     AgentState = "VERIFYING"
	StateSuccess       AgentState = "SUCCESS"
	StateFailed        AgentState = "FAILED"
	StateAborted       AgentState = "ABORTED"        // budget or max-iter exceeded
	StatePoison        AgentState = "POISON"          // same patch failed 3×
	StateHumanRequired AgentState = "HUMAN_REQUIRED" // confidence too low or invariant violated
)

// TraceEntry is one recorded decision in the decision trace.
type TraceEntry struct {
	TraceID    string    `json:"trace_id"`
	Iteration  int       `json:"iteration"`
	Step       string    `json:"step"`       // "planner" | "architect" | "executor" | "verifier"
	Decision   string    `json:"decision"`   // short description of what was decided
	Reason     string    `json:"reason"`     // why this decision was made
	Confidence float64   `json:"confidence"` // LLM confidence (0 if not applicable)
	Result     string    `json:"result"`     // "ok" | "failed" | "skipped" | "poison" | "human"
	LatencyMS  int64     `json:"latency_ms"`
	At         time.Time `json:"at"`
}

// LedgerEntry records one applied (or failed) patch.
type LedgerEntry struct {
	PatchHash string    `json:"patch_hash"`
	File      string    `json:"file"`
	Status    string    `json:"status"` // "applied" | "failed" | "reverted"
	FailCount int       `json:"fail_count"`
	At        time.Time `json:"at"`
}

// RunResult is what the AgentLoop returns to its caller.
type RunResult struct {
	State       AgentState   `json:"state"`
	TraceID     string       `json:"trace_id"`
	Iterations  int          `json:"iterations"`
	VerifyScore float64      `json:"verify_score,omitempty"`
	Reason      string       `json:"reason,omitempty"`
	Trace       []TraceEntry `json:"trace"`
}

// VerifyConfig carries domain-specific parameters for the verifier.
type VerifyConfig struct {
	// Browser / infra
	CDPPort int
	VNCPort int
	// ExpectedFBUserID is the Facebook numeric user ID (c_user cookie value)
	// of the account that should be logged in. When set, the browser verifier
	// will reject a session that belongs to a different account.
	// Leave empty to skip identity verification.
	ExpectedFBUserID string
	// Frontend
	FrontendURL string
	// Infra
	ContainerName string
	// Job
	JobDBPath string
}
