import type { Metadata } from 'next'
import './globals.css'
import { ThemeProvider } from '../components/ThemeProvider'

export const metadata: Metadata = {
  title: 'AutoFlow | THG',
  description: 'Facebook Sales Intelligence Workspace for AI-native sales automation teams.',
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="vi" data-density="balanced" suppressHydrationWarning>
      <head>
        {/* Geist + Geist Mono are the sole font families per the design
            system. JetBrains Mono stays as a fallback for code blocks
            that still benefit from stable OS monospace metrics. */}
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="" />
        <link
          href="https://fonts.googleapis.com/css2?family=Geist:wght@400;500;600;700&family=Geist+Mono:ital,wght@0,400;0,500;1,400&display=swap"
          rel="stylesheet"
        />
      </head>
      <body>
        <ThemeProvider attribute="data-theme" defaultTheme="dark" enableSystem={false}>
          {children}
        </ThemeProvider>
      </body>
    </html>
  )
}
