import type {
  DashboardStats,
  Job,
  Lead,
  LearningResponse,
  ListIdentitiesResponse,
  ListJobsResponse,
  ListLeadsResponse,
  ListSessionsResponse,
  ListWorkspacesResponse,
  OutcomeType,
  SubmitTaskResponse,
} from './types'

// All requests use relative paths — proxied by Next.js to the Go backend.
// See next.config.mjs for the rewrite rule.

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(body.error ?? `HTTP ${res.status}`)
  }
  return res.json() as Promise<T>
}

function qs(params: Record<string, string | number | undefined>): string {
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v != null && v !== '') p.set(k, String(v))
  }
  const s = p.toString()
  return s ? '?' + s : ''
}

export const api = {
  /** POST /api/v1/tasks — submit a natural-language task */
  submitTask(text: string, orgId = 0): Promise<SubmitTaskResponse> {
    return req('/api/v1/tasks', {
      method: 'POST',
      body: JSON.stringify({ text, org_id: orgId }),
    })
  },

  /** GET /api/v1/tasks/:taskId — poll a single job by task_id */
  getTask(taskId: string): Promise<Job> {
    return req(`/api/v1/tasks/${taskId}`)
  },

  /** GET /api/v1/jobs — list jobs with optional status filter */
  listJobs(params: { status?: string; limit?: number } = {}): Promise<ListJobsResponse> {
    return req(`/api/v1/jobs${qs(params)}`)
  },

  /** GET /api/v1/leads — list scored leads with filters */
  listLeads(
    params: {
      org_id?: number
      category?: string
      keyword?: string
      min_score?: number
      limit?: number
      offset?: number
    } = {}
  ): Promise<ListLeadsResponse> {
    return req(`/api/v1/leads${qs(params)}`)
  },

  /** GET /api/v1/dashboard/stats — aggregated KPI counts */
  getDashboardStats(orgId = 0): Promise<DashboardStats> {
    return req(`/api/v1/dashboard/stats${qs({ org_id: orgId })}`)
  },

  /** GET /api/v1/sessions — list browser sessions */
  listSessions(orgId = 0): Promise<ListSessionsResponse> {
    return req(`/api/v1/sessions${qs({ org_id: orgId })}`)
  },

  /** GET /api/v1/identities — list browser identities */
  listIdentities(orgId = 0): Promise<ListIdentitiesResponse> {
    return req(`/api/v1/identities${qs({ org_id: orgId })}`)
  },

  /** GET /api/v1/learning — get learning profile + weight history */
  getLearning(orgId = 0): Promise<LearningResponse> {
    return req(`/api/v1/learning${qs({ org_id: orgId })}`)
  },

  /** POST /api/v1/leads/:id/outcome — record conversion outcome signal */
  recordOutcome(leadId: number, outcome: OutcomeType, score: number, orgId = 0): Promise<void> {
    return req(`/api/v1/leads/${leadId}/outcome`, {
      method: 'POST',
      body: JSON.stringify({ org_id: orgId, outcome, score }),
    })
  },

  /** GET /api/v1/browser/workspaces — list Facebook accounts + container status */
  listWorkspaces(): Promise<ListWorkspacesResponse> {
    return req('/api/v1/browser/workspaces')
  },

  /** POST /api/v1/browser/workspaces/:id/start — launch Docker container */
  startWorkspace(id: number): Promise<void> {
    return req(`/api/v1/browser/workspaces/${id}/start`, { method: 'POST' })
  },

  /** POST /api/v1/browser/workspaces/:id/stop — kill Docker container */
  stopWorkspace(id: number): Promise<void> {
    return req(`/api/v1/browser/workspaces/${id}/stop`, { method: 'POST' })
  },

  /** POST /api/v1/browser/workspaces/:id/mark-logged-in — record Facebook login */
  markLoggedIn(id: number): Promise<void> {
    return req(`/api/v1/browser/workspaces/${id}/mark-logged-in`, { method: 'POST' })
  },
} as const
