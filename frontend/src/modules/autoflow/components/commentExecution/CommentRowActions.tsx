'use client';

import { useState } from 'react';
import { CheckCircle, RotateCw } from 'lucide-react';
import type { OutboundMessage } from '../../services/outboxService';
import { humanVerifyComment, retryComment } from '../../services/outboxService';
import { commentActions, commentStatus, effectiveOutcome } from './statusMessages';

// Per-comment manual actions (spec: specs/COMMENT_ASYNC_REVERIFY.md companion, Part A/B).
// "Xác nhận đã đăng" appears ONLY for submitted_unverified (operator saw it on Facebook);
// "Thử lại" ONLY for retryable pre-submit failures. Both call the backend correction/queue —
// the FE never silently changes state. Rendered in the detail pane (the row is a <button>).

interface Props {
  message: OutboundMessage;
  correctionReason?: string; // a succeeded correction (human_verified/reverified) → already posted
  onDone: () => void; // reload the list after a successful action
}

export function CommentRowActions({ message, correctionReason, onDone }: Readonly<Props>) {
  const [busy, setBusy] = useState(false);
  // Use the EFFECTIVE outcome so a corrected (manually/async verified) row reads as success
  // → the "Xác nhận đã đăng" button auto-hides once a correction exists.
  const outcome = effectiveOutcome(message.verification_outcome, correctionReason);
  const { severity } = commentStatus(message.execution_state ?? '', outcome);
  // Skip open_post here — the detail pane already renders a "Mở post" link.
  const actions = commentActions(severity, outcome, undefined)
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
