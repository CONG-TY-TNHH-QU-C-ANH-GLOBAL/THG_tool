'use client'
import { useState, useEffect, useCallback, useRef } from 'react'
import { api } from '@/lib/api-client'
import { useSSE } from '@/hooks/useSSE'
import { LeadCategoryBadge } from '@/components/StatusBadge'
import type { Lead, LeadCategory, SSEEvent } from '@/lib/types'

const PAGE_SIZE = 50

const CATEGORY_FILTERS: Array<{ label: string; value: string }> = [
  { label: 'All', value: '' },
  { label: '🔥 Hot', value: 'hot' },
  { label: '☀️ Warm', value: 'warm' },
  { label: '❄️ Cold', value: 'cold' },
]

export default function LeadsPage() {
  const [leads, setLeads] = useState<Lead[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [newCount, setNewCount] = useState(0)

  // Filters
  const [category, setCategory] = useState<string>('')
  const [keyword, setKeyword] = useState('')
  const [minScore, setMinScore] = useState('')
  const [page, setPage] = useState(0)

  const keywordRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const fetchLeads = useCallback(
    async (reset = false) => {
      const offset = reset ? 0 : page * PAGE_SIZE
      try {
        const res = await api.listLeads({
          category: category || undefined,
          keyword: keyword || undefined,
          min_score: minScore ? Number(minScore) : undefined,
          limit: PAGE_SIZE,
          offset,
        })
        setLeads(res.leads ?? [])
        setTotal(res.count)
        if (reset) setPage(0)
      } finally {
        setLoading(false)
      }
    },
    [category, keyword, minScore, page]
  )

  useEffect(() => { fetchLeads(true) }, [category, minScore])
  useEffect(() => { fetchLeads() }, [page])

  // Debounce keyword
  const handleKeyword = (v: string) => {
    setKeyword(v)
    if (keywordRef.current) clearTimeout(keywordRef.current)
    keywordRef.current = setTimeout(() => fetchLeads(true), 400)
  }

  useSSE(
    useCallback(
      (event: SSEEvent) => {
        if (event.type !== 'lead.inserted') return
        // Don't inject into filtered views — just show a "new leads" counter
        setNewCount((n) => n + 1)
      },
      []
    )
  )

  const handleShowNew = () => {
    setNewCount(0)
    fetchLeads(true)
  }

  const totalPages = Math.ceil(total / PAGE_SIZE)

  return (
    <div className="space-y-5 max-w-6xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Leads</h1>
          <p className="text-sm text-gray-500 mt-0.5">{total} leads · scoring by backend only</p>
        </div>
        {newCount > 0 && (
          <button
            onClick={handleShowNew}
            className="px-4 py-2 bg-indigo-600/20 border border-indigo-500 rounded-lg text-indigo-300 text-sm font-medium hover:bg-indigo-600/30 transition-colors"
          >
            ↑ {newCount} new lead{newCount > 1 ? 's' : ''} — click to load
          </button>
        )}
      </div>

      {/* Filters */}
      <div className="flex flex-wrap gap-3 items-center">
        {/* Category */}
        <div className="flex gap-1.5">
          {CATEGORY_FILTERS.map(({ label, value }) => (
            <button
              key={value}
              onClick={() => setCategory(value)}
              className={`px-3 py-1 text-xs rounded-full font-medium transition-colors ${
                category === value
                  ? 'bg-indigo-600 text-white'
                  : 'bg-gray-800 text-gray-400 hover:text-white'
              }`}
            >
              {label}
            </button>
          ))}
        </div>

        {/* Keyword */}
        <input
          className="bg-gray-800 border border-gray-700 rounded-lg px-3 py-1 text-sm text-white placeholder-gray-600 focus:outline-none focus:ring-1 focus:ring-indigo-500 w-52"
          placeholder="Search keyword…"
          value={keyword}
          onChange={(e) => handleKeyword(e.target.value)}
        />

        {/* Min score */}
        <div className="flex items-center gap-2">
          <span className="text-xs text-gray-500">Min score</span>
          <input
            type="number"
            min={0}
            max={100}
            className="bg-gray-800 border border-gray-700 rounded-lg px-3 py-1 text-sm text-white w-20 focus:outline-none focus:ring-1 focus:ring-indigo-500"
            placeholder="0"
            value={minScore}
            onChange={(e) => setMinScore(e.target.value)}
          />
        </div>
      </div>

      {/* Table */}
      <div className="bg-gray-900 border border-gray-800 rounded-xl overflow-hidden">
        {loading ? (
          <p className="px-6 py-8 text-sm text-gray-500">Loading…</p>
        ) : leads.length === 0 ? (
          <p className="px-6 py-8 text-sm text-gray-500">
            No leads yet. Submit a task from the dashboard to start crawling.
          </p>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-800 text-xs text-gray-500 uppercase tracking-wider">
                <th className="px-4 py-3 text-left">Author</th>
                <th className="px-4 py-3 text-left">Content</th>
                <th className="px-4 py-3 text-left">Score</th>
                <th className="px-4 py-3 text-left">Category</th>
                <th className="px-4 py-3 text-left">Signals</th>
                <th className="px-4 py-3 text-left">Date</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-800">
              {leads.map((lead) => (
                <LeadRow key={lead.id} lead={lead} />
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between text-sm text-gray-500">
          <span>
            Page {page + 1} of {totalPages} · {total} total
          </span>
          <div className="flex gap-2">
            <button
              disabled={page === 0}
              onClick={() => setPage((p) => p - 1)}
              className="px-3 py-1 bg-gray-800 rounded-lg hover:bg-gray-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              ← Prev
            </button>
            <button
              disabled={page >= totalPages - 1}
              onClick={() => setPage((p) => p + 1)}
              className="px-3 py-1 bg-gray-800 rounded-lg hover:bg-gray-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              Next →
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

function LeadRow({ lead }: { lead: Lead }) {
  const score = lead.lead_score.toFixed(1)
  const scoreColor =
    lead.category === 'hot'
      ? 'text-rose-400'
      : lead.category === 'warm'
      ? 'text-amber-400'
      : 'text-blue-400'

  return (
    <tr className="hover:bg-gray-800/50 transition-colors group">
      {/* Author */}
      <td className="px-4 py-3">
        {lead.author_profile_url ? (
          <a
            href={lead.author_profile_url}
            target="_blank"
            rel="noreferrer"
            className="text-indigo-400 hover:underline text-xs font-medium"
          >
            {lead.author_name || 'Unknown'}
          </a>
        ) : (
          <span className="text-gray-400 text-xs">{lead.author_name || 'Unknown'}</span>
        )}
      </td>

      {/* Content (truncated) */}
      <td className="px-4 py-3 max-w-xs">
        <p className="text-gray-300 text-xs line-clamp-2 leading-relaxed">{lead.content}</p>
      </td>

      {/* Score */}
      <td className="px-4 py-3">
        <span className={`text-sm font-bold tabular-nums ${scoreColor}`}>{score}</span>
        <span className="text-gray-600 text-xs">/100</span>
      </td>

      {/* Category */}
      <td className="px-4 py-3">
        <LeadCategoryBadge category={lead.category as LeadCategory} />
      </td>

      {/* Signals */}
      <td className="px-4 py-3">
        <div className="flex flex-wrap gap-1">
          {(lead.signals ?? []).slice(0, 3).map((sig, i) => (
            <span
              key={i}
              className="px-1.5 py-0.5 bg-gray-800 text-gray-400 text-xs rounded font-mono"
            >
              {sig}
            </span>
          ))}
          {(lead.signals ?? []).length > 3 && (
            <span className="text-xs text-gray-600">+{lead.signals.length - 3}</span>
          )}
        </div>
      </td>

      {/* Date */}
      <td className="px-4 py-3 text-gray-500 text-xs whitespace-nowrap">
        {new Date(lead.created_at).toLocaleDateString()}
      </td>
    </tr>
  )
}
