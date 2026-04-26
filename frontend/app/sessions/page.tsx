'use client'
import { useState, useEffect, useCallback } from 'react'
import { api } from '@/lib/api-client'
import type { BrowserSession, SessionStatus } from '@/lib/types'

const STATUS_COLOR: Record<SessionStatus, string> = {
  active:     'bg-emerald-500/20 text-emerald-400 border-emerald-800',
  idle:       'bg-gray-700/40 text-gray-400 border-gray-700',
  error:      'bg-rose-500/20 text-rose-400 border-rose-800',
  terminated: 'bg-gray-800 text-gray-600 border-gray-800',
}

export default function SessionsPage() {
  const [sessions, setSessions] = useState<BrowserSession[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchSessions = useCallback(async () => {
    try {
      const res = await api.listSessions()
      setSessions(res.sessions ?? [])
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load sessions')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchSessions() }, [fetchSessions])

  return (
    <div className="space-y-5 max-w-5xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Browser Sessions</h1>
          <p className="text-sm text-gray-500 mt-0.5">{sessions.length} active Chrome workspaces</p>
        </div>
        <button
          onClick={fetchSessions}
          className="px-3 py-1.5 text-sm bg-gray-800 hover:bg-gray-700 rounded-lg text-gray-300 transition-colors"
        >
          ↻ Refresh
        </button>
      </div>

      {error && (
        <div className="bg-rose-900/30 border border-rose-800 rounded-lg px-4 py-3 text-sm text-rose-300">
          {error}
        </div>
      )}

      <div className="bg-gray-900 border border-gray-800 rounded-xl overflow-hidden">
        {loading ? (
          <p className="px-6 py-8 text-sm text-gray-500">Loading…</p>
        ) : sessions.length === 0 ? (
          <p className="px-6 py-8 text-sm text-gray-500">
            No sessions yet. Start a Chrome workspace from the Browser page.
          </p>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-800 text-xs text-gray-500 uppercase tracking-wider">
                <th className="px-4 py-3 text-left">Account</th>
                <th className="px-4 py-3 text-left">Status</th>
                <th className="px-4 py-3 text-left">CDP</th>
                <th className="px-4 py-3 text-left">VNC</th>
                <th className="px-4 py-3 text-left">Started</th>
                <th className="px-4 py-3 text-left">Last Active</th>
                <th className="px-4 py-3 text-left">Error</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-800">
              {sessions.map((s) => (
                <tr key={s.id} className="hover:bg-gray-800/50 transition-colors">
                  <td className="px-4 py-3 font-mono text-gray-300 text-xs">#{s.account_id}</td>
                  <td className="px-4 py-3">
                    <span className={`px-2 py-0.5 rounded-full text-xs font-medium border ${STATUS_COLOR[s.status]}`}>
                      {s.status}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-gray-400 text-xs font-mono">
                    {s.cdp_port > 0 ? `:${s.cdp_port}` : '—'}
                  </td>
                  <td className="px-4 py-3 text-gray-400 text-xs font-mono">
                    {s.vnc_port > 0 ? `:${s.vnc_port}` : '—'}
                  </td>
                  <td className="px-4 py-3 text-gray-500 text-xs whitespace-nowrap">
                    {new Date(s.started_at).toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-gray-500 text-xs whitespace-nowrap">
                    {new Date(s.last_active_at).toLocaleString()}
                  </td>
                  <td className="px-4 py-3 text-rose-400 text-xs max-w-xs truncate">
                    {s.error_msg || '—'}
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
