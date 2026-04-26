import type { JobStatus, LeadCategory } from '@/lib/types'

const JOB_COLORS: Record<JobStatus, string> = {
  pending:   'bg-gray-700 text-gray-300',
  running:   'bg-amber-900/60 text-amber-300 animate-pulse',
  completed: 'bg-emerald-900/60 text-emerald-300',
  failed:    'bg-rose-900/60 text-rose-300',
}

const LEAD_COLORS: Record<LeadCategory, string> = {
  hot:  'bg-rose-900/60 text-rose-300',
  warm: 'bg-amber-900/60 text-amber-300',
  cold: 'bg-blue-900/60 text-blue-300',
}

export function JobStatusBadge({ status }: { status: JobStatus }) {
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded-md text-xs font-medium ${JOB_COLORS[status]}`}>
      {status}
    </span>
  )
}

export function LeadCategoryBadge({ category }: { category: LeadCategory }) {
  const icons: Record<LeadCategory, string> = { hot: '🔥', warm: '☀️', cold: '❄️' }
  return (
    <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-medium ${LEAD_COLORS[category]}`}>
      {icons[category]} {category}
    </span>
  )
}
