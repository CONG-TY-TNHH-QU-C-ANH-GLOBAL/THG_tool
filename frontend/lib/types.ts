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

// ── API response envelopes ────────────────────────────────────────────────────

export interface ListJobsResponse {
  jobs: Job[]
  count: number
}

export interface ListLeadsResponse {
  leads: Lead[]
  count: number
}

export interface SubmitTaskResponse {
  task_id: string
  job_id: number
  status: JobStatus
  intent: string
}
