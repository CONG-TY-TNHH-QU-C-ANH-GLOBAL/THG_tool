'use client';
import { useEffect, useState, type ReactNode } from 'react';
import { ArrowLeft, ArrowRight, Check, UserPlus } from 'lucide-react';
import * as api from '../services/api';
import { isPlatformRole, type AuthUser } from '../services/authService';
import { useAuth } from '../hooks/useAuth';
import { useAuthStore } from '../stores/authStore';
import { LangSwitch } from './ds/LangSwitch';
import { useLang } from '../i18n/useLang';

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

function Shell({ children, lang }: { children: ReactNode; lang: 'vi' | 'en' }) {
  return (
    <main className="auth-shell">
      <aside className="auth-side">
        <div className="brand">
          <div className="brand-mark">A</div>
          <span className="brand-name">AutoFlow<span className="dim">.thg</span></span>
        </div>
        <div>
          <div className="eyebrow"><span className="dot" />WORKSPACE INVITE</div>
          <h2 style={{ marginTop: 16, fontSize: 36, maxWidth: 380 }}>
            {lang === 'vi' ? (
              <>Tham gia <span className="title-mono">workspace của team.</span></>
            ) : (
              <>Join your team's <span className="title-mono">workspace.</span></>
            )}
          </h2>
          <p style={{ marginTop: 16, maxWidth: 360, fontSize: 14 }}>
            {lang === 'vi'
              ? 'Tài khoản nhân viên được tạo bởi chính người nhận invite, gắn đúng workspace + role + audit log riêng.'
              : 'Each invitee creates their own user, bound to the right workspace, role and audit trail.'}
          </p>
        </div>
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-faint)', letterSpacing: '0.04em' }}>
          <span className="pulse" style={{ verticalAlign: 'middle', marginRight: 8 }} />
          {lang === 'vi' ? 'Email-bound · 1 invite = 1 staff login' : 'Email-bound · 1 invite = 1 staff login'}
        </div>
      </aside>
      <div className="auth-form-wrap">{children}</div>
    </main>
  );
}

