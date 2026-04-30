import { useState, type ReactNode } from 'react';
import { ArrowLeft, Building2, Check, LockKeyhole, Mail, Zap } from 'lucide-react';
import { useAuth } from '../hooks/useAuth';
import { isPlatformRole, type AuthUser } from '../services/authService';

type AuthMode = 'login' | 'register' | 'forgot' | 'success';
type AuthSuccessRole = 'admin' | 'staff' | 'founder' | 'superadmin';

interface AuthProps {
  mode: AuthMode;
  setMode: (m: AuthMode) => void;
  onSuccess: (role: AuthSuccessRole) => void;
  onNeedsOnboarding?: () => void;
  goBack: () => void;
}

function routeRoleFor(user?: Partial<AuthUser> | null): AuthSuccessRole {
  if (isPlatformRole(user?.role)) return 'founder';
  return user?.role === 'admin' ? 'admin' : 'staff';
}

const GoogleIcon = () => (
  <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true">
    <path fill="#4285F4" d="M17.64 9.2c0-.637-.057-1.251-.164-1.84H9v3.481h4.844c-.209 1.125-.843 2.078-1.796 2.717v2.258h2.908c1.702-1.567 2.684-3.874 2.684-6.615z" />
    <path fill="#34A853" d="M9 18c2.43 0 4.467-.806 5.956-2.18l-2.908-2.259c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332A8.997 8.997 0 0 0 9 18z" />
    <path fill="#FBBC05" d="M3.964 10.71A5.41 5.41 0 0 1 3.682 9c0-.593.102-1.17.282-1.71V4.958H.957A8.996 8.996 0 0 0 0 9c0 1.452.348 2.827.957 4.042l3.007-2.332z" />
    <path fill="#EA4335" d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.891 11.426 0 9 0A8.997 8.997 0 0 0 .957 4.958L3.964 6.29C4.672 4.163 6.656 3.58 9 3.58z" />
  </svg>
);

