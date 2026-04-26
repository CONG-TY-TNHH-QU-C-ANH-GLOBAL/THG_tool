package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/thg/scraper/internal/events"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/learning"
	"github.com/thg/scraper/internal/parser"
	"github.com/thg/scraper/internal/store"
)

// Server exposes the full SaaS API: task submission, job monitoring,
// results/leads retrieval, dashboard stats, and real-time SSE stream.
type Server struct {
	jobStore *jobs.Store
	appStore *store.AppStore
	parser   parser.Parser
	bus      *events.Bus
	learner  *learning.Engine
	mux      *http.ServeMux

	// SSE poller state
	seenMu   sync.Mutex
	seenJobs map[int64]jobSnapshot

	// Lead SSE poller — tracks last seen task_lead ID
	lastLeadID atomic.Int64
}

type jobSnapshot struct {
	status   string
	progress int
}

// DashboardStats is the response for GET /api/v1/dashboard/stats.
type DashboardStats struct {
	TotalJobs     int     `json:"total_jobs"`
	PendingJobs   int     `json:"pending_jobs"`
	RunningJobs   int     `json:"running_jobs"`
	CompletedJobs int     `json:"completed_jobs"`
	FailedJobs    int     `json:"failed_jobs"`
	TotalLeads    int     `json:"total_leads"`
	HotLeads      int     `json:"hot_leads"`
	WarmLeads     int     `json:"warm_leads"`
	ColdLeads     int     `json:"cold_leads"`
	SuccessRate   float64 `json:"success_rate"` // completed / (completed+failed) * 100
}

func New(jobStore *jobs.Store, appStore *store.AppStore, p parser.Parser, bus *events.Bus, learner *learning.Engine) *Server {
	s := &Server{
		jobStore: jobStore,
		appStore: appStore,
		parser:   p,
		bus:      bus,
		learner:  learner,
		mux:      http.NewServeMux(),
		seenJobs: make(map[int64]jobSnapshot),
	}
	s.routes()
	return s
}

// ServeHTTP wraps the mux with CORS middleware.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.mux.ServeHTTP(w, r)
}

// Start launches background pollers for SSE. Call before serving HTTP.
func (s *Server) Start(ctx context.Context) {
	go s.pollJobs(ctx)
	if s.appStore != nil {
		go s.pollLeads(ctx)
	}
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)

	// Task submission + lookup
	s.mux.HandleFunc("POST /api/v1/tasks", s.handleSubmitTask)
	s.mux.HandleFunc("GET /api/v1/tasks/{task_id}", s.handleGetTask)

	// Job queue
	s.mux.HandleFunc("GET /api/v1/jobs", s.handleListJobs)

	// Multi-tenant results + leads
	s.mux.HandleFunc("GET /api/v1/results", s.handleListResults)
	s.mux.HandleFunc("GET /api/v1/leads", s.handleListLeads)

	// Dashboard stats
	s.mux.HandleFunc("GET /api/v1/dashboard/stats", s.handleDashboardStats)

	// Browser intelligence
	s.mux.HandleFunc("GET /api/v1/sessions", s.handleListSessions)
	s.mux.HandleFunc("GET /api/v1/identities", s.handleListIdentities)

	// Self-learning
	s.mux.HandleFunc("GET /api/v1/learning", s.handleGetLearning)
	s.mux.HandleFunc("POST /api/v1/leads/{id}/outcome", s.handleRecordOutcome)

	// Real-time SSE stream
	s.mux.HandleFunc("GET /api/v1/events/stream", s.handleSSE)
}

// ── handlers ──────────────────────────────────────────────────────────────────

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"subscribers": s.bus.Len(),
	})
}

type submitRequest struct {
	Text  string `json:"text"`
	OrgID int64  `json:"org_id"`
}

