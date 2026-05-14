'use client';
import { useState, type ReactNode } from 'react';
import { ArrowLeft, ArrowRight, Check, Mail } from 'lucide-react';
import { useAuth } from '../hooks/useAuth';
import { isPlatformRole, type AuthUser } from '../services/authService';
import { LangSwitch } from './ds/LangSwitch';
import { useLang } from '../i18n/useLang';

type AuthMode = 'login' | 'register' | 'forgot' | 'success';
type AuthSuccessRole = 'admin' | 'staff' | 'founder' | 'superadmin';

interface AuthProps {
  mode: AuthMode;
  setMode: (m: AuthMode) => void;
  onSuccess: (role: AuthSuccessRole) => void;
  goBack: () => void;
}

function routeRoleFor(user?: Partial<AuthUser> | null): AuthSuccessRole {
  if (isPlatformRole(user?.role)) return 'founder';
  return user?.role === 'admin' ? 'admin' : 'staff';
}

const GoogleGlyph = () => (
  <svg width="14" height="14" viewBox="0 0 18 18" aria-hidden="true">
    <path fill="#4285F4" d="M17.64 9.2c0-.637-.057-1.251-.164-1.84H9v3.481h4.844c-.209 1.125-.843 2.078-1.796 2.717v2.258h2.908c1.702-1.567 2.684-3.874 2.684-6.615z" />
    <path fill="#34A853" d="M9 18c2.43 0 4.467-.806 5.956-2.18l-2.908-2.259c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332A8.997 8.997 0 0 0 9 18z" />
    <path fill="#FBBC05" d="M3.964 10.71A5.41 5.41 0 0 1 3.682 9c0-.593.102-1.17.282-1.71V4.958H.957A8.996 8.996 0 0 0 0 9c0 1.452.348 2.827.957 4.042l3.007-2.332z" />
    <path fill="#EA4335" d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.891 11.426 0 9 0A8.997 8.997 0 0 0 .957 4.958L3.964 6.29C4.672 4.163 6.656 3.58 9 3.58z" />
  </svg>
);

function AuthShell({ children, lang }: { children: ReactNode; lang: 'vi' | 'en' }) {
  return (
    <main className="auth-shell">
      <aside className="auth-side">
        <div className="auth-mascot" style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-start' }}>
          <img src="/assets/thg-pegasus.png" alt="Pegasus" style={{ height: 220, opacity: 0.92, display: 'block' }} />
          <img src="/assets/thg-wordmark.png" alt="THG" style={{ height: 64, marginTop: 18, opacity: 0.82, display: 'block' }} />
        </div>
        <div className="auth-tag" style={{ marginTop: 'auto' }}>
          <div className="eyebrow"><span className="dot" />{lang === 'vi' ? 'WORKSPACE FACEBOOK' : 'FACEBOOK WORKSPACE'}</div>
          <h2 style={{ fontSize: 26, marginTop: 14, lineHeight: 1.15, maxWidth: '22ch' }}>
            {lang === 'vi'
              ? 'Facebook automation chỉ phát huy giá trị khi hệ thống thực sự hiểu doanh nghiệp.'
              : 'Facebook automation only works when the system understands the business first.'}
          </h2>
          <p style={{ marginTop: 16, fontSize: 13.5, color: 'var(--text-mute)', maxWidth: '38ch' }}>
            {lang === 'vi'
              ? 'Workspace của đội sales bạn để vận hành Facebook liên tục — không phải scraper một lần.'
              : 'The workspace your sales team uses to run Facebook continuously — not a one-off scraper.'}
          </p>
        </div>
      </aside>
      <div className="auth-form-wrap">{children}</div>
    </main>
  );
}

