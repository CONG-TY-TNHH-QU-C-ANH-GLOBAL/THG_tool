'use client'
import { useState, useEffect, useCallback } from 'react'
import Link from 'next/link'
import { api } from '@/lib/api-client'
import { useSSE } from '@/hooks/useSSE'
import { JobStatusBadge } from '@/components/StatusBadge'
import { ProgressBar } from '@/components/ProgressBar'
import type { Job, JobStatus, SSEEvent } from '@/lib/types'

const STATUS_FILTERS: Array<{ label: string; value: string }> = [
  { label: 'All', value: '' },
  { label: 'Pending', value: 'pending' },
  { label: 'Running', value: 'running' },
  { label: 'Completed', value: 'completed' },
  { label: 'Failed', value: 'failed' },
]

export default function JobsPage() {
  const [jobs, setJobs] = useState<Job[]>([])
  const [statusFilter, setStatusFilter] = useState('')
  const [loading, setLoading] = useState(true)

  const fetchJobs = useCallback(async () => {
    try {
      const res = await api.listJobs({ status: statusFilter || undefined, limit: 100 })
      setJobs(res.jobs ?? [])
    } finally {
      setLoading(false)
    }
  }, [statusFilter])

  useEffect(() => { fetchJobs() }, [fetchJobs])

  useSSE(
    useCallback(
      (event: SSEEvent) => {
        if (!event.job_id) return
        // Patch job in-place if we have it; otherwise refresh list
        setJobs((prev) => {
          const idx = prev.findIndex((j) => j.id === event.job_id)
          if (idx === -1) {
            fetchJobs()
            return prev
          }
          const updated = [...prev]
          updated[idx] = {
            ...updated[idx],
            status: (event.status as JobStatus) ?? updated[idx].status,
            progress: event.progress ?? updated[idx].progress,
          }
          return updated
        })
      },
      [fetchJobs]
    )
  )

  return (
    <div className="space-y-5 max-w-5xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Jobs</h1>
          <p className="text-sm text-gray-500 mt-0.5">{jobs.length} jobs · live progress via SSE</p>
        </div>
        <button
          onClick={fetchJobs}
          className="px-3 py-1.5 text-sm bg-gray-800 hover:bg-gray-700 rounded-lg text-gray-300 transition-colors"
        >
          ↻ Refresh
        </button>
      </div>

      {/* Status filter */}
      <div className="flex gap-2">
        {STATUS_FILTERS.map(({ label, value }) => (
          <button
            key={value}
            onClick={() => setStatusFilter(value)}
            className={`px-3 py-1 text-xs rounded-full font-medium transition-colors ${
              statusFilter === value
                ? 'bg-indigo-600 text-white'
                : 'bg-gray-800 text-gray-400 hover:text-white'
            }`}
          >
            {label}
          </button>
        ))}
      </div>

      {/* Table */}
      <div className="bg-gray-900 border border-gray-800 rounded-xl overflow-hidden">
        {loading ? (
          <p className="px-6 py-8 text-sm text-gray-500">Loading…</p>
        ) : jobs.length === 0 ? (
          <p className="px-6 py-8 text-sm text-gray-500">No jobs found. Submit a task from the dashboard.</p>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-800 text-xs text-gray-500 uppercase tracking-wider">
                <th className="px-4 py-3 text-left">ID</th>
                <th className="px-4 py-3 text-left">Intent</th>
                <th className="px-4 py-3 text-left">Status</th>
                <th className="px-4 py-3 text-left w-40">Progress</th>
                <th className="px-4 py-3 text-left">Created</th>
                <th className="px-4 py-3 text-left"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-800">
              {jobs.map((job) => (
                <tr key={job.id} className="hover:bg-gray-800/50 transition-colors">
                  <td className="px-4 py-3 font-mono text-gray-400 text-xs">{job.id}</td>
                  <td className="px-4 py-3 text-gray-200 font-medium">{job.intent}</td>
                  <td className="px-4 py-3">
                    <JobStatusBadge status={job.status} />
                  </td>
                  <td className="px-4 py-3">
                    <ProgressBar value={job.progress} label size="sm" />
                  </td>
                  <td className="px-4 py-3 text-gray-500 text-xs">
                    {new Date(job.created_at).toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <Link
                      href={`/jobs/${job.task_id}`}
                      className="text-indigo-400 hover:text-indigo-300 text-xs"
                    >
                      Details →
                    </Link>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
