'use client';

import { useState } from 'react';
import { CheckCircle, RotateCw } from 'lucide-react';
import type { OutboundMessage } from '../../services/outboxService';
import { humanVerifyComment, retryComment } from '../../services/outboxService';
import { commentActions, commentStatus } from './statusMessages';

// Per-comment manual actions (spec: specs/COMMENT_ASYNC_REVERIFY.md companion, Part A/B).
// "Xác nhận đã đăng" appears ONLY for submitted_unverified (operator saw it on Facebook);
// "Thử lại" ONLY for retryable pre-submit failures. Both call the backend correction/queue —
// the FE never silently changes state. Rendered in the detail pane (the row is a <button>).

interface Props {
  message: OutboundMessage;
  onDone: () => void; // reload the list after a successful action
}

export function CommentRowActions({ message, onDone }: Props) {
  const [busy, setBusy] = useState(false);
  const { severity } = commentStatus(message.execution_state ?? '', message.verification_outcome ?? '');
  // Skip open_post here — the detail pane already renders a "Mở post" link.
  const actions = commentActions(severity, message.verification_outcome ?? '', undefined)
    .filter((a) => a.key !== 'open_post');
  if (actions.length === 0) return null;

  async function run(key: string, confirmText?: string) {
    if (busy) return;
    if (confirmText && !window.confirm(confirmText)) return;
    setBusy(true);
    try {
      if (key === 'human_verify') await humanVerifyComment(message.id);
      else if (key === 'retry') await retryComment(message.id);
      onDone();
    } catch (err) {
      window.alert(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      {actions.map((a) => (
        <button
          key={a.key}
          type="button"
          className="btn btn-ghost btn-sm"
          disabled={busy || !a.enabled}
          onClick={() => void run(a.key, a.confirm)}
          style={a.key === 'human_verify' ? { color: 'var(--ok, #16a34a)' } : undefined}
        >
          {a.key === 'human_verify' ? <CheckCircle size={13} /> : <RotateCw size={13} />}
          {a.label}
        </button>
      ))}
    </>
  );
}
