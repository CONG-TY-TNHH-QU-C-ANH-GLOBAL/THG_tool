'use client'
import { useState, useEffect, useCallback } from 'react'
import { useParams } from 'next/navigation'
import Link from 'next/link'
import { api } from '@/lib/api-client'
import { useSSE } from '@/hooks/useSSE'
import { JobStatusBadge } from '@/components/StatusBadge'
import { ProgressBar } from '@/components/ProgressBar'
import type { Job, JobStatus, SSEEvent } from '@/lib/types'

export default function JobDetailPage() {
  const { id: taskId } = useParams<{ id: string }>()
  const [job, setJob] = useState<Job | null>(null)
  const [loading, setLoading] = useState(true)
  const [notFound, setNotFound] = useState(false)

  const fetchJob = useCallback(async () => {
    try {
      const j = await api.getTask(taskId)
      setJob(j)
    } catch {
      setNotFound(true)
    } finally {
      setLoading(false)
    }
  }, [taskId])

  useEffect(() => { fetchJob() }, [fetchJob])

  useSSE(
    useCallback(
      (event: SSEEvent) => {
        if (event.task_id !== taskId) return

        if (event.type === 'job.completed' || event.type === 'job.failed') {
          // Reload full job to get result / error fields
          fetchJob()
          return
        }

        setJob((prev) =>
          prev
            ? {
                ...prev,
                status: (event.status as JobStatus) ?? prev.status,
                progress: event.progress ?? prev.progress,
              }
            : prev
        )
      },
      [taskId, fetchJob]
    )
  )

  if (loading) {
    return <p className="text-gray-500 text-sm">Loading…</p>
  }

  if (notFound || !job) {
    return (
      <div className="space-y-4">
        <p className="text-rose-400 text-sm">Job not found: {taskId}</p>
        <Link href="/jobs" className="text-indigo-400 hover:underline text-sm">← Back to Jobs</Link>
      </div>
    )
  }

  let parsedResult: unknown = null
  if (job.result) {
    try { parsedResult = JSON.parse(job.result) } catch { /* raw string */ }
  }

  return (
    <div className="space-y-6 max-w-3xl">
      {/* Breadcrumb */}
      <div className="flex items-center gap-2 text-sm text-gray-500">
        <Link href="/jobs" className="hover:text-gray-300 transition-colors">Jobs</Link>
        <span>/</span>
        <span className="text-gray-300 font-mono">{taskId}</span>
      </div>

      {/* Header */}
      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">{job.intent}</h1>
          <p className="text-sm text-gray-500 mt-1 font-mono">task_id: {job.task_id}</p>
        </div>
        <JobStatusBadge status={job.status} />
      </div>

      {/* Progress */}
      <div className="bg-gray-900 border border-gray-800 rounded-xl p-5 space-y-3">
        <div className="flex items-center justify-between">
          <p className="text-sm font-medium text-gray-300">Execution progress</p>
          <span className="text-sm tabular-nums text-gray-400">{job.progress}%</span>
        </div>
        <ProgressBar value={job.progress} size="md" />
        <div className="grid grid-cols-2 gap-4 pt-2 text-xs text-gray-500">
          <div>
            <span className="text-gray-600">Attempt</span>
            <span className="ml-2 text-gray-400">{job.attempt} / {job.max_attempts}</span>
          </div>
          <div>
            <span className="text-gray-600">Created</span>
            <span className="ml-2 text-gray-400">{new Date(job.created_at).toLocaleString()}</span>
          </div>
          <div>
            <span className="text-gray-600">Updated</span>
            <span className="ml-2 text-gray-400">{new Date(job.updated_at).toLocaleString()}</span>
          </div>
        </div>
      </div>

      {/* Error */}
      {job.status === 'failed' && job.error && (
        <div className="bg-rose-900/20 border border-rose-800 rounded-xl p-5">
          <p className="text-xs font-semibold text-rose-400 uppercase tracking-widest mb-2">Error</p>
          <p className="text-sm text-rose-300 font-mono whitespace-pre-wrap">{job.error}</p>
        </div>
      )}

      {/* Result */}
      {job.status === 'completed' && Boolean(parsedResult) && (
        <div className="bg-gray-900 border border-gray-800 rounded-xl p-5 space-y-3">
          <p className="text-xs font-semibold text-emerald-400 uppercase tracking-widest">Result</p>
          <ResultSummary data={parsedResult as ResultData} />
          <details className="group">
            <summary className="text-xs text-gray-500 cursor-pointer hover:text-gray-300 select-none">
              Raw JSON
            </summary>
            <pre className="mt-3 text-xs text-gray-400 bg-gray-950 rounded-lg p-4 overflow-x-auto max-h-80">
              {JSON.stringify(parsedResult, null, 2)}
            </pre>
          </details>
        </div>
      )}

      {/* Live indicator */}
      {job.status === 'running' && (
        <div className="flex items-center gap-2 text-xs text-indigo-400">
          <span className="w-2 h-2 rounded-full bg-indigo-400 animate-ping" />
          Receiving live updates via SSE…
        </div>
      )}
    </div>
  )
}

interface ResultData {
  stats?: { total_fetched?: number; total_returned?: number }
  records?: unknown[]
  insights?: unknown[]
}

function ResultSummary({ data }: { data: ResultData }) {
  const stats = data?.stats
  const records = data?.records ?? []
  return (
    <div className="grid grid-cols-3 gap-4">
      <Stat label="Fetched" value={stats?.total_fetched ?? 0} />
      <Stat label="Returned" value={stats?.total_returned ?? records.length} />
      <Stat label="Insights" value={data?.insights?.length ?? 0} />
    </div>
  )
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="bg-gray-800 rounded-lg px-4 py-3 text-center">
      <p className="text-xl font-bold text-white tabular-nums">{value}</p>
      <p className="text-xs text-gray-500 mt-0.5">{label}</p>
    </div>
  )
}
