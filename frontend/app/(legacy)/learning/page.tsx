'use client'
import { useState, useEffect, useCallback } from 'react'
import { api } from '@/lib/api-client'
import type { LearningResponse, LearningWeights, OutcomeType } from '@/lib/types'

export default function LearningPage() {
  const [data, setData] = useState<LearningResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchLearning = useCallback(async () => {
    try {
      const res = await api.getLearning()
      setData(res)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load learning data')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchLearning() }, [fetchLearning])

  return (
    <div className="space-y-6 max-w-4xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Self-Learning Intelligence</h1>
          <p className="text-sm text-gray-500 mt-0.5">
            Adaptive scoring weights — updated by conversion signals in real time
          </p>
        </div>
        <button
          onClick={fetchLearning}
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

      {loading && <p className="text-sm text-gray-500">Loading…</p>}

      {!loading && data && (
        <>
          {/* Live weights */}
          <section className="bg-gray-900 border border-gray-800 rounded-xl p-5 space-y-4">
            <p className="text-xs font-semibold text-gray-500 uppercase tracking-widest">
              Current Scoring Weights
            </p>
            <WeightBars weights={data.live_weights} />
          </section>

          {/* Outcome counts */}
          <section>
            <p className="text-xs font-semibold text-gray-500 uppercase tracking-widest mb-3">
              Outcome Distribution
            </p>
            <div className="grid grid-cols-3 gap-4">
              <OutcomeStat
                label="Converted"
                value={data.outcome_counts.converted}
                color="text-emerald-400"
              />
              <OutcomeStat
                label="Rejected"
                value={data.outcome_counts.rejected}
                color="text-rose-400"
              />
              <OutcomeStat
                label="Ignored"
                value={data.outcome_counts.ignored}
                color="text-gray-400"
              />
            </div>
          </section>

          {/* Weight history */}
          {data.history.length > 0 && (
            <section className="bg-gray-900 border border-gray-800 rounded-xl p-5 space-y-3">
              <p className="text-xs font-semibold text-gray-500 uppercase tracking-widest">
                Weight History (last {data.history.length})
              </p>
              <div className="space-y-2 max-h-72 overflow-y-auto">
                {data.history.map((entry) => (
                  <div
                    key={entry.id}
                    className="flex items-center justify-between text-xs border border-gray-800 rounded-lg px-3 py-2"
                  >
                    <span className="text-gray-500 font-mono">
                      {new Date(entry.created_at).toLocaleString()}
                    </span>
                    <span className="text-gray-600 mx-3">→</span>
                    <WeightPills weights={entry.weights} />
                    <span className={`ml-3 text-xs font-medium ${outcomeColor(entry.trigger_outcome as OutcomeType)}`}>
                      {entry.trigger_outcome}
                    </span>
                  </div>
                ))}
              </div>
            </section>
          )}

          {data.last_updated && (
            <p className="text-xs text-gray-600">
              Last weight update: {new Date(data.last_updated).toLocaleString()}
            </p>
          )}
        </>
      )}
    </div>
  )
}

function WeightBars({ weights }: { weights: LearningWeights }) {
  const bars: Array<{ label: string; key: keyof LearningWeights; color: string }> = [
    { label: 'Keyword Relevance', key: 'keyword_relevance', color: 'bg-indigo-500' },
    { label: 'Engagement',        key: 'engagement',        color: 'bg-amber-500'  },
    { label: 'Content Quality',   key: 'content_quality',   color: 'bg-emerald-500' },
  ]
  return (
    <div className="space-y-3">
      {bars.map(({ label, key, color }) => {
        const pct = Math.round(weights[key] * 100)
        return (
          <div key={key} className="space-y-1">
            <div className="flex justify-between text-xs">
              <span className="text-gray-400">{label}</span>
              <span className="text-gray-300 tabular-nums font-medium">{pct}%</span>
            </div>
            <div className="w-full bg-gray-800 rounded-full h-2">
              <div
                className={`h-2 rounded-full transition-all duration-500 ${color}`}
                style={{ width: `${pct}%` }}
              />
            </div>
          </div>
        )
      })}
    </div>
  )
}

function WeightPills({ weights }: { weights: LearningWeights }) {
  return (
    <div className="flex gap-1.5">
      <Pill label="KR" value={weights.keyword_relevance} />
      <Pill label="EN" value={weights.engagement} />
      <Pill label="CQ" value={weights.content_quality} />
    </div>
  )
}

function Pill({ label, value }: { label: string; value: number }) {
  return (
    <span className="px-1.5 py-0.5 bg-gray-800 rounded font-mono text-gray-400">
      {label} {Math.round(value * 100)}%
    </span>
  )
}

function OutcomeStat({ label, value, color }: { label: string; value: number; color: string }) {
  return (
    <div className="bg-gray-900 border border-gray-800 rounded-xl px-4 py-4 text-center">
      <p className={`text-2xl font-bold tabular-nums ${color}`}>{value}</p>
      <p className="text-xs text-gray-500 mt-1">{label}</p>
    </div>
  )
}

function outcomeColor(outcome: OutcomeType): string {
  switch (outcome) {
    case 'converted': return 'text-emerald-400'
    case 'rejected':  return 'text-rose-400'
    default:          return 'text-gray-500'
  }
}
