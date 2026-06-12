'use client';
import { useCallback, useEffect, useRef, useState } from 'react';
import { Bell } from 'lucide-react';
import {
  listNotifications,
  markAllNotificationsRead,
  markNotificationRead,
  type AppNotification,
} from '../../services/notificationsService';
import InviteNotificationCard from './InviteNotificationCard';

const POLL_MS = 60_000;

/**
 * Notification bell (PR-1): unread badge + dropdown. Invite
 * notifications render as explicit accept cards; everything else as a
 * plain title/body row. Polls lazily + refetches on window focus.
 */
export default function NotificationBell() {
  const [open, setOpen] = useState(false);
  const [items, setItems] = useState<AppNotification[]>([]);
  const [unread, setUnread] = useState(0);
  const wrapRef = useRef<HTMLDivElement | null>(null);

  const refresh = useCallback(() => {
    listNotifications()
      .then(({ notifications, unread }) => {
        setItems(notifications);
        setUnread(unread);
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, POLL_MS);
    const onFocus = () => refresh();
    window.addEventListener('focus', onFocus);
    return () => {
      clearInterval(interval);
      window.removeEventListener('focus', onFocus);
    };
  }, [refresh]);

  useEffect(() => {
    function onClick(e: MouseEvent) {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener('mousedown', onClick);
    return () => document.removeEventListener('mousedown', onClick);
  }, []);

  async function handleHandled(id: number) {
    try {
      await markNotificationRead(id);
    } catch {
      /* already read / gone */
    }
    refresh();
  }

  return (
    <div ref={wrapRef} style={{ position: 'relative' }}>
      <button
        type="button"
        className="btn btn-ghost btn-sm"
        aria-label="Notifications"
        onClick={() => setOpen(o => !o)}
        style={{ position: 'relative' }}
      >
        <Bell size={14} />
        {unread > 0 && (
          <span
            style={{
              position: 'absolute', top: 0, right: 0, minWidth: 14, height: 14,
              borderRadius: 7, background: 'var(--hot)', color: '#fff',
              fontSize: 9, lineHeight: '14px', textAlign: 'center', padding: '0 3px',
            }}
          >
            {unread > 9 ? '9+' : unread}
          </span>
        )}
      </button>
      {open && (
        <div
          className="card"
          style={{
            position: 'absolute', top: '100%', right: 0, marginTop: 6, width: 340,
            maxHeight: 420, overflowY: 'auto', zIndex: 60, padding: 0,
            boxShadow: '0 10px 30px rgba(0,0,0,0.25)',
          }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '8px 12px', borderBottom: '1px solid var(--line)' }}>
            <strong style={{ fontSize: 12 }}>Thông báo</strong>
            {unread > 0 && (
              <button
                type="button"
                className="btn btn-ghost btn-sm"
                onClick={() => { void markAllNotificationsRead().then(refresh); }}
              >
                Đánh dấu đã đọc
              </button>
            )}
          </div>
          {items.length === 0 && (
            <p style={{ padding: 16, fontSize: 12, color: 'var(--text-faint)', margin: 0 }}>
              Chưa có thông báo nào.
            </p>
          )}
          {items.map(n =>
            n.type === 'workspace_invite_received' && !n.read_at ? (
              <InviteNotificationCard key={n.id} notification={n} onHandled={id => void handleHandled(id)} />
            ) : (
              <div
                key={n.id}
                style={{ padding: '10px 12px', borderBottom: '1px solid var(--line)', opacity: n.read_at ? 0.6 : 1, cursor: n.read_at ? 'default' : 'pointer' }}
                onClick={() => { if (!n.read_at) void handleHandled(n.id); }}
              >
                <strong style={{ fontSize: 12.5, color: 'var(--text)', display: 'block', marginBottom: 2 }}>{n.title}</strong>
                <span style={{ fontSize: 12, color: 'var(--text-mute)' }}>{n.body}</span>
              </div>
            ),
          )}
        </div>
      )}
    </div>
  );
}
