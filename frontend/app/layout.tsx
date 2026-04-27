import type { Metadata } from 'next'
import { Inter } from 'next/font/google'
import './globals.css'
import { Nav } from '@/components/Nav'
import { PageWrapper } from '@/components/PageWrapper'

const inter = Inter({ subsets: ['latin'] })

export const metadata: Metadata = {
  title: 'THG Lead Intelligence',
  description: 'AI-powered lead intelligence & crawl orchestration',
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className={inter.className}>
        <div className="flex h-screen overflow-hidden">
          <Nav />
          <main className="flex-1 overflow-y-auto min-w-0">
            <PageWrapper>{children}</PageWrapper>
          </main>
        </div>
      </body>
    </html>
  )
}
