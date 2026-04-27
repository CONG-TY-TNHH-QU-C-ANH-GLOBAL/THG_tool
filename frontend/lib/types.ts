// ── Backend contract types ─────────────────────────────────────────────────────
// These mirror the Go structs exactly. Frontend MUST NOT compute scoring,
// filtering, or deduplication — backend is the single source of truth.

export type JobStatus = 'pending' | 'running' | 'completed' | 'failed'
export type LeadCategory = 'hot' | 'warm' | 'cold'

/** Mirrors internal/jobs.Job */
export interface Job {
  id: number
  task_id: string
  intent: string
  status: JobStatus
  progress: number        // 0–100
  attempt: number
  max_attempts: number
  error?: string
  result?: string         // JSON OutputDataset when completed
  created_at: string
  updated_at: string
}

/** Mirrors internal/store.TaskLead */
export interface Lead {
  id: number
  task_id: string
  org_id: number
  source_url: string
  author_profile_url: string
  author_name: string
  content: string
  lead_score: number      // 0–100
  category: LeadCategory
  signals: string[]
  created_at: string
}

/** Mirrors internal/api.DashboardStats */
export interface DashboardStats {
  total_jobs: number
  pending_jobs: number
  running_jobs: number
  completed_jobs: number
  failed_jobs: number
  total_leads: number
  hot_leads: number
  warm_leads: number
  cold_leads: number
  success_rate: number    // 0–100
}

// ── SSE event contract ────────────────────────────────────────────────────────
// All events follow: { type, payload? } plus optional flat fields.

export type SSEEventType =
  | 'job.created'
  | 'job.running'
  | 'job.progress'
  | 'job.completed'
  | 'job.failed'
  | 'lead.inserted'

export interface SSEEvent {
  type: SSEEventType
  job_id?: number
  task_id?: string
  status?: JobStatus
  progress?: number
  message?: string
  payload?: Lead | Record<string, unknown>
}

// ── Browser intelligence types ────────────────────────────────────────────────

export type SessionStatus = 'active' | 'idle' | 'error' | 'terminated'
export type SessionState = 'clean' | 'warned' | 'restricted' | 'banned'

/** Mirrors internal/store.BrowserSession */
export interface BrowserSession {
  id: number
  account_id: number
  org_id: number
  status: SessionStatus
  cdp_port: number
  vnc_port: number
  started_at: string
  last_active_at: string
  error_msg: string
}

/** Mirrors internal/store.BrowserIdentity */
export interface BrowserIdentity {
  id: number
  account_id: number
  org_id: number
  user_agent: string
  screen_w: number
  screen_h: number
  timezone: string
  languages: string
  webgl_vendor: string
  webgl_renderer: string
  session_state: SessionState
  updated_at: string
}

// ── Self-learning types ───────────────────────────────────────────────────────

export interface LearningWeights {
  keyword_relevance: number
  engagement: number
  content_quality: number
}

/** Mirrors internal/store.LearningProfile */
export interface LearningProfile {
  id: number
  org_id: number
  keyword_relevance: number
  engagement: number
  content_quality: number
  converted_count: number
  rejected_count: number
  ignored_count: number
  updated_at: string
}

/** Mirrors internal/store.LearningHistoryEntry */
export interface LearningHistoryEntry {
  id: number
  org_id: number
  weights: LearningWeights
  trigger_outcome: string
  created_at: string
}

export interface LearningResponse {
  profile: LearningProfile | null
  history: LearningHistoryEntry[]
  live_weights: LearningWeights
  outcome_counts: { converted: number; rejected: number; ignored: number }
  last_updated: string
}

export type OutcomeType = 'converted' | 'rejected' | 'ignored'

// ── Browser workspace types ───────────────────────────────────────────────────

/** Mirrors api.workspaceItem — one Facebook account + its container state */
export interface Workspace {
  id: number
  name: string
  status: string
  running: boolean
  cdp_port?: number
  vnc_port?: number
  container_id?: string
  browser_logged_in: boolean
}

export interface ListWorkspacesResponse {
  workspaces: Workspace[]
  count: number
}

// ── API response envelopes ────────────────────────────────────────────────────

export interface ListJobsResponse {
  jobs: Job[]
  count: number
}

export interface ListLeadsResponse {
  leads: Lead[]
  count: number
}

export interface ListSessionsResponse {
  sessions: BrowserSession[]
  count: number
}

export interface ListIdentitiesResponse {
  identities: BrowserIdentity[]
  count: number
}

export interface SubmitTaskResponse {
  task_id: string
  job_id: number
  status: JobStatus
  intent: string
}
