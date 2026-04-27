'use client'
import { useState, useEffect, useCallback } from 'react'
import { api } from '@/lib/api-client'
import { useSSE } from '@/hooks/useSSE'
import { KPICard } from '@/components/KPICard'
import { SubmitTask } from '@/components/SubmitTask'
import type { DashboardStats, SSEEvent } from '@/lib/types'

export default function DashboardPage() {
  const [stats, setStats] = useState<DashboardStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [liveEvents, setLiveEvents] = useState<string[]>([])

  const fetchStats = useCallback(async () => {
    try {
      const s = await api.getDashboardStats()
      setStats(s)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load stats')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchStats() }, [fetchStats])

  useSSE(
    useCallback(
      (event: SSEEvent) => {
        // Refresh stats on meaningful state changes
        if (['job.completed', 'job.failed', 'lead.inserted'].includes(event.type)) {
          fetchStats()
        }
        // Append to live event feed (cap at 20)
        setLiveEvents((prev) => [
          `${new Date().toLocaleTimeString()} · ${event.type}${event.task_id ? ` · ${event.task_id.slice(0, 8)}` : ''}`,
          ...prev.slice(0, 19),
        ])
      },
      [fetchStats]
    )
  )

  return (
    <div className="space-y-6 max-w-5xl">
      <div>
        <h1 className="text-2xl font-bold text-white">Dashboard</h1>
        <p className="text-sm text-gray-500 mt-0.5">Lead intelligence overview — live via SSE</p>
      </div>

      {error && (
        <div className="bg-rose-900/30 border border-rose-800 rounded-lg px-4 py-3 text-sm text-rose-300">
          {error} — Is the backend running on port 8080?
        </div>
      )}

      {/* Job KPIs */}
      <section>
        <p className="text-xs font-semibold text-gray-500 uppercase tracking-widest mb-3">Jobs</p>
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
          <KPICard label="Total" value={loading ? '—' : stats!.total_jobs} />
          <KPICard label="Running" value={loading ? '—' : stats!.running_jobs} color="yellow" />
          <KPICard label="Completed" value={loading ? '—' : stats!.completed_jobs} color="green" />
          <KPICard label="Failed" value={loading ? '—' : stats!.failed_jobs} color="red" />
        </div>
      </section>

      {/* Lead KPIs */}
      <section>
        <p className="text-xs font-semibold text-gray-500 uppercase tracking-widest mb-3">Leads</p>
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
          <KPICard label="Total Leads" value={loading ? '—' : stats!.total_leads} />
          <KPICard label="Hot 🔥" value={loading ? '—' : stats!.hot_leads} color="red" />
          <KPICard label="Warm ☀️" value={loading ? '—' : stats!.warm_leads} color="yellow" />
          <KPICard
            label="Success Rate"
            value={loading ? '—' : `${stats!.success_rate.toFixed(1)}%`}
            color="green"
          />
        </div>
      </section>

      {/* Submit task */}
      <section>
        <p className="text-xs font-semibold text-gray-500 uppercase tracking-widest mb-3">
          Submit New Task
        </p>
        <SubmitTask onSubmit={() => setTimeout(fetchStats, 600)} />
      </section>

      {/* Live event feed */}
      <section>
        <p className="text-xs font-semibold text-gray-500 uppercase tracking-widest mb-3">
          Live Event Feed
        </p>
        <div className="bg-gray-900 border border-gray-800 rounded-xl p-4 h-44 overflow-y-auto font-mono text-xs space-y-1">
          {liveEvents.length === 0 ? (
            <p className="text-gray-600">Waiting for events…</p>
          ) : (
            liveEvents.map((e, i) => (
              <p key={i} className="text-gray-400">{e}</p>
            ))
          )}
        </div>
      </section>
    </div>
  )
}
