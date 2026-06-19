interface KPICardProps {
  label: string
  value: string | number
  sub?: string
  color?: 'default' | 'green' | 'yellow' | 'red' | 'indigo'
}

const colorMap = {
  default: 'text-white',
  green:   'text-emerald-400',
  yellow:  'text-amber-400',
  red:     'text-rose-400',
  indigo:  'text-indigo-400',
}

export function KPICard({ label, value, sub, color = 'default' }: Readonly<KPICardProps>) {
  return (
    <div className="bg-gray-900 border border-gray-800 rounded-xl px-5 py-4">
      <p className="text-xs font-medium text-gray-500 uppercase tracking-wide">{label}</p>
      <p className={`mt-1.5 text-3xl font-bold tabular-nums ${colorMap[color]}`}>{value}</p>
      {sub && <p className="mt-1 text-xs text-gray-600">{sub}</p>}
    </div>
  )
}
