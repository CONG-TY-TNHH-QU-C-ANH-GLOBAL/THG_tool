interface ProgressBarProps {
  value: number   // 0–100
  label?: boolean // show percentage text
  size?: 'sm' | 'md'
}

export function ProgressBar({ value, label = false, size = 'sm' }: Readonly<ProgressBarProps>) {
  const pct = Math.min(100, Math.max(0, value))
  const h = size === 'md' ? 'h-2.5' : 'h-1.5'
  const color = pct === 100 ? 'bg-emerald-500' : pct > 0 ? 'bg-indigo-500' : 'bg-gray-700'

  return (
    <div className="flex items-center gap-2 w-full">
      <div className={`flex-1 bg-gray-800 rounded-full overflow-hidden ${h}`}>
        <div
          className={`${h} ${color} rounded-full transition-all duration-300`}
          style={{ width: `${pct}%` }}
        />
      </div>
      {label && (
        <span className="text-xs text-gray-400 tabular-nums w-8 text-right">{pct}%</span>
      )}
    </div>
  )
}
