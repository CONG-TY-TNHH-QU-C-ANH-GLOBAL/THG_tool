'use client'
import { usePathname } from 'next/navigation'

export function PageWrapper({ children }: { children: React.ReactNode }) {
  const path = usePathname()
  const fullBleed = path?.startsWith('/browser')
  if (fullBleed) return <div className="h-full overflow-hidden">{children}</div>
  return <div className="p-6">{children}</div>
}