func (s *Server) handleSubmitTask(w http.ResponseWriter, r *http.Request) {
	var req submitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid JSON: "+err.Error()))
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		writeJSON(w, http.StatusBadRequest, errResp("text is required"))
		return
	}

	task, err := s.parser.Parse(r.Context(), req.Text)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errResp(err.Error()))
		return
	}
	task.OrgID = req.OrgID

	payload, err := json.Marshal(task)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errResp("marshal task: "+err.Error()))
		return
	}

	job, err := s.jobStore.Submit(r.Context(), task, string(payload))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errResp("submit: "+err.Error()))
		return
	}

	if s.appStore != nil {
		if err := s.appStore.CreateTask(r.Context(), task.TaskID, task.OrgID, task.Intent); err != nil {
			log.Printf("api: create app task: %v", err)
		}
	}

	s.bus.Publish(events.Event{
		Type:   "job.created",
		JobID:  job.ID,
		TaskID: job.TaskID,
		Status: job.Status,
	})

	writeJSON(w, http.StatusAccepted, map[string]any{
		"task_id": job.TaskID,
		"job_id":  job.ID,
		"status":  job.Status,
		"intent":  job.Intent,
	})
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	if taskID == "" {
		writeJSON(w, http.StatusBadRequest, errResp("task_id required"))
		return
	}
	job, err := s.jobStore.GetByTaskID(r.Context(), taskID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSON(w, http.StatusNotFound, errResp("task not found"))
			return
		}
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit := parseLimit(r, 50, 200)
	list, err := s.jobStore.List(r.Context(), status, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}
	if list == nil {
		list = []jobs.Job{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": list, "count": len(list)})
}

func (s *Server) handleListResults(w http.ResponseWriter, r *http.Request) {
	if s.appStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, errResp("app store not configured"))
		return
	}
	orgID := parseInt64(r.URL.Query().Get("org_id"), 0)
	intent := r.URL.Query().Get("intent")
	status := r.URL.Query().Get("status")
	limit := parseLimit(r, 50, 200)
	offset := parseInt(r.URL.Query().Get("offset"), 0)

	tasks, err := s.appStore.ListTasks(r.Context(), orgID, intent, status, limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}
	if tasks == nil {
		tasks = []store.AppTask{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": tasks, "count": len(tasks)})
}

func (s *Server) handleListLeads(w http.ResponseWriter, r *http.Request) {
	if s.appStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, errResp("app store not configured"))
		return
	}
	orgID := parseInt64(r.URL.Query().Get("org_id"), 0)
	category := r.URL.Query().Get("category")
	keyword := r.URL.Query().Get("keyword")
	minScore := parseFloat(r.URL.Query().Get("min_score"), 0)
	limit := parseLimit(r, 50, 200)
	offset := parseInt(r.URL.Query().Get("offset"), 0)

	leads, err := s.appStore.ListLeads(r.Context(), orgID, category, keyword, minScore, limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}
	if leads == nil {
		leads = []store.TaskLead{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"leads": leads, "count": len(leads)})
}

func (s *Server) handleDashboardStats(w http.ResponseWriter, r *http.Request) {
	orgID := parseInt64(r.URL.Query().Get("org_id"), 0)

	jc, err := s.jobStore.GetStatusCounts(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}

	var lc store.LeadCounts
	if s.appStore != nil {
		lc, _ = s.appStore.GetLeadCounts(r.Context(), orgID)
	}

	total := jc.Completed + jc.Failed
	var successRate float64
	if total > 0 {
		successRate = float64(jc.Completed) / float64(total) * 100
	}

	writeJSON(w, http.StatusOK, DashboardStats{
		TotalJobs:     jc.Pending + jc.Running + jc.Completed + jc.Failed,
		PendingJobs:   jc.Pending,
		RunningJobs:   jc.Running,
		CompletedJobs: jc.Completed,
		FailedJobs:    jc.Failed,
		TotalLeads:    lc.Total,
		HotLeads:      lc.Hot,
		WarmLeads:     lc.Warm,
		ColdLeads:     lc.Cold,
		SuccessRate:   successRate,
	})
}

// ── Browser intelligence handlers ────────────────────────────────────────────

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	if s.appStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, errResp("app store not configured"))
		return
	}
	orgID := parseInt64(r.URL.Query().Get("org_id"), 0)
	sessions, err := s.appStore.ListSessions(r.Context(), orgID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}
	if sessions == nil {
		sessions = []store.BrowserSession{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions, "count": len(sessions)})
}

func (s *Server) handleListIdentities(w http.ResponseWriter, r *http.Request) {
	if s.appStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, errResp("app store not configured"))
		return
	}
	orgID := parseInt64(r.URL.Query().Get("org_id"), 0)
	identities, err := s.appStore.ListIdentities(r.Context(), orgID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}
	if identities == nil {
		identities = []store.BrowserIdentity{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"identities": identities, "count": len(identities)})
}

// ── Self-learning handlers ────────────────────────────────────────────────────

