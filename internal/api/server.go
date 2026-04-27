package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/thg/scraper/internal/events"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/learning"
	"github.com/thg/scraper/internal/parser"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/workspace"
)

// Server exposes the full SaaS API: task submission, job monitoring,
// results/leads retrieval, dashboard stats, and real-time SSE stream.
type Server struct {
	jobStore *jobs.Store
	appStore *store.AppStore
	parser   parser.Parser
	bus      *events.Bus
	learner  *learning.Engine
	wm       *workspace.Manager
	mux      *http.ServeMux

	// SSE poller state
	seenMu   sync.Mutex
	seenJobs map[int64]jobSnapshot

	// Lead SSE poller — tracks last seen task_lead ID
	lastLeadID atomic.Int64
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  65536,
	WriteBufferSize: 65536,
	CheckOrigin:     func(*http.Request) bool { return true },
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

// SetWorkspaceManager wires in the Docker workspace manager for browser routes.
func (s *Server) SetWorkspaceManager(wm *workspace.Manager) {
	s.wm = wm
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

	// Browser workspace routes
	s.mux.HandleFunc("GET /api/v1/browser/workspaces", s.handleListWorkspaces)
	s.mux.HandleFunc("POST /api/v1/browser/workspaces/{id}/start", s.handleStartWorkspace)
	s.mux.HandleFunc("POST /api/v1/browser/workspaces/{id}/stop", s.handleStopWorkspace)
	s.mux.HandleFunc("POST /api/v1/browser/workspaces/{id}/mark-logged-in", s.handleMarkLoggedIn)
	s.mux.HandleFunc("GET /ws/vnc/{id}", s.handleVNCProxy)
	s.mux.HandleFunc("GET /vnc-viewer/{id}", s.handleVNCViewer)
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

// ── Browser workspace handlers ────────────────────────────────────────────────

type workspaceItem struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Status          string `json:"status"`
	Running         bool   `json:"running"`
	CDPPort         int    `json:"cdp_port,omitempty"`
	VNCPort         int    `json:"vnc_port,omitempty"`
	ContainerID     string `json:"container_id,omitempty"`
	BrowserLoggedIn bool   `json:"browser_logged_in"`
}

func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	if s.appStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, errResp("app store not configured"))
		return
	}
	accounts, err := s.appStore.GetFacebookAccounts(r.Context(), 0)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}
	items := make([]workspaceItem, 0, len(accounts))
	for _, acc := range accounts {
		item := workspaceItem{
			ID:              acc.ID,
			Name:            acc.Name,
			Status:          acc.Status,
			BrowserLoggedIn: acc.BrowserLoggedIn,
		}
		if s.wm != nil {
			if inst := s.wm.Get(acc.ID); inst != nil {
				item.Running = true
				item.CDPPort = inst.CDPPort
				item.VNCPort = inst.VNCPort
				item.ContainerID = inst.ContainerID
			}
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": items, "count": len(items)})
}

func (s *Server) handleStartWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.wm == nil {
		writeJSON(w, http.StatusServiceUnavailable, errResp("workspace manager not configured"))
		return
	}
	accountID := parseInt64(r.PathValue("id"), 0)
	if accountID == 0 {
		writeJSON(w, http.StatusBadRequest, errResp("invalid account id"))
		return
	}
	name := fmt.Sprintf("account_%d", accountID)
	if s.appStore != nil {
		if accounts, err := s.appStore.GetFacebookAccounts(r.Context(), 0); err == nil {
			for _, acc := range accounts {
				if acc.ID == accountID {
					name = acc.Name
					break
				}
			}
		}
	}
	inst, err := s.wm.Start(accountID, name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "started",
		"account_id":   accountID,
		"vnc_port":     inst.VNCPort,
		"cdp_port":     inst.CDPPort,
		"container_id": inst.ContainerID,
	})
}

