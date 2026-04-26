import type { SSEEvent } from './types'

// SSE connects to the proxied endpoint so the browser sees same-origin.
const SSE_URL = '/api/v1/events/stream'
const RECONNECT_DELAY_MS = 3_000

/**
 * Connect to the SSE stream. Automatically reconnects on errors.
 * Returns a cleanup function — call it to close the connection.
 *
 * Frontend contract: events arrive as JSON matching SSEEvent.
 * Frontend MUST NOT derive scores or filter state from events —
 * only use the backend-supplied values directly.
 */
export function connectSSE(onEvent: (e: SSEEvent) => void): () => void {
  if (typeof window === 'undefined') return () => {}

  let es: EventSource | null = null
  let retryTimer: ReturnType<typeof setTimeout> | null = null
  let closed = false

  const connect = () => {
    if (closed) return
    es = new EventSource(SSE_URL)

    es.onmessage = (msg) => {
      if (!msg.data || msg.data === '') return
      try {
        const event = JSON.parse(msg.data) as SSEEvent
        onEvent(event)
      } catch {
        // malformed event — ignore
      }
    }

    es.onerror = () => {
      es?.close()
      if (!closed) {
        retryTimer = setTimeout(connect, RECONNECT_DELAY_MS)
      }
    }
  }

  connect()

  return () => {
    closed = true
    es?.close()
    if (retryTimer) clearTimeout(retryTimer)
  }
}
