package agentloop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/gofiber/fiber/v2"
)

// RunRequest is the JSON body for POST /api/agent/run.
type RunRequest struct {
	Task           string   `json:"task"`
	Logs           string   `json:"logs,omitempty"`
	AvailableFiles []string `json:"available_files,omitempty"`
	DomainHint     string   `json:"domain_hint,omitempty"`
	// VerifyConfig overrides (optional — server defaults are used if omitted).
	CDPPort       int    `json:"cdp_port,omitempty"`
	VNCPort       int    `json:"vnc_port,omitempty"`
	FrontendURL   string `json:"frontend_url,omitempty"`
	ContainerName string `json:"container_name,omitempty"`
}

// Handler wraps AgentLoop as a Fiber HTTP handler.
// POST /api/agent/run → RunResult JSON
//
// The caller must be an admin — this endpoint applies patches to live files.
type Handler struct {
	loop    *AgentLoop
	baseDir string
	timeout time.Duration
}

// NewHandler creates a Fiber-compatible handler for the agent loop.
// baseDir is the repo root (used to enumerate files when AvailableFiles is empty).
// timeout caps the entire agent run (default: 5 minutes).
func NewHandler(apiKey, plannerModel, architectModel, baseDir string, defaultVerify VerifyConfig, timeout time.Duration) *Handler {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &Handler{
		loop:    New(apiKey, plannerModel, architectModel, baseDir, defaultVerify),
		baseDir: baseDir,
		timeout: timeout,
	}
}

// Handle is the Fiber handler for POST /api/agent/run.
func (h *Handler) Handle(c *fiber.Ctx) error {
	var req RunRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body: " + err.Error()})
	}
	if req.Task == "" {
		return c.Status(400).JSON(fiber.Map{"error": "task is required"})
	}

	// Build verify config — request overrides take precedence over server defaults.
	verifyCfg := h.loop.verifier.cfg
	if req.CDPPort > 0 {
		verifyCfg.CDPPort = req.CDPPort
	}
	if req.VNCPort > 0 {
		verifyCfg.VNCPort = req.VNCPort
	}
	if req.FrontendURL != "" {
		verifyCfg.FrontendURL = req.FrontendURL
	}
	if req.ContainerName != "" {
		verifyCfg.ContainerName = req.ContainerName
	}

	// Auto-enumerate Go source files if caller didn't provide a list.
	availFiles := req.AvailableFiles
	if len(availFiles) == 0 {
		availFiles = enumerateGoFiles(h.baseDir)
	}

	task := Task{
		Description:    req.Task,
		Logs:           req.Logs,
		AvailableFiles: availFiles,
		DomainHint:     req.DomainHint,
	}

	// Build a per-request agent loop with the overridden verify config.
	loop := New(
		h.loop.planner.apiKey,
		h.loop.planner.model,
		h.loop.architect.model,
		h.baseDir,
		verifyCfg,
	)

	ctx, cancel := context.WithTimeout(c.Context(), h.timeout)
	defer cancel()

	result := loop.Run(ctx, task)

	status := 200
	if result.State == StateFailed || result.State == StateAborted || result.State == StatePoison {
		status = 500
	}
	return c.Status(status).JSON(result)
}

// HandleStatus returns the set of available agent domains and configuration.
// GET /api/agent/status
func (h *Handler) HandleStatus(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"available_domains": []string{"browser", "frontend", "infra", "job", "unknown"},
		"max_iterations":    MaxIterations,
		"planner_model":     h.loop.planner.model,
		"architect_model":   h.loop.architect.model,
		"pass_threshold":    VerifyPassThreshold,
		"poison_threshold":  PoisonThreshold,
		"verify_config": fiber.Map{
			"cdp_port":       h.loop.verifier.cfg.CDPPort,
			"vnc_port":       h.loop.verifier.cfg.VNCPort,
			"frontend_url":   h.loop.verifier.cfg.FrontendURL,
			"container_name": h.loop.verifier.cfg.ContainerName,
		},
	})
}

// enumerateGoFiles returns all *.go files under baseDir (relative paths).
func enumerateGoFiles(baseDir string) []string {
	var files []string
	_ = filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".go" {
			rel, _ := filepath.Rel(baseDir, path)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	return files
}

// DecisionTraceResponse wraps a RunResult for the trace endpoint.
type DecisionTraceResponse struct {
	TraceID    string       `json:"trace_id"`
	State      AgentState   `json:"state"`
	Iterations int          `json:"iterations"`
	Score      float64      `json:"verify_score"`
	Reason     string       `json:"reason"`
	Entries    []TraceEntry `json:"entries"`
}

// FormatTrace converts a RunResult into a structured trace response.
func FormatTrace(r RunResult) DecisionTraceResponse {
	b, _ := json.Marshal(r)
	var tr DecisionTraceResponse
	_ = json.Unmarshal(b, &tr)
	tr.Entries = r.Trace
	return tr
}
