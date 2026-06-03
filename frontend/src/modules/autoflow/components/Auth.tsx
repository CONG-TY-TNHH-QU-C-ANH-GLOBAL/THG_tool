'use client';
import { useState, type ReactNode } from 'react';
import { ArrowLeft, ArrowRight, Check, Mail, ShieldCheck, Eye, Users } from 'lucide-react';
import { useAuth } from '../hooks/useAuth';
import { isPlatformRole, type AuthUser } from '../services/authService';
import { LangSwitch } from './ds/LangSwitch';
import { useLang } from '../i18n/useLang';
import styles from './auth.module.css';

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
  <svg width="15" height="15" viewBox="0 0 18 18" aria-hidden="true">
    <path fill="#4285F4" d="M17.64 9.2c0-.637-.057-1.251-.164-1.84H9v3.481h4.844c-.209 1.125-.843 2.078-1.796 2.717v2.258h2.908c1.702-1.567 2.684-3.874 2.684-6.615z" />
    <path fill="#34A853" d="M9 18c2.43 0 4.467-.806 5.956-2.18l-2.908-2.259c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332A8.997 8.997 0 0 0 9 18z" />
    <path fill="#FBBC05" d="M3.964 10.71A5.41 5.41 0 0 1 3.682 9c0-.593.102-1.17.282-1.71V4.958H.957A8.996 8.996 0 0 0 0 9c0 1.452.348 2.827.957 4.042l3.007-2.332z" />
    <path fill="#EA4335" d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.891 11.426 0 9 0A8.997 8.997 0 0 0 .957 4.958L3.964 6.29C4.672 4.163 6.656 3.58 9 3.58z" />
  </svg>
);

function BrandSide({ lang }: { lang: 'vi' | 'en' }) {
  const trust = lang === 'vi'
    ? [
        { icon: ShieldCheck, t: 'Trình duyệt thật, không dùng bot ảo' },
        { icon: Users, t: 'Phân quyền theo từng nhân viên' },
        { icon: Eye, t: 'Giám sát đội sales theo thời gian thực' },
      ]
    : [
        { icon: ShieldCheck, t: 'Real browsers, never fake bots' },
        { icon: Users, t: 'Per-staff permissions' },
        { icon: Eye, t: 'Real-time visibility into your team' },
      ];

  const avatars = [
    { n: 'L', g: 'linear-gradient(140deg,#6366f1,#4338ca)' },
    { n: 'M', g: 'linear-gradient(140deg,#0ea5e9,#0369a1)' },
    { n: 'H', g: 'linear-gradient(140deg,#f43f5e,#be123c)' },
  ];

  return (
    <aside className={styles.side}>
      <div className={styles.sideBrand}>
        <span className={styles.sideMark}>
          <img src="/assets/thg-pegasus.png" alt="THG" />
        </span>
        <span>
          <strong>THG</strong>
          <span>{lang === 'vi' ? 'Nền tảng tự động hoá' : 'Automation platform'}</span>
        </span>
      </div>

      <div className={styles.sideMid}>
        <span className={styles.sideEyebrow}>
          <span className={styles.dotLive} />
          {lang === 'vi' ? 'Workspace bán hàng' : 'Sales workspace'}
        </span>
        <h2 className={styles.sideTitle}>
          {lang === 'vi' ? (
            <>Cả đội sales của bạn, vận hành từ <em>một workspace duy nhất</em>.</>
          ) : (
            <>Your whole sales team, run from <em>one single workspace</em>.</>
          )}
        </h2>
        <p className={styles.sideSub}>
          {lang === 'vi'
            ? 'Facebook, Taobao và 1688 — tự động hóa thông minh bằng trình duyệt thật, an toàn cho mọi tài khoản.'
            : 'Facebook, Taobao and 1688 — smart automation on real browsers, safe for every account.'}
        </p>

        <div className={styles.trust}>
          {trust.map(({ icon: Icon, t }) => (
            <div key={t} className={styles.trustItem}>
              <span className={styles.tick}><Icon size={13} /></span>
              {t}
            </div>
          ))}
        </div>

        <div className={styles.miniCard}>
          <div className={styles.miniAvatars}>
            {avatars.map((a) => (
              <span key={a.n} style={{ background: a.g }}>{a.n}</span>
            ))}
          </div>
          <div>
            <b>{lang === 'vi' ? '5 nhân viên đang hoạt động' : '5 staff active now'}</b>
            <small>{lang === 'vi' ? 'Phản hồi trung bình 2.4 phút' : 'Avg. response 2.4 min'}</small>
          </div>
        </div>
      </div>

      <div className={styles.sideFoot}>© 2026 THG Automation Platform</div>
    </aside>
  );
}

