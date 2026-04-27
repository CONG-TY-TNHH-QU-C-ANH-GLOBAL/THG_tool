'use client'
import dynamic from 'next/dynamic'

const AutoFlowApp = dynamic(
  () => import('@/src/modules/autoflow/AutoFlowApp'),
  { ssr: false }
)

export default function HomePage() {
  return <AutoFlowApp />
}