func (s *Server) handleGetLearning(w http.ResponseWriter, r *http.Request) {
	if s.appStore == nil || s.learner == nil {
		writeJSON(w, http.StatusServiceUnavailable, errResp("learning not configured"))
		return
	}
	orgID := parseInt64(r.URL.Query().Get("org_id"), 0)

	profile, err := s.appStore.GetLearningProfile(r.Context(), orgID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}

	history, err := s.appStore.ListLearningHistory(r.Context(), orgID, 50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}
	if history == nil {
		history = []store.LearningHistoryEntry{}
	}

	converted, rejected, ignored, lastUpdated := s.learner.Stats(orgID)
	liveWeights := s.learner.GetCurrentWeights(r.Context(), orgID)

	writeJSON(w, http.StatusOK, map[string]any{
		"profile":      profile,
		"history":      history,
		"live_weights": liveWeights,
		"outcome_counts": map[string]any{
			"converted": converted,
			"rejected":  rejected,
			"ignored":   ignored,
		},
		"last_updated": lastUpdated,
	})
}

type outcomeRequest struct {
	OrgID   int64   `json:"org_id"`
	Outcome string  `json:"outcome"` // converted|rejected|ignored
	Score   float64 `json:"score"`
}

func (s *Server) handleRecordOutcome(w http.ResponseWriter, r *http.Request) {
	if s.appStore == nil || s.learner == nil {
		writeJSON(w, http.StatusServiceUnavailable, errResp("learning not configured"))
		return
	}
	leadID := parseInt64(r.PathValue("id"), 0)
	if leadID == 0 {
		writeJSON(w, http.StatusBadRequest, errResp("invalid lead id"))
		return
	}

	var req outcomeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errResp("invalid JSON: "+err.Error()))
		return
	}
	if req.Outcome == "" {
		writeJSON(w, http.StatusBadRequest, errResp("outcome is required"))
		return
	}

	// Record in DB
	ev := store.OutcomeEvent{
		OrgID:     req.OrgID,
		LeadID:    leadID,
		Outcome:   req.Outcome,
		Score:     req.Score,
		CreatedAt: time.Now(),
	}
	if err := s.appStore.InsertOutcomeEvent(r.Context(), ev); err != nil {
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}

	// Feed learning engine
	sig := learning.OutcomeSignal{
		OrgID:   req.OrgID,
		LeadID:  leadID,
		Outcome: req.Outcome,
		Score:   req.Score,
	}
	if err := s.learner.ProcessOutcome(r.Context(), sig); err != nil {
		log.Printf("api: learning ProcessOutcome: %v", err)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "recorded"})
}

// ── SSE handler ───────────────────────────────────────────────────────────────

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	id, ch := s.bus.Subscribe()
	defer s.bus.Unsubscribe(id)

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			fmt.Fprintf(w, ": keep-alive\n\n")
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// ── background pollers ────────────────────────────────────────────────────────

// pollJobs polls the jobs table every 500ms and publishes job state change events.
func (s *Server) pollJobs(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			jobList, err := s.jobStore.List(ctx, "", 200)
			if err != nil {
				continue
			}
			s.seenMu.Lock()
			for _, j := range jobList {
				prev, known := s.seenJobs[j.ID]
				if !known {
					s.bus.Publish(events.Event{
						Type:     "job.created",
						JobID:    j.ID,
						TaskID:   j.TaskID,
						Status:   j.Status,
						Progress: j.Progress,
					})
					s.seenJobs[j.ID] = jobSnapshot{j.Status, j.Progress}
					continue
				}
				if prev.status != j.Status || prev.progress != j.Progress {
					evType := "job." + j.Status
					if prev.status == j.Status {
						evType = "job.progress"
					}
					s.bus.Publish(events.Event{
						Type:     evType,
						JobID:    j.ID,
						TaskID:   j.TaskID,
						Status:   j.Status,
						Progress: j.Progress,
					})
					s.seenJobs[j.ID] = jobSnapshot{j.Status, j.Progress}
				}
			}
			s.seenMu.Unlock()
		}
	}
}

// pollLeads polls task_leads for new rows and publishes lead.inserted events.
func (s *Server) pollLeads(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			newLeads, err := s.appStore.ListLeadsSince(ctx, s.lastLeadID.Load(), 50)
			if err != nil || len(newLeads) == 0 {
				continue
			}
			for _, lead := range newLeads {
				payload, _ := json.Marshal(lead)
				s.bus.Publish(events.Event{
					Type:    "lead.inserted",
					TaskID:  lead.TaskID,
					Payload: payload,
				})
				s.lastLeadID.Store(lead.ID)
			}
		}
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: encode response: %v", err)
	}
}

func errResp(msg string) map[string]string { return map[string]string{"error": msg} }

func parseLimit(r *http.Request, def, max int) int {
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= max {
			return n
		}
	}
	return def
}

func parseInt(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func parseInt64(s string, def int64) int64 {
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	return def
}

func parseFloat(s string, def float64) float64 {
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return def
}