export default function Auth({ mode, setMode, onSuccess, goBack }: AuthProps) {
  const { lang, t } = useLang();
  const [sent, setSent] = useState(false);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [regName, setRegName] = useState('');
  const [regEmail, setRegEmail] = useState('');
  const [regPassword, setRegPassword] = useState('');
  const [regConfirm, setRegConfirm] = useState('');
  const [regLoading, setRegLoading] = useState(false);
  const [regError, setRegError] = useState('');
  const { login, isLoading } = useAuth();

  async function handleSignup() {
    setRegError('');
    if (!regName || !regEmail || !regPassword) {
      setRegError(lang === 'vi' ? 'Vui lòng điền đầy đủ thông tin' : 'Please fill in every field');
      return;
    }
    if (regPassword !== regConfirm) {
      setRegError(lang === 'vi' ? 'Mật khẩu không khớp' : 'Passwords do not match');
      return;
    }
    setRegLoading(true);
    try {
      const res = await fetch('/api/auth/signup', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: regName, email: regEmail, password: regPassword }),
      });
      const data = await res.json();
      if (!res.ok) {
        setRegError(data.error || (lang === 'vi' ? 'Đăng ký thất bại' : 'Signup failed'));
        return;
      }
      const { useAuthStore } = await import('../stores/authStore');
      useAuthStore.getState().setAuth(data.access_token, data.user);
      onSuccess(routeRoleFor(data.user));
    } catch {
      setRegError(lang === 'vi' ? 'Lỗi kết nối, thử lại sau' : 'Connection error, try again');
    } finally {
      setRegLoading(false);
    }
  }

  async function handleLogin() {
    setError('');
    try {
      await login(email, password);
      const { useAuthStore } = await import('../stores/authStore');
      const user = useAuthStore.getState().user;
      onSuccess(routeRoleFor(user));
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : (lang === 'vi' ? 'Đăng nhập thất bại' : 'Login failed'));
    }
  }

  const goGoogle = () => { window.location.href = '/api/auth/google'; };
  const resetToLogin = () => { setSent(false); setMode('login'); };

  if (mode === 'success') {
    return (
      <AuthShell lang={lang}>
        <div className="auth-form" style={{ textAlign: 'center' }}>
          <div className="auth-success-icon"><Check size={28} /></div>
          <h1>{lang === 'vi' ? 'Tổ chức đã được tạo' : 'Workspace created'}</h1>
          <p className="sub">
            {lang === 'vi' ? 'Workspace của bạn đã sẵn sàng sử dụng.' : 'Your workspace is ready to go.'}
          </p>
          <button className="btn btn-primary btn-lg" style={{ width: '100%', justifyContent: 'center' }} onClick={() => onSuccess('admin')}>
            {lang === 'vi' ? 'Vào workspace' : 'Enter workspace'} <ArrowRight size={14} />
          </button>
        </div>
      </AuthShell>
    );
  }

  if (mode === 'forgot') {
    return (
      <AuthShell lang={lang}>
        <form className="auth-form" onSubmit={e => e.preventDefault()}>
          <button type="button" className="auth-back" onClick={resetToLogin}>
            <ArrowLeft size={13} /> {lang === 'vi' ? 'Quay lại đăng nhập' : 'Back to sign in'}
          </button>
          {!sent ? (
            <>
              <h1>{lang === 'vi' ? 'Quên mật khẩu?' : 'Forgot password?'}</h1>
              <p className="sub">
                {lang === 'vi' ? 'Nhập email tài khoản để nhận link đặt lại mật khẩu.' : 'Enter your account email to receive a reset link.'}
              </p>
              <div className="auth-fields">
                <label className="field">
                  <span className="field-label">EMAIL</span>
                  <input className="input" type="email" placeholder="ban@congty.vn" />
                </label>
              </div>
              <button type="button" className="btn btn-primary btn-lg" style={{ width: '100%', justifyContent: 'center' }} onClick={() => setSent(true)}>
                {lang === 'vi' ? 'Gửi link đặt lại' : 'Send reset link'} <ArrowRight size={14} />
              </button>
            </>
          ) : (
            <div style={{ textAlign: 'center' }}>
              <div className="auth-success-icon"><Mail size={26} /></div>
              <h1>{lang === 'vi' ? 'Đã gửi email' : 'Email sent'}</h1>
              <p className="sub">
                {lang === 'vi' ? 'Kiểm tra hộp thư và nhấn link để đặt lại mật khẩu.' : 'Check your inbox and click the link to reset your password.'}
              </p>
              <button type="button" className="btn btn-ghost" style={{ width: '100%', justifyContent: 'center' }} onClick={() => setMode('login')}>
                {lang === 'vi' ? 'Quay lại đăng nhập' : 'Back to sign in'}
              </button>
            </div>
          )}
        </form>
      </AuthShell>
    );
  }

  if (mode === 'login') {
    return (
      <AuthShell lang={lang}>
        <form className="auth-form" onSubmit={e => e.preventDefault()}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
            <button type="button" className="auth-back" onClick={goBack}>
              <ArrowLeft size={13} /> {lang === 'vi' ? 'Trang chủ' : 'Home'}
            </button>
            <LangSwitch />
          </div>
          <h1>{lang === 'vi' ? 'Chào mừng trở lại.' : 'Welcome back.'}</h1>
          <p className="sub">{t.auth.loginSubtitle}</p>

          <div className="auth-fields">
            <label className="field">
              <span className="field-label">EMAIL</span>
              <input
                className="input"
                type="email"
                value={email}
                onChange={e => setEmail(e.target.value)}
                placeholder="ban@congty.vn"
                autoComplete="email"
              />
            </label>
            <label className="field">
              <span className="field-label">{lang === 'vi' ? 'MẬT KHẨU' : 'PASSWORD'}</span>
              <input
                className="input"
                type="password"
                value={password}
                onChange={e => setPassword(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleLogin()}
                autoComplete="current-password"
                placeholder="••••••••"
              />
              <span className="field-help" style={{ textAlign: 'right' }}>
                <button type="button" className="auth-back" style={{ marginBottom: 0 }} onClick={() => setMode('forgot')}>
                  {t.auth.forgot}
                </button>
              </span>
            </label>
          </div>

          {error && <div className="auth-error">{error}</div>}

          <button type="button" className="btn btn-primary btn-lg" style={{ width: '100%', justifyContent: 'center' }} onClick={handleLogin} disabled={isLoading}>
            {isLoading ? (lang === 'vi' ? 'Đang đăng nhập…' : 'Signing in…') : t.auth.loginCta}
            <ArrowRight size={14} />
          </button>

          <div className="auth-divider">{lang === 'vi' ? 'HOẶC' : 'OR'}</div>

          <button type="button" className="btn btn-ghost" style={{ width: '100%', justifyContent: 'center' }} onClick={goGoogle}>
            <GoogleGlyph /> {t.auth.googleCta}
          </button>

          <div className="auth-foot">
            {t.auth.noAccount}{' '}
            <button type="button" onClick={() => setMode('register')}>
              {lang === 'vi' ? 'Tạo workspace miễn phí' : 'Create a free workspace'} →
            </button>
          </div>
        </form>
      </AuthShell>
    );
  }

  return (
    <AuthShell lang={lang}>
      <form className="auth-form" onSubmit={e => e.preventDefault()}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
          <button type="button" className="auth-back" onClick={goBack}>
            <ArrowLeft size={13} /> {lang === 'vi' ? 'Trang chủ' : 'Home'}
          </button>
          <LangSwitch />
        </div>
        <h1>{t.auth.registerTitle}</h1>
        <p className="sub">{t.auth.registerSubtitle}</p>

        <div className="auth-fields">
          <label className="field">
            <span className="field-label">{lang === 'vi' ? 'HỌ TÊN' : 'FULL NAME'}</span>
            <input className="input" placeholder="Nguyễn Văn A" value={regName} onChange={e => setRegName(e.target.value)} autoComplete="name" />
          </label>
          <label className="field">
            <span className="field-label">EMAIL</span>
            <input className="input" type="email" placeholder="ban@congty.vn" value={regEmail} onChange={e => setRegEmail(e.target.value)} autoComplete="email" />
          </label>
          <label className="field">
            <span className="field-label">{lang === 'vi' ? 'MẬT KHẨU' : 'PASSWORD'}</span>
            <input className="input" type="password" placeholder={lang === 'vi' ? 'Tối thiểu 8 ký tự' : 'Min. 8 characters'} value={regPassword} onChange={e => setRegPassword(e.target.value)} autoComplete="new-password" />
          </label>
          <label className="field">
            <span className="field-label">{lang === 'vi' ? 'XÁC NHẬN' : 'CONFIRM'}</span>
            <input className="input" type="password" placeholder={lang === 'vi' ? 'Nhập lại mật khẩu' : 'Re-enter password'} value={regConfirm} onChange={e => setRegConfirm(e.target.value)} autoComplete="new-password" />
          </label>
        </div>

        {regError && <div className="auth-error">{regError}</div>}

        <button type="button" className="btn btn-primary btn-lg" style={{ width: '100%', justifyContent: 'center' }} onClick={handleSignup} disabled={regLoading}>
          {regLoading ? (lang === 'vi' ? 'Đang tạo…' : 'Creating…') : t.auth.registerCta}
          <ArrowRight size={14} />
        </button>

        <div className="auth-divider">{lang === 'vi' ? 'HOẶC' : 'OR'}</div>

        <button type="button" className="btn btn-ghost" style={{ width: '100%', justifyContent: 'center' }} onClick={goGoogle}>
          <GoogleGlyph /> {t.auth.googleCta}
        </button>

        <div className="auth-foot">
          {t.auth.hasAccount}{' '}
          <button type="button" onClick={() => setMode('login')}>{t.auth.loginCta} →</button>
        </div>
      </form>
    </AuthShell>
  );
}
