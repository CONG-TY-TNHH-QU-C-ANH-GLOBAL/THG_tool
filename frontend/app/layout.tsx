import type { Metadata } from 'next'
import { GeistSans } from 'geist/font/sans'
import { GeistMono } from 'geist/font/mono'
import './globals.css'
import { ThemeProvider } from '../components/ThemeProvider'

// The `geist` package (Vercel's official) inlines the font assets at
// build time so the browser never fetches fonts.googleapis.com at
// runtime. Keeps the production CSP tight (style-src 'self'
// 'unsafe-inline' in deploy/nginx.conf) without widening the trust
// boundary to Google's CDN. The CSS variables flow into tokens.css
// via --font-sans / --font-mono.
//
// Why this package and not next/font/google: Geist isn't shipped via
// next/font/google in Next 14.2.5 — Vercel distributes it standalone.

export const metadata: Metadata = {
  title: 'AutoFlow | THG',
  description: 'Facebook Sales Intelligence Workspace for AI-native sales automation teams.',
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html
      lang="vi"
      data-density="balanced"
      suppressHydrationWarning
      className={`${GeistSans.variable} ${GeistMono.variable}`}
    >
      <body>
        <ThemeProvider attribute="data-theme" defaultTheme="dark" enableSystem={false}>
          {children}
        </ThemeProvider>
      </body>
    </html>
  )
}
