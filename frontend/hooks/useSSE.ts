'use client'
import { useEffect, useRef } from 'react'
import { connectSSE } from '@/lib/sse-client'
import type { SSEEvent } from '@/lib/types'

/**
 * Subscribe to the global SSE stream. The callback fires for every event.
 * Uses a ref so the caller can safely use unstable callback references.
 */
export function useSSE(onEvent: (e: SSEEvent) => void): void {
  const cbRef = useRef(onEvent)
  cbRef.current = onEvent

  useEffect(() => {
    const disconnect = connectSSE((e) => cbRef.current(e))
    return disconnect
  }, [])
}
