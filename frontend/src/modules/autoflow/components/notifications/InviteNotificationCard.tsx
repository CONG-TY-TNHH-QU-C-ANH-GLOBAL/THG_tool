'use client';
import { ArrowRight, UserPlus } from 'lucide-react';
import {
  parsePayload,
  type AppNotification,
  type InvitePayload,
} from '../../services/notificationsService';
import { useAcceptInvite } from './useAcceptInvite';

interface Props {
  notification: AppNotification;
  onHandled: (id: number) => void; // mark-read + refresh list
}

/**
 * The explicit invite acceptance card (PR-1): an invite is never
 * auto-accepted — the user must press «Đồng ý tham gia».
 */
export default function InviteNotificationCard({ notification, onHandled }: Props) {
  const { accept, accepting, error } = useAcceptInvite();
  const payload = parsePayload<InvitePayload>(notification);

  async function handleAccept() {
    if (!payload?.token) return;
    const ok = await accept(payload.token);
    if (ok) onHandled(notification.id);
  }

  return (
    <div style={{ padding: '10px 12px', borderBottom: '1px solid var(--line)' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
        <UserPlus size={13} color="var(--accent)" />
        <strong style={{ fontSize: 12.5, color: 'var(--text)' }}>{notification.title}</strong>
      </div>
      <p style={{ fontSize: 12, color: 'var(--text-mute)', margin: '0 0 8px' }}>{notification.body}</p>
      {error && (
        <p style={{ fontSize: 11.5, color: 'var(--hot)', margin: '0 0 8px' }}>
          Không nhận được lời mời — thử lại hoặc dùng link trong email.
        </p>
      )}
      <div style={{ display: 'flex', gap: 8 }}>
        <button
          type="button"
          className="btn btn-primary btn-sm"
          disabled={accepting || !payload?.token}
          onClick={() => void handleAccept()}
        >
          {accepting ? 'Đang tham gia…' : 'Đồng ý tham gia'} <ArrowRight size={12} />
        </button>
        <button
          type="button"
          className="btn btn-ghost btn-sm"
          disabled={accepting}
          onClick={() => onHandled(notification.id)}
        >
          Để sau
        </button>
      </div>
    </div>
  );
}
