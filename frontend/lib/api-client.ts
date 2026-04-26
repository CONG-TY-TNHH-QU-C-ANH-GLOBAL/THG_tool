import type {
  DashboardStats,
  Job,
  Lead,
  ListJobsResponse,
  ListLeadsResponse,
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
} as const