function AuthShell({ children, lang }: { children: ReactNode; lang: 'vi' | 'en' }) {
  return (
    <main className={styles.shell}>
      <BrandSide lang={lang} />
      <div className={styles.formWrap}>{children}</div>
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
        <div className={`${styles.form} ${styles.center}`}>
          <div className={styles.successIcon}><Check size={28} /></div>
          <h1 className={styles.h1}>{lang === 'vi' ? 'Workspace đã được tạo' : 'Workspace created'}</h1>
          <p className={styles.sub}>
            {lang === 'vi' ? 'Workspace của bạn đã sẵn sàng sử dụng.' : 'Your workspace is ready to go.'}
          </p>
          <button className={styles.btnPrimary} onClick={() => onSuccess('admin')}>
            {lang === 'vi' ? 'Vào workspace' : 'Enter workspace'} <ArrowRight size={16} />
          </button>
        </div>
      </AuthShell>
    );
  }

  if (mode === 'forgot') {
    return (
      <AuthShell lang={lang}>
        <form className={styles.form} onSubmit={(e) => e.preventDefault()}>
          <div className={styles.formTop}>
            <button type="button" className={styles.back} onClick={resetToLogin}>
              <ArrowLeft size={14} /> {lang === 'vi' ? 'Quay lại đăng nhập' : 'Back to sign in'}
            </button>
            <LangSwitch />
          </div>
          {!sent ? (
            <>
              <h1 className={styles.h1}>{lang === 'vi' ? 'Quên mật khẩu?' : 'Forgot password?'}</h1>
              <p className={styles.sub}>
                {lang === 'vi' ? 'Nhập email tài khoản để nhận link đặt lại mật khẩu.' : 'Enter your account email to receive a reset link.'}
              </p>
              <div className={styles.fields}>
                <label className={styles.field}>
                  <span className={styles.label}>Email</span>
                  <input className={styles.input} type="email" placeholder="ban@congty.vn" />
                </label>
              </div>
              <button type="button" className={styles.btnPrimary} onClick={() => setSent(true)}>
                {lang === 'vi' ? 'Gửi link đặt lại' : 'Send reset link'} <ArrowRight size={16} />
              </button>
            </>
          ) : (
            <div className={styles.center}>
              <div className={styles.successIcon}><Mail size={26} /></div>
              <h1 className={styles.h1}>{lang === 'vi' ? 'Đã gửi email' : 'Email sent'}</h1>
              <p className={styles.sub}>
                {lang === 'vi' ? 'Kiểm tra hộp thư và nhấn link để đặt lại mật khẩu.' : 'Check your inbox and click the link to reset your password.'}
              </p>
              <button type="button" className={styles.btnGhost} style={{ marginTop: 24 }} onClick={() => setMode('login')}>
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
        <form className={styles.form} onSubmit={(e) => e.preventDefault()}>
          <div className={styles.formTop}>
            <button type="button" className={styles.back} onClick={goBack}>
              <ArrowLeft size={14} /> {lang === 'vi' ? 'Trang chủ' : 'Home'}
            </button>
            <LangSwitch />
          </div>
          <h1 className={styles.h1}>{lang === 'vi' ? 'Chào mừng trở lại.' : 'Welcome back.'}</h1>
          <p className={styles.sub}>{t.auth.loginSubtitle}</p>

          <div className={styles.fields}>
            <label className={styles.field}>
              <span className={styles.label}>Email</span>
              <input
                className={styles.input}
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="ban@congty.vn"
                autoComplete="email"
              />
            </label>
            <label className={styles.field}>
              <span className={`${styles.label} ${styles.labelRow}`}>
                {lang === 'vi' ? 'Mật khẩu' : 'Password'}
                <button type="button" className={styles.inlineLink} onClick={() => setMode('forgot')}>
                  {t.auth.forgot}
                </button>
              </span>
              <input
                className={styles.input}
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleLogin()}
                autoComplete="current-password"
                placeholder="••••••••"
              />
            </label>
          </div>

          {error && <div className={styles.error}>{error}</div>}

          <button type="button" className={styles.btnPrimary} onClick={handleLogin} disabled={isLoading}>
            {isLoading ? (lang === 'vi' ? 'Đang đăng nhập…' : 'Signing in…') : t.auth.loginCta}
            <ArrowRight size={16} />
          </button>

          <div className={styles.divider}>{lang === 'vi' ? 'HOẶC' : 'OR'}</div>

          <button type="button" className={styles.btnGhost} onClick={goGoogle}>
            <GoogleGlyph /> {t.auth.googleCta}
          </button>

          <div className={styles.foot}>
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
      <form className={styles.form} onSubmit={(e) => e.preventDefault()}>
        <div className={styles.formTop}>
          <button type="button" className={styles.back} onClick={goBack}>
            <ArrowLeft size={14} /> {lang === 'vi' ? 'Trang chủ' : 'Home'}
          </button>
          <LangSwitch />
        </div>
        <h1 className={styles.h1}>{t.auth.registerTitle}</h1>
        <p className={styles.sub}>{t.auth.registerSubtitle}</p>

        <div className={styles.fields}>
          <label className={styles.field}>
            <span className={styles.label}>{lang === 'vi' ? 'Họ tên' : 'Full name'}</span>
            <input className={styles.input} placeholder="Nguyễn Văn A" value={regName} onChange={(e) => setRegName(e.target.value)} autoComplete="name" />
          </label>
          <label className={styles.field}>
            <span className={styles.label}>Email</span>
            <input className={styles.input} type="email" placeholder="ban@congty.vn" value={regEmail} onChange={(e) => setRegEmail(e.target.value)} autoComplete="email" />
          </label>
          <label className={styles.field}>
            <span className={styles.label}>{lang === 'vi' ? 'Mật khẩu' : 'Password'}</span>
            <input className={styles.input} type="password" placeholder={lang === 'vi' ? 'Tối thiểu 8 ký tự' : 'Min. 8 characters'} value={regPassword} onChange={(e) => setRegPassword(e.target.value)} autoComplete="new-password" />
          </label>
          <label className={styles.field}>
            <span className={styles.label}>{lang === 'vi' ? 'Xác nhận mật khẩu' : 'Confirm password'}</span>
            <input className={styles.input} type="password" placeholder={lang === 'vi' ? 'Nhập lại mật khẩu' : 'Re-enter password'} value={regConfirm} onChange={(e) => setRegConfirm(e.target.value)} autoComplete="new-password" />
          </label>
        </div>

        {regError && <div className={styles.error}>{regError}</div>}

        <button type="button" className={styles.btnPrimary} onClick={handleSignup} disabled={regLoading}>
          {regLoading ? (lang === 'vi' ? 'Đang tạo…' : 'Creating…') : t.auth.registerCta}
          <ArrowRight size={16} />
        </button>

        <div className={styles.divider}>{lang === 'vi' ? 'HOẶC' : 'OR'}</div>

        <button type="button" className={styles.btnGhost} onClick={goGoogle}>
          <GoogleGlyph /> {t.auth.googleCta}
        </button>

        <div className={styles.foot}>
          {t.auth.hasAccount}{' '}
          <button type="button" onClick={() => setMode('login')}>{t.auth.loginCta} →</button>
        </div>
      </form>
    </AuthShell>
  );
}
