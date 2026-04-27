'use client'
import { useEffect, useState, useCallback } from 'react'
import { api } from '@/lib/api-client'
import type { Workspace } from '@/lib/types'

function statusBadge(ws: Workspace) {
  if (ws.running) return <span className="px-2 py-0.5 rounded-full text-xs font-medium bg-green-900 text-green-300">Running</span>
  if (ws.status === 'suspended') return <span className="px-2 py-0.5 rounded-full text-xs font-medium bg-red-900 text-red-300">Suspended</span>
  return <span className="px-2 py-0.5 rounded-full text-xs font-medium bg-gray-700 text-gray-400">Offline</span>
}

export default function BrowserPage() {
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [selected, setSelected] = useState<number | null>(null)
  const [busy, setBusy] = useState<Record<number, boolean>>({})
  const [error, setError] = useState<string | null>(null)
  const [frameKey, setFrameKey] = useState(0)

  const load = useCallback(async () => {
    try {
      const res = await api.listWorkspaces()
      setWorkspaces(res.workspaces ?? [])
    } catch (e) {
      setError(String(e))
    }
  }, [])

  useEffect(() => {
    load()
    const t = setInterval(load, 5000)
    return () => clearInterval(t)
  }, [load])

  const selectedWs = workspaces.find(w => w.id === selected)

  async function handleStart(id: number) {
    setBusy(b => ({ ...b, [id]: true }))
    try {
      await api.startWorkspace(id)
      await load()
      setSelected(id)
      setFrameKey(k => k + 1)
    } catch (e) {
      setError(String(e))
    } finally {
      setBusy(b => ({ ...b, [id]: false }))
    }
  }

  async function handleStop(id: number) {
    setBusy(b => ({ ...b, [id]: true }))
    try {
      await api.stopWorkspace(id)
      if (selected === id) setSelected(null)
      await load()
    } catch (e) {
      setError(String(e))
    } finally {
      setBusy(b => ({ ...b, [id]: false }))
    }
  }

  async function handleMarkLoggedIn(id: number) {
    setBusy(b => ({ ...b, [id]: true }))
    try {
      await api.markLoggedIn(id)
      await load()
    } catch (e) {
      setError(String(e))
    } finally {
      setBusy(b => ({ ...b, [id]: false }))
    }
  }

  return (
    <div className="flex h-full overflow-hidden">
      {/* ── Left panel: account list ── */}
      <div className="w-72 shrink-0 border-r border-gray-800 flex flex-col bg-gray-900">
        <div className="px-4 py-4 border-b border-gray-800 flex items-center justify-between">
          <h2 className="text-sm font-semibold text-white">Facebook Accounts</h2>
          <button
            onClick={load}
            className="text-gray-500 hover:text-gray-300 text-xs transition-colors"
            title="Refresh"
          >
            ↻
          </button>
        </div>

        {error && (
          <div className="mx-3 mt-3 px-3 py-2 rounded bg-red-950 text-red-400 text-xs">
            {error}
            <button className="ml-2 underline" onClick={() => setError(null)}>dismiss</button>
          </div>
        )}

        {workspaces.length === 0 && !error && (
          <div className="px-4 py-8 text-center text-gray-500 text-sm">
            No Facebook accounts found.<br />
            <span className="text-xs text-gray-600">Add accounts via Telegram or the API.</span>
          </div>
        )}

        <ul className="flex-1 overflow-y-auto divide-y divide-gray-800">
          {workspaces.map(ws => (
            <li
              key={ws.id}
              className={`px-4 py-3 cursor-pointer transition-colors ${
                selected === ws.id ? 'bg-gray-800' : 'hover:bg-gray-800/50'
              }`}
              onClick={() => setSelected(ws.id)}
            >
              <div className="flex items-center justify-between mb-1.5">
                <span className="text-sm font-medium text-white truncate mr-2">{ws.name}</span>
                {statusBadge(ws)}
              </div>

              {ws.running && (
                <p className="text-xs text-gray-500 mb-2">
                  cdp:{ws.cdp_port} · vnc:{ws.vnc_port}
                </p>
              )}

              {ws.browser_logged_in && (
                <p className="text-xs text-green-500 mb-2">✓ Đã đăng nhập Facebook</p>
              )}

              <div className="flex gap-1.5 flex-wrap">
                {!ws.running ? (
                  <button
                    disabled={busy[ws.id]}
                    onClick={e => { e.stopPropagation(); handleStart(ws.id) }}
                    className="px-2 py-1 rounded text-xs font-medium bg-indigo-700 hover:bg-indigo-600 text-white disabled:opacity-50 transition-colors"
                  >
                    {busy[ws.id] ? 'Starting…' : 'Start'}
                  </button>
                ) : (
                  <>
                    <button
                      disabled={busy[ws.id]}
                      onClick={e => { e.stopPropagation(); setSelected(ws.id); setFrameKey(k => k + 1) }}
                      className="px-2 py-1 rounded text-xs font-medium bg-gray-700 hover:bg-gray-600 text-white transition-colors"
                    >
                      View
                    </button>
                    <button
                      disabled={busy[ws.id]}
                      onClick={e => { e.stopPropagation(); handleStop(ws.id) }}
                      className="px-2 py-1 rounded text-xs font-medium bg-red-800 hover:bg-red-700 text-white disabled:opacity-50 transition-colors"
                    >
                      {busy[ws.id] ? 'Stopping…' : 'Stop'}
                    </button>
                    {!ws.browser_logged_in && (
                      <button
                        disabled={busy[ws.id]}
                        onClick={e => { e.stopPropagation(); handleMarkLoggedIn(ws.id) }}
                        className="px-2 py-1 rounded text-xs font-medium bg-green-800 hover:bg-green-700 text-white disabled:opacity-50 transition-colors"
                      >
                        Đã đăng nhập
                      </button>
                    )}
                  </>
                )}
              </div>
            </li>
          ))}
        </ul>
      </div>

      {/* ── Right panel: VNC viewer ── */}
      <div className="flex-1 min-w-0 bg-gray-950 flex flex-col">
        {selectedWs?.running ? (
          <iframe
            key={`${selectedWs.id}-${frameKey}`}
            src={`/vnc-viewer/${selectedWs.id}`}
            className="flex-1 w-full border-0"
            title={`Browser — ${selectedWs.name}`}
            allow="clipboard-read; clipboard-write"
          />
        ) : (
          <div className="flex-1 flex flex-col items-center justify-center text-center text-gray-600 select-none">
            <div className="text-5xl mb-4">🖥️</div>
            {selected && !selectedWs?.running ? (
              <>
                <p className="text-base font-medium text-gray-400 mb-1">Container chưa chạy</p>
                <p className="text-sm mb-4">Nhấn <strong>Start</strong> để khởi động Chrome.</p>
                <button
                  disabled={busy[selected]}
                  onClick={() => handleStart(selected)}
                  className="px-4 py-2 rounded bg-indigo-700 hover:bg-indigo-600 text-white text-sm font-medium disabled:opacity-50 transition-colors"
                >
                  {busy[selected] ? 'Starting…' : 'Start Browser'}
                </button>
              </>
            ) : (
              <>
                <p className="text-base font-medium text-gray-500 mb-1">Chọn một tài khoản</p>
                <p className="text-sm">để xem Chrome đang chạy của tài khoản đó.</p>
              </>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
