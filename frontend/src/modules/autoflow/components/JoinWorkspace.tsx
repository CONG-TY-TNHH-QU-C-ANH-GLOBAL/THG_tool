import { useEffect, useState, type ReactNode } from 'react';
import { ArrowLeft, Check, LockKeyhole, Mail, UserPlus } from 'lucide-react';
import * as api from '../services/api';
import { isPlatformRole, type AuthUser } from '../services/authService';
import { useAuth } from '../hooks/useAuth';
import { useAuthStore } from '../stores/authStore';

interface InviteInfo {
  org_name: string;
  email: string;
  role: string;
  expires_at: string;
}

interface JoinWorkspaceProps {
  token: string;
  onJoined: (role: 'admin' | 'staff' | 'founder' | 'superadmin') => void;
  goBack: () => void;
}

function routeRoleFor(user?: Partial<AuthUser> | null): 'admin' | 'staff' | 'founder' | 'superadmin' {
  if (isPlatformRole(user?.role)) return 'founder';
  return user?.role === 'admin' ? 'admin' : 'staff';
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="auth-field">
      <span>{label}</span>
      {children}
    </label>
  );
}

export default function JoinWorkspace({ token, onJoined, goBack }: JoinWorkspaceProps) {
  const { user, login, isLoading } = useAuth();
  const [invite, setInvite] = useState<InviteInfo | null>(null);
  const [mode, setMode] = useState<'signup' | 'login'>('signup');
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [msg, setMsg] = useState('');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    setMsg('');
    fetch(`/api/auth/invite/${encodeURIComponent(token)}`)
      .then(r => (r.ok ? r.json() : Promise.reject()))
      .then((data: InviteInfo) => {
        setInvite(data);
        setEmail(data.email);
      })
      .catch(() => setMsg('Invite không tồn tại hoặc đã hết hạn.'));
  }, [token]);

  const acceptInvite = async () => {
    setMsg('');
    setLoading(true);
    try {
      const data = await api.post<{ access_token: string; user: AuthUser }>(`/auth/join/${encodeURIComponent(token)}`, {});
      useAuthStore.getState().setAuth(data.access_token, data.user);
      onJoined(routeRoleFor(data.user));
    } catch (err) {
      setMsg(err instanceof Error ? err.message : 'Không nhận được invite.');
    } finally {
      setLoading(false);
    }
  };

  const signupAndJoin = async () => {
    setMsg('');
    if (!invite) return;
    if (!name.trim() || !password || password !== confirm) {
      setMsg('Điền tên và xác nhận mật khẩu hợp lệ.');
      return;
    }
    setLoading(true);
    try {
      const res = await fetch('/api/auth/signup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({ name: name.trim(), email: invite.email, password }),
      });
      const data = await res.json();
      if (!res.ok) {
        setMsg(data.error || 'Không tạo được tài khoản.');
        if (res.status === 409) setMode('login');
        return;
      }
      useAuthStore.getState().setAuth(data.access_token, data.user);
      onJoined(routeRoleFor(data.user));
    } catch {
      setMsg('Lỗi kết nối, thử lại sau.');
    } finally {
      setLoading(false);
    }
  };

  const loginAndJoin = async () => {
    setMsg('');
    if (!invite) return;
    setLoading(true);
    try {
      await login(email, password);
      const loggedInUser = useAuthStore.getState().user;
      if (loggedInUser?.org_id && loggedInUser.email.toLowerCase() === invite.email.toLowerCase()) {
        onJoined(routeRoleFor(loggedInUser));
        return;
      }
      await acceptInvite();
    } catch (err) {
      setMsg(err instanceof Error ? err.message : 'Đăng nhập thất bại.');
      setLoading(false);
    }
  };

  return (
    <main className="auth-shell">
      <section className="auth-stage">
        <aside className="auth-brand-panel" aria-hidden="true">
          <div className="auth-brand-mark">
            <UserPlus size={22} />
          </div>
          <div>
            <p className="auth-eyebrow">Workspace Invite</p>
            <h2>Join THG AutoFlow</h2>
            <p className="auth-brand-copy">Tài khoản nhân viên được tạo bởi chính người nhận invite, gắn đúng workspace, role và audit log riêng.</p>
          </div>
          <div className="auth-status-list">
            <div><span /> Email-bound invite</div>
            <div><span /> Real workspace user ID</div>
            <div><span /> Independent staff login</div>
          </div>
        </aside>

        <div className="auth-form-panel">
          <div className="auth-card auth-card-wide">
            <button className="auth-back-btn" onClick={goBack}>
              <ArrowLeft size={14} />
              Quay lại
            </button>

            <div className="auth-header">
              <div className="auth-header-icon">
                {mode === 'signup' ? <Mail size={22} /> : <LockKeyhole size={22} />}
              </div>
              <h1>{invite ? `Tham gia ${invite.org_name}` : 'Đang kiểm tra invite'}</h1>
              <p>{invite ? `${invite.email} · ${invite.role}` : 'Đang tải thông tin lời mời workspace.'}</p>
            </div>

            {user && invite && (
              <div style={{ marginBottom: 18, padding: 12, border: '1px solid #2a3146', borderRadius: 10, background: '#151a28' }}>
                <p style={{ color: '#e5e7eb', fontSize: 13, fontWeight: 700, margin: 0 }}>Đang đăng nhập: {user.email}</p>
                <p style={{ color: '#9ca3af', fontSize: 12, margin: '5px 0 12px' }}>Nhận invite bằng tài khoản hiện tại nếu email trùng với lời mời.</p>
                <button className="auth-secondary-btn" onClick={acceptInvite} disabled={loading}>
                  <Check size={16} />
                  Nhận invite
                </button>
              </div>
            )}

            {mode === 'signup' ? (
              <div className="auth-form">
                <Field label="Họ và tên">
                  <input className="auth-input" value={name} onChange={e => setName(e.target.value)} autoComplete="name" />
                </Field>
                <Field label="Email được mời">
                  <input className="auth-input" value={invite?.email ?? email} readOnly />
                </Field>
                <Field label="Mật khẩu">
                  <input className="auth-input" type="password" value={password} onChange={e => setPassword(e.target.value)} autoComplete="new-password" />
                </Field>
                <Field label="Xác nhận mật khẩu">
                  <input className="auth-input" type="password" value={confirm} onChange={e => setConfirm(e.target.value)} autoComplete="new-password" />
                </Field>
                {msg && <p className="auth-error">{msg}</p>}
                <button className="auth-primary-btn" onClick={signupAndJoin} disabled={loading || !invite}>
                  {loading ? 'Đang tạo...' : 'Tạo tài khoản và vào workspace'}
                </button>
                <p className="auth-switch">
                  Đã có tài khoản?
                  <button onClick={() => setMode('login')}>Đăng nhập để nhận invite</button>
                </p>
              </div>
            ) : (
              <div className="auth-form">
                <Field label="Email">
                  <input className="auth-input" type="email" value={email} onChange={e => setEmail(e.target.value)} autoComplete="email" />
                </Field>
                <Field label="Mật khẩu">
                  <input className="auth-input" type="password" value={password} onChange={e => setPassword(e.target.value)} autoComplete="current-password" />
                </Field>
                {msg && <p className="auth-error">{msg}</p>}
                <button className="auth-primary-btn" onClick={loginAndJoin} disabled={loading || isLoading || !invite}>
                  {loading || isLoading ? 'Đang xử lý...' : 'Đăng nhập và nhận invite'}
                </button>
                <p className="auth-switch">
                  Chưa có tài khoản?
                  <button onClick={() => setMode('signup')}>Tạo tài khoản</button>
                </p>
              </div>
            )}
          </div>
        </div>
      </section>
    </main>
  );
}
