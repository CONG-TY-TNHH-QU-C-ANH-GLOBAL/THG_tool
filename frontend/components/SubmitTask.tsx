'use client'
import { useState } from 'react'
import { api } from '@/lib/api-client'
import type { SubmitTaskResponse } from '@/lib/types'

interface Props {
  onSubmit?: (res: SubmitTaskResponse) => void
}

export function SubmitTask({ onSubmit }: Readonly<Props>) {
  const [text, setText] = useState('')
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<SubmitTaskResponse | null>(null)
  const [error, setError] = useState<string | null>(null)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!text.trim()) return
    setLoading(true)
    setError(null)
    setResult(null)
    try {
      const res = await api.submitTask(text.trim())
      setResult(res)
      setText('')
      onSubmit?.(res)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Submission failed')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="bg-gray-900 border border-gray-800 rounded-xl p-5 space-y-4">
      <form onSubmit={handleSubmit} className="flex gap-3">
        <input
          className="flex-1 bg-gray-800 border border-gray-700 rounded-lg px-4 py-2.5 text-sm text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-indigo-500"
          placeholder='e.g. "cào nhóm ship hàng mỹ https://facebook.com/groups/123"'
          value={text}
          onChange={(e) => setText(e.target.value)}
          disabled={loading}
        />
        <button
          type="submit"
          disabled={loading || !text.trim()}
          className="px-5 py-2.5 bg-indigo-600 hover:bg-indigo-500 disabled:bg-gray-700 disabled:cursor-not-allowed text-white text-sm font-medium rounded-lg transition-colors"
        >
          {loading ? 'Submitting…' : 'Submit'}
        </button>
      </form>

      {error && (
        <p className="text-sm text-rose-400">⚠ {error}</p>
      )}

      {result && (
        <div className="text-sm text-emerald-400 font-mono bg-gray-800 rounded-lg px-4 py-3">
          ✓ Task submitted · intent: <strong>{result.intent}</strong> · task_id: {result.task_id}
        </div>
      )}
    </div>
  )
}
