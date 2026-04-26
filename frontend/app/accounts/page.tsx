'use client'
import { useState, useEffect, useCallback } from 'react'
import { api } from '@/lib/api-client'
import type { BrowserIdentity, SessionState } from '@/lib/types'

const STATE_COLOR: Record<SessionState, string> = {
  clean:      'bg-emerald-500/20 text-emerald-400 border-emerald-800',
  warned:     'bg-amber-500/20 text-amber-400 border-amber-800',
  restricted: 'bg-orange-500/20 text-orange-400 border-orange-800',
  banned:     'bg-rose-500/20 text-rose-400 border-rose-800',
}

export default function AccountsPage() {
  const [identities, setIdentities] = useState<BrowserIdentity[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchIdentities = useCallback(async () => {
    try {
      const res = await api.listIdentities()
      setIdentities(res.identities ?? [])
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load account identities')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchIdentities() }, [fetchIdentities])

  return (
    <div className="space-y-5 max-w-5xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Account Identities</h1>
          <p className="text-sm text-gray-500 mt-0.5">
            {identities.length} browser fingerprints · session health per account
          </p>
        </div>
        <button
          onClick={fetchIdentities}
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

      {loading ? (
        <p className="text-sm text-gray-500">Loading…</p>
      ) : identities.length === 0 ? (
        <p className="text-sm text-gray-500">
          No account identities registered yet. Identities are seeded automatically when a workspace starts.
        </p>
      ) : (
        <div className="grid gap-4">
          {identities.map((id) => (
            <IdentityCard key={id.id} identity={id} />
          ))}
        </div>
      )}
    </div>
  )
}

function IdentityCard({ identity: bi }: { identity: BrowserIdentity }) {
  return (
    <div className="bg-gray-900 border border-gray-800 rounded-xl p-5 space-y-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="w-8 h-8 rounded-full bg-indigo-700 flex items-center justify-center text-xs font-bold text-white">
            {bi.account_id}
          </div>
          <div>
            <p className="text-sm font-medium text-white">Account #{bi.account_id}</p>
            <p className="text-xs text-gray-500">{bi.timezone} · {bi.screen_w}×{bi.screen_h}</p>
          </div>
        </div>
        <span className={`px-2 py-0.5 rounded-full text-xs font-medium border ${STATE_COLOR[bi.session_state]}`}>
          {bi.session_state}
        </span>
      </div>

      <div className="grid grid-cols-2 gap-2 text-xs">
        <Detail label="Languages" value={bi.languages} />
        <Detail label="Updated" value={new Date(bi.updated_at).toLocaleString()} />
      </div>

      <div className="text-xs text-gray-600 font-mono truncate">{bi.user_agent}</div>

      {(bi.webgl_vendor || bi.webgl_renderer) && (
        <div className="text-xs text-gray-600 font-mono truncate">
          {bi.webgl_vendor} · {bi.webgl_renderer}
        </div>
      )}
    </div>
  )
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <span className="text-gray-600">{label}: </span>
      <span className="text-gray-400">{value}</span>
    </div>
  )
}
