'use client';
import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { LogOut } from 'lucide-react';
import * as api from '../../services/api';
import { useAuthStore, type AuthUser } from '../../stores/authStore';

/**
 * Leave Workspace (membership-vulnerability fix): the user-facing,
 * NON-destructive exit — the login account survives and can join
 * another workspace. The last admin is blocked server-side
 * (LAST_ADMIN) until they hand over the admin role.
 */
export default function LeaveWorkspaceCard() {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState('');

  async function leave() {
    if (!window.confirm('Rời workspace này?\nTài khoản đăng nhập của bạn vẫn còn và có thể tạo hoặc tham gia workspace khác.')) return;
    setBusy(true);
    setMsg('');
    try {
      const data = await api.post<{ access_token: string; user: AuthUser }>('/auth/me/leave-workspace', {});
      const store = useAuthStore.getState();
      store.setAuth(data.access_token, data.user);
      await store.hydrate();
      router.push('/services');
    } catch (err) {
      setMsg(err instanceof Error ? err.message : 'Không rời được workspace.');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="card" style={{ padding: 14, display: 'flex', flexDirection: 'column', gap: 8 }}>
      <strong style={{ fontSize: 13 }}>Rời workspace</strong>
      <p style={{ fontSize: 12, color: 'var(--text-mute)', margin: 0 }}>
        Bạn sẽ không còn là thành viên của workspace này. Tài khoản đăng nhập vẫn còn — bạn có thể tạo
        workspace mới hoặc nhận lời mời khác. Nếu bạn là admin cuối cùng, hãy chuyển quyền admin trước.
      </p>
      {msg && <p style={{ fontSize: 12, color: 'var(--hot)', margin: 0 }}>{msg}</p>}
      <div>
        <button type="button" className="btn btn-ghost btn-sm" disabled={busy} onClick={() => void leave()} style={{ color: 'var(--hot)' }}>
          <LogOut size={12} /> {busy ? 'Đang xử lý…' : 'Rời workspace'}
        </button>
      </div>
    </div>
  );
}
