'use client'
import Link from 'next/link'
import { usePathname } from 'next/navigation'

const NAV = [
  { href: '/dashboard', icon: '📊', label: 'Dashboard' },
  { href: '/jobs',      icon: '⚙️',  label: 'Jobs'      },
  { href: '/leads',     icon: '🎯',  label: 'Leads'     },
]

export function Nav() {
  const path = usePathname()
  return (
    <aside className="w-56 shrink-0 bg-gray-900 border-r border-gray-800 flex flex-col">
      <div className="px-4 py-5 border-b border-gray-800">
        <p className="text-base font-bold text-white leading-tight">THG Intelligence</p>
        <p className="text-xs text-gray-500 mt-0.5">Lead AI System</p>
      </div>

      <nav className="flex-1 p-3 space-y-0.5">
        {NAV.map(({ href, icon, label }) => {
          const active = path?.startsWith(href)
          return (
            <Link
              key={href}
              href={href}
              className={`flex items-center gap-2.5 px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
                active
                  ? 'bg-indigo-600 text-white'
                  : 'text-gray-400 hover:bg-gray-800 hover:text-white'
              }`}
            >
              <span className="text-base leading-none">{icon}</span>
              {label}
            </Link>
          )
        })}
      </nav>

      <div className="px-4 py-3 border-t border-gray-800">
        <p className="text-xs text-gray-600">v0.1.0 · mock runtime</p>
      </div>
    </aside>
  )
}
