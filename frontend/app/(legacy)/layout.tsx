import { Nav } from '@/components/Nav'
import { PageWrapper } from '@/components/PageWrapper'

export default function LegacyLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-screen overflow-hidden">
      <Nav />
      <main className="flex-1 overflow-y-auto min-w-0">
        <PageWrapper>{children}</PageWrapper>
      </main>
    </div>
  )
}