export default function JoinWorkspace({ token, onJoined, goBack }: JoinWorkspaceProps) {
  const { user, login, isLoading } = useAuth();
  const { lang } = useLang();
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
      .catch(() => setMsg(lang === 'vi' ? 'Invite không tồn tại hoặc đã hết hạn.' : 'Invite not found or expired.'));
  }, [token, lang]);

  const acceptInvite = async () => {
    setMsg('');
    setLoading(true);
    try {
      const data = await api.post<{ access_token: string; user: AuthUser }>(`/auth/join/${encodeURIComponent(token)}`, {});
      useAuthStore.getState().setAuth(data.access_token, data.user);
      onJoined(routeRoleFor(data.user));
    } catch (err) {
      setMsg(err instanceof Error ? err.message : (lang === 'vi' ? 'Không nhận được invite.' : 'Failed to accept invite.'));
    } finally {
      setLoading(false);
    }
  };

  const signupAndJoin = async () => {
    setMsg('');
    if (!invite) return;
    if (!name.trim() || !password || password !== confirm) {
      setMsg(lang === 'vi' ? 'Điền tên và xác nhận mật khẩu hợp lệ.' : 'Enter your name and a matching password confirmation.');
      return;
    }
    setLoading(true);
    try {
      const res = await fetch('/api/auth/signup', {
        method: 'POST', credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name.trim(), email: invite.email, password }),
      });
      const data = await res.json();
      if (!res.ok) {
        setMsg(data.error || (lang === 'vi' ? 'Không tạo được tài khoản.' : 'Could not create account.'));
        if (res.status === 409) setMode('login');
        return;
      }
      useAuthStore.getState().setAuth(data.access_token, data.user);
      onJoined(routeRoleFor(data.user));
    } catch {
      setMsg(lang === 'vi' ? 'Lỗi kết nối, thử lại sau.' : 'Connection error, try again.');
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
      setMsg(err instanceof Error ? err.message : (lang === 'vi' ? 'Đăng nhập thất bại.' : 'Login failed.'));
      setLoading(false);
    }
  };

  return (
    <Shell lang={lang}>
      <form className="auth-form" onSubmit={e => e.preventDefault()}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
          <button type="button" className="auth-back" onClick={goBack}>
            <ArrowLeft size={13} /> {lang === 'vi' ? 'Quay lại' : 'Back'}
          </button>
          <LangSwitch />
        </div>

        <div style={{ display: 'inline-flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
          <span className="avatar"><UserPlus size={14} /></span>
          <span className="eyebrow">{lang === 'vi' ? 'JOIN WORKSPACE' : 'JOIN WORKSPACE'}</span>
        </div>
        <h1>{invite ? (lang === 'vi' ? `Tham gia ${invite.org_name}` : `Join ${invite.org_name}`) : (lang === 'vi' ? 'Đang kiểm tra invite' : 'Checking invite')}</h1>
        <p className="sub">{invite ? `${invite.email} · ${invite.role}` : (lang === 'vi' ? 'Đang tải thông tin lời mời…' : 'Loading invite…')}</p>

        {user && invite && (
          <div className="card" style={{ padding: 16, marginBottom: 16 }}>
            <div className="eyebrow" style={{ marginBottom: 6 }}>
              {lang === 'vi' ? 'ĐANG ĐĂNG NHẬP' : 'CURRENTLY SIGNED IN'}
            </div>
            <p style={{ fontSize: 13, color: 'var(--text)', marginBottom: 12 }}>{user.email}</p>
            <button type="button" className="btn btn-ghost btn-sm" onClick={acceptInvite} disabled={loading}>
              <Check size={13} /> {lang === 'vi' ? 'Nhận invite ngay' : 'Accept invite'}
            </button>
          </div>
        )}

        {mode === 'signup' ? (
          <>
            <div className="auth-fields">
              <label className="field">
                <span className="field-label">{lang === 'vi' ? 'HỌ TÊN' : 'FULL NAME'}</span>
                <input className="input" value={name} onChange={e => setName(e.target.value)} autoComplete="name" />
              </label>
              <label className="field">
                <span className="field-label">EMAIL</span>
                <input className="input" value={invite?.email ?? email} readOnly />
              </label>
              <label className="field">
                <span className="field-label">{lang === 'vi' ? 'MẬT KHẨU' : 'PASSWORD'}</span>
                <input className="input" type="password" value={password} onChange={e => setPassword(e.target.value)} autoComplete="new-password" />
              </label>
              <label className="field">
                <span className="field-label">{lang === 'vi' ? 'XÁC NHẬN' : 'CONFIRM'}</span>
                <input className="input" type="password" value={confirm} onChange={e => setConfirm(e.target.value)} autoComplete="new-password" />
              </label>
            </div>
            {msg && <div className="auth-error">{msg}</div>}
            <button type="button" className="btn btn-primary btn-lg" style={{ width: '100%', justifyContent: 'center' }} onClick={signupAndJoin} disabled={loading || !invite}>
              {loading ? (lang === 'vi' ? 'Đang tạo…' : 'Creating…') : (lang === 'vi' ? 'Tạo tài khoản và vào workspace' : 'Create account and join')}
              <ArrowRight size={14} />
            </button>
            <div className="auth-foot">
              {lang === 'vi' ? 'Đã có tài khoản?' : 'Already have an account?'}{' '}
              <button type="button" onClick={() => setMode('login')}>
                {lang === 'vi' ? 'Đăng nhập để nhận invite' : 'Sign in to accept'} →
              </button>
            </div>
          </>
        ) : (
          <>
            <div className="auth-fields">
              <label className="field">
                <span className="field-label">EMAIL</span>
                <input className="input" type="email" value={email} onChange={e => setEmail(e.target.value)} autoComplete="email" />
              </label>
              <label className="field">
                <span className="field-label">{lang === 'vi' ? 'MẬT KHẨU' : 'PASSWORD'}</span>
                <input className="input" type="password" value={password} onChange={e => setPassword(e.target.value)} autoComplete="current-password" />
              </label>
            </div>
            {msg && <div className="auth-error">{msg}</div>}
            <button type="button" className="btn btn-primary btn-lg" style={{ width: '100%', justifyContent: 'center' }} onClick={loginAndJoin} disabled={loading || isLoading || !invite}>
              {loading || isLoading ? (lang === 'vi' ? 'Đang xử lý…' : 'Working…') : (lang === 'vi' ? 'Đăng nhập và nhận invite' : 'Sign in and accept')}
              <ArrowRight size={14} />
            </button>
            <div className="auth-foot">
              {lang === 'vi' ? 'Chưa có tài khoản?' : 'No account yet?'}{' '}
              <button type="button" onClick={() => setMode('signup')}>
                {lang === 'vi' ? 'Tạo tài khoản' : 'Create account'} →
              </button>
            </div>
          </>
        )}
      </form>
    </Shell>
  );
}