func (s *Server) handleStopWorkspace(w http.ResponseWriter, r *http.Request) {
	if s.wm == nil {
		writeJSON(w, http.StatusServiceUnavailable, errResp("workspace manager not configured"))
		return
	}
	accountID := parseInt64(r.PathValue("id"), 0)
	if accountID == 0 {
		writeJSON(w, http.StatusBadRequest, errResp("invalid account id"))
		return
	}
	s.wm.Stop(accountID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) handleMarkLoggedIn(w http.ResponseWriter, r *http.Request) {
	if s.appStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, errResp("app store not configured"))
		return
	}
	accountID := parseInt64(r.PathValue("id"), 0)
	if accountID == 0 {
		writeJSON(w, http.StatusBadRequest, errResp("invalid account id"))
		return
	}
	if err := s.appStore.SetFacebookAccountLoggedIn(r.Context(), accountID, true); err != nil {
		writeJSON(w, http.StatusInternalServerError, errResp(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleVNCProxy bridges WebSocket (browser) ↔ TCP (x11vnc inside Docker container).
func (s *Server) handleVNCProxy(w http.ResponseWriter, r *http.Request) {
	if s.wm == nil {
		http.Error(w, "workspace manager not configured", http.StatusServiceUnavailable)
		return
	}
	accountID := parseInt64(r.PathValue("id"), 0)
	if accountID == 0 {
		http.Error(w, "invalid account id", http.StatusBadRequest)
		return
	}
	inst := s.wm.Get(accountID)
	if inst == nil || inst.VNCPort == 0 {
		http.Error(w, "browser not running — start it first", http.StatusNotFound)
		return
	}

	vncAddr := fmt.Sprintf("127.0.0.1:%d", inst.VNCPort)
	tcp, err := net.DialTimeout("tcp", vncAddr, 8*time.Second)
	if err != nil {
		http.Error(w, "VNC not reachable: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer tcp.Close()

	ws, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[VNCProxy] upgrade failed: %v", err)
		return
	}
	defer ws.Close()

	errc := make(chan error, 2)

	go func() {
		buf := make([]byte, 65536)
		for {
			n, err := tcp.Read(buf)
			if n > 0 {
				if werr := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					errc <- werr
					return
				}
			}
			if err != nil {
				errc <- err
				return
			}
		}
	}()

	go func() {
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				errc <- err
				return
			}
			if _, werr := tcp.Write(data); werr != nil {
				errc <- werr
				return
			}
		}
	}()

	<-errc
	log.Printf("[VNCProxy] Tunnel closed for account %d", accountID)
}

// handleVNCViewer serves a self-contained noVNC HTML page for iframe embedding.
// The page connects directly to /ws/vnc/{id} on the same host (port 8080).
func (s *Server) handleVNCViewer(w http.ResponseWriter, r *http.Request) {
	accountID := r.PathValue("id")
	html := `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>Browser</title>
<style>
* { margin:0; padding:0; box-sizing:border-box; }
body { background:#0f172a; overflow:hidden; }
#screen { width:100vw; height:100vh; }
#status { position:fixed; bottom:6px; left:8px; color:#475569; font:11px monospace; pointer-events:none; }
</style>
</head>
<body>
<div id="screen"></div>
<div id="status">Đang kết nối...</div>
<script type="module">
import RFB from 'https://cdn.jsdelivr.net/npm/@novnc/novnc@1.5.0/core/rfb.js';
const id = '` + accountID + `';
const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
const wsUrl = proto + '//' + location.host + '/ws/vnc/' + id;
const st = document.getElementById('status');
try {
  const rfb = new RFB(document.getElementById('screen'), wsUrl, { wsProtocols: ['binary'] });
  rfb.scaleViewport = true;
  rfb.resizeSession = false;
  rfb.addEventListener('connect', () => { st.textContent = 'Đã kết nối · account ' + id; });
  rfb.addEventListener('disconnect', e => { st.textContent = 'Mất kết nối: ' + (e.detail?.reason ?? ''); });
  rfb.addEventListener('securityfailure', e => { st.textContent = 'Lỗi bảo mật: ' + e.detail?.reason; });
} catch(e) { st.textContent = 'Lỗi: ' + e.message; }
</script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, html)
}