const Field = ({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) => (
  <label className="auth-field">
    <span>{label}</span>
    {children}
  </label>
);

export default function Auth({ mode, setMode, onSuccess, onNeedsOnboarding, goBack }: AuthProps) {
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
      setRegError('Vui lòng điền đầy đủ thông tin');
      return;
    }
    if (regPassword !== regConfirm) {
      setRegError('Mật khẩu không khớp');
      return;
    }
    setRegLoading(true);
    try {
      const res = await fetch('/api/auth/signup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: regName, email: regEmail, password: regPassword }),
      });
      const data = await res.json();
      if (!res.ok) {
        setRegError(data.error || 'Đăng ký thất bại');
        return;
      }
      const { useAuthStore } = await import('../stores/authStore');
      useAuthStore.getState().setAuth(data.access_token, data.user);
      if (data.needs_onboarding && onNeedsOnboarding) {
        onNeedsOnboarding();
      } else {
        onSuccess(routeRoleFor(data.user));
      }
    } catch {
      setRegError('Lỗi kết nối, thử lại sau');
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
      if (isPlatformRole(user?.role)) {
        onSuccess(routeRoleFor(user));
        return;
      }
      if (user?.org_id === 0 && onNeedsOnboarding) {
        onNeedsOnboarding();
        return;
      }
      onSuccess(routeRoleFor(user));
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Đăng nhập thất bại');
    }
  }

  const resetToLogin = () => {
    setSent(false);
    setMode('login');
  };

  const goGoogle = () => {
    window.location.href = '/api/auth/google';
  };

  if (mode === 'success') {
    return (
      <AuthShell compact>
        <div className="auth-card auth-card-centered">
          <div className="auth-success-icon">
            <Check size={30} />
          </div>
          <h1>Tổ chức đã được tạo</h1>
          <p>Workspace của bạn đã sẵn sàng sử dụng.</p>
          <button className="auth-primary-btn" onClick={() => onSuccess('admin')}>Vào workspace</button>
        </div>
      </AuthShell>
    );
  }

  if (mode === 'forgot') {
    return (
      <AuthShell compact>
        <div className="auth-card">
          <button className="auth-back-btn" onClick={resetToLogin}>
            <ArrowLeft size={14} />
            Quay lại đăng nhập
          </button>

          {!sent ? (
            <>
              <AuthHeader icon={<Mail size={22} />} title="Quên mật khẩu?" subtitle="Nhập email tài khoản để nhận link đặt lại mật khẩu." />
              <Field label="Email tài khoản">
                <input className="auth-input" type="email" placeholder="you@company.com" />
              </Field>
              <button className="auth-primary-btn" onClick={() => setSent(true)}>Gửi link đặt lại</button>
            </>
          ) : (
            <div className="auth-card-centered">
              <div className="auth-success-icon">
                <Check size={26} />
              </div>
              <h1>Email đã được gửi</h1>
              <p>Kiểm tra hộp thư và nhấn link để đặt lại mật khẩu.</p>
              <button className="auth-secondary-btn" onClick={() => setMode('login')}>Quay lại đăng nhập</button>
            </div>
          )}
        </div>
      </AuthShell>
    );
  }

  if (mode === 'login') {
    return (
      <AuthShell>
        <div className="auth-card">
          <button className="auth-back-btn" onClick={goBack}>
            <ArrowLeft size={14} />
            Trang chủ
          </button>

          <AuthHeader icon={<LockKeyhole size={22} />} title="Đăng nhập AutoFlow" subtitle="Truy cập workspace automation của tổ chức." />

          <div className="auth-form">
            <Field label="Email">
              <input
                className="auth-input"
                type="email"
                value={email}
                onChange={e => setEmail(e.target.value)}
                placeholder="you@company.com"
                autoComplete="email"
              />
            </Field>

            <Field label="Mật khẩu">
              <input
                className="auth-input"
                type="password"
                value={password}
                onChange={e => setPassword(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleLogin()}
                autoComplete="current-password"
              />
            </Field>
          </div>

          <div className="auth-inline-row">
            <span />
            <button className="auth-link-btn" onClick={() => setMode('forgot')}>Quên mật khẩu?</button>
          </div>

          {error && <p className="auth-error">{error}</p>}

          <button className="auth-primary-btn" onClick={handleLogin} disabled={isLoading}>
            {isLoading ? 'Đang đăng nhập...' : 'Đăng nhập'}
          </button>

          <AuthDivider />

          <button className="auth-google-btn" onClick={goGoogle}>
            <GoogleIcon />
            Đăng nhập với Google
          </button>

          <p className="auth-switch">
            Chưa có tài khoản?
            <button onClick={() => setMode('register')}>Tạo tổ chức</button>
          </p>
        </div>
      </AuthShell>
    );
  }

  return (
    <AuthShell>
      <div className="auth-card auth-card-wide">
        <button className="auth-back-btn" onClick={goBack}>
          <ArrowLeft size={14} />
          Trang chủ
        </button>

        <AuthHeader icon={<Building2 size={22} />} title="Tạo tài khoản" subtitle="Khởi tạo tài khoản quản trị cho workspace." />

        <div className="auth-register-grid">
          <Field label="Họ và tên">
            <input className="auth-input" placeholder="Nguyễn Văn A" value={regName} onChange={e => setRegName(e.target.value)} autoComplete="name" />
          </Field>
          <Field label="Email">
            <input className="auth-input" type="email" placeholder="you@company.com" value={regEmail} onChange={e => setRegEmail(e.target.value)} autoComplete="email" />
          </Field>
          <Field label="Mật khẩu">
            <input className="auth-input" type="password" placeholder="Tối thiểu 8 ký tự" value={regPassword} onChange={e => setRegPassword(e.target.value)} autoComplete="new-password" />
          </Field>
          <Field label="Xác nhận mật khẩu">
            <input className="auth-input" type="password" placeholder="Nhập lại mật khẩu" value={regConfirm} onChange={e => setRegConfirm(e.target.value)} autoComplete="new-password" />
          </Field>
        </div>

        {regError && <p className="auth-error">{regError}</p>}

        <button className="auth-primary-btn" onClick={handleSignup} disabled={regLoading}>
          {regLoading ? 'Đang tạo...' : 'Tạo tài khoản'}
        </button>

        <AuthDivider />

        <button className="auth-google-btn" onClick={goGoogle}>
          <GoogleIcon />
          Đăng ký với Google
        </button>

        <p className="auth-switch">
          Đã có tài khoản?
          <button onClick={() => setMode('login')}>Đăng nhập</button>
        </p>
      </div>
    </AuthShell>
  );
}

function AuthShell({ children, compact = false }: { children: ReactNode; compact?: boolean }) {
  return (
    <main className={`auth-shell ${compact ? 'auth-shell-compact' : ''}`}>
      <section className="auth-stage">
        {!compact && (
          <aside className="auth-brand-panel" aria-hidden="true">
            <div className="auth-brand-mark">
              <Zap size={22} />
            </div>
            <div>
              <p className="auth-eyebrow">AutoFlow Workspace</p>
              <h2>AI Facebook Sales Intelligence</h2>
              <p className="auth-brand-copy">Quản lý workspace, tài khoản Facebook và automation theo từng tổ chức.</p>
            </div>
            <div className="auth-status-list">
              <div><span /> Browser workspace</div>
              <div><span /> Lead intelligence</div>
              <div><span /> Team approval flow</div>
            </div>
          </aside>
        )}
        <div className="auth-form-panel">{children}</div>
      </section>
    </main>
  );
}

function AuthHeader({ icon, title, subtitle }: { icon: ReactNode; title: string; subtitle: string }) {
  return (
    <div className="auth-header">
      <div className="auth-header-icon">{icon}</div>
      <h1>{title}</h1>
      <p>{subtitle}</p>
    </div>
  );
}

function AuthDivider() {
  return (
    <div className="auth-divider">
      <span />
      <small>hoặc</small>
      <span />
    </div>
  );
}
