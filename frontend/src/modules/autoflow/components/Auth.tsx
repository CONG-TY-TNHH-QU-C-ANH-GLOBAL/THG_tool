import { useState } from 'react';
import { theme } from '../constants/styles';
import { ArrowLeft, Zap, Check, Mail } from 'lucide-react';
import { useAuth } from '../hooks/useAuth';

type AuthMode = 'login' | 'register' | 'forgot' | 'success';

interface AuthProps {
  mode: AuthMode;
  setMode: (m: AuthMode) => void;
  onSuccess: (role: 'admin' | 'staff' | 'superadmin') => void;
  goBack: () => void;
}

const D = { background: theme.bg, color: theme.text, fontFamily: 'system-ui, sans-serif' };
const card = (x: Record<string, unknown> = {}) => ({ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 12, padding: 20, ...x });
const inp = { background: '#2a2f45', border: '1px solid #374151', borderRadius: 9, padding: '10px 14px', color: '#fff', fontSize: 13, outline: 'none', width: '100%', boxSizing: 'border-box' as const };
const PB = (p: Record<string, unknown> = {}) => ({ padding: '10px 20px', borderRadius: 9, border: 'none', cursor: 'pointer', fontSize: 14, fontWeight: 500, background: theme.primary, color: '#fff', ...p });
const SB = (p: Record<string, unknown> = {}) => ({ padding: '10px 20px', borderRadius: 9, border: '1px solid #374151', cursor: 'pointer', fontSize: 13, background: 'transparent', color: '#d1d5db', ...p });

const Lbl = ({ t }: { t: string }) => <p style={{ color: '#9ca3af', fontSize: 12, marginBottom: 5 }}>{t}</p>;
const Inp = (p: React.InputHTMLAttributes<HTMLInputElement>) => <input style={inp} {...p} />;

const box = { maxWidth: 460, margin: '48px auto', ...card({ padding: 36 }) };

export default function Auth({ mode, setMode, onSuccess, goBack }: AuthProps) {
  const [step, setStep] = useState(1);
  const [sent, setSent] = useState(false);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const { login, isLoading } = useAuth();

  async function handleLogin() {
    setError('');
    try {
      await login(email, password);
      const { useAuthStore } = await import('../stores/authStore');
      const user = useAuthStore.getState().user;
      onSuccess(user?.role === 'superadmin' ? 'superadmin' : user?.role === 'admin' ? 'admin' : 'staff');
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Đăng nhập thất bại');
    }
  }

  const Back = () => (
    <button
      onClick={() => { setSent(false); setMode('login'); }}
      style={{ display: 'flex', alignItems: 'center', gap: 6, background: 'none', border: 'none', color: '#9ca3af', fontSize: 13, cursor: 'pointer', marginBottom: 22, padding: 0 }}
    >
      <ArrowLeft size={13} />Quay lại đăng nhập
    </button>
  );

  if (mode === 'success') return (
    <div style={{ ...D, minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div style={{ ...card({ padding: 44 }), textAlign: 'center', maxWidth: 440 }}>
        <div style={{ width: 72, height: 72, background: '#16a34a22', border: '2px solid #16a34a', borderRadius: '50%', display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '0 auto 18px' }}>
          <Check size={34} color="#4ade80" />
        </div>
        <h2 style={{ color: '#f9fafb', fontSize: 21, fontWeight: 700, marginBottom: 8 }}>Tổ chức đã được tạo!</h2>
        <p style={{ color: '#9ca3af', fontSize: 13, marginBottom: 0 }}>Workspace của bạn đã sẵn sàng sử dụng.</p>
        <div style={{ background: '#111520', borderRadius: 10, padding: 16, margin: '20px 0', textAlign: 'left' }}>
          {[{ l: 'Tổ chức', v: 'VinFast Sản Xuất' }, { l: 'Gói', v: 'Pro (Trial 14 ngày)' }, { l: 'Admin', v: 'admin@vinfast.vn' }, { l: 'Org ID', v: 'org_vf_2025_042' }].map(r => (
            <div key={r.l} style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
              <span style={{ color: '#6b7280', fontSize: 12 }}>{r.l}</span>
              <span style={{ color: '#e5e7eb', fontSize: 12, fontWeight: 500 }}>{r.v}</span>
            </div>
          ))}
        </div>
        <button onClick={() => onSuccess('admin')} style={{ ...PB(), width: '100%', padding: '12px', fontSize: 15 } as React.CSSProperties}>Vào workspace →</button>
      </div>
    </div>
  );

  if (mode === 'forgot') return (
    <div style={{ ...D, minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div style={box}>
        <Back />
        {!sent ? (
          <>
            <div style={{ textAlign: 'center', marginBottom: 26 }}>
              <div style={{ width: 48, height: 48, background: '#312e8133', border: '1px solid #4f46e544', borderRadius: 14, display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '0 auto 14px' }}>
                <Mail size={22} color="#818cf8" />
              </div>
              <h2 style={{ color: '#f9fafb', fontSize: 19, fontWeight: 700 }}>Quên mật khẩu?</h2>
              <p style={{ color: '#9ca3af', fontSize: 13, marginTop: 6 }}>Nhập email — chúng tôi gửi link đặt lại ngay</p>
            </div>
            <Lbl t="Email tài khoản" />
            <Inp type="email" placeholder="you@company.com" style={{ marginBottom: 16 }} />
            <button onClick={() => setSent(true)} style={{ ...PB(), width: '100%', padding: '11px' } as React.CSSProperties}>Gửi link đặt lại</button>
          </>
        ) : (
          <div style={{ textAlign: 'center' }}>
            <div style={{ width: 58, height: 58, background: '#16a34a22', border: '2px solid #16a34a55', borderRadius: '50%', display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '0 auto 18px' }}>
              <Check size={26} color="#4ade80" />
            </div>
            <h3 style={{ color: '#f9fafb', fontSize: 17, fontWeight: 600, marginBottom: 8 }}>Email đã được gửi!</h3>
            <p style={{ color: '#9ca3af', fontSize: 13, marginBottom: 20 }}>Kiểm tra hộp thư và nhấn link để đặt lại mật khẩu. Hiệu lực 30 phút.</p>
            <button onClick={() => setMode('login')} style={SB({ fontSize: 13 }) as React.CSSProperties}>Quay lại đăng nhập</button>
          </div>
        )}
      </div>
    </div>
  );

  if (mode === 'login') return (
    <div style={{ ...D, minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div style={box}>
        <button onClick={goBack} style={{ display: 'flex', alignItems: 'center', gap: 6, background: 'none', border: 'none', color: '#9ca3af', fontSize: 12, cursor: 'pointer', marginBottom: 22, padding: 0 }}>
          <ArrowLeft size={13} />Trang chủ
        </button>
        <div style={{ textAlign: 'center', marginBottom: 26 }}>
          <div style={{ width: 40, height: 40, background: theme.primary, borderRadius: 10, display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '0 auto 12px' }}>
            <Zap size={18} color="#fff" />
          </div>
          <h2 style={{ color: '#f9fafb', fontSize: 20, fontWeight: 700 }}>Đăng nhập AutoFlow</h2>
        </div>
        <Lbl t="Email" /><Inp type="email" value={email} onChange={e => setEmail(e.target.value)} placeholder="you@company.com" style={{ marginBottom: 14 }} />
        <Lbl t="Mật khẩu" /><Inp type="password" value={password} onChange={e => setPassword(e.target.value)} onKeyDown={e => e.key === 'Enter' && handleLogin()} />
        <div style={{ display: 'flex', justifyContent: 'flex-end', margin: '8px 0 18px' }}>
          <button onClick={() => setMode('forgot')} style={{ background: 'none', border: 'none', color: '#818cf8', fontSize: 12, cursor: 'pointer' }}>Quên mật khẩu?</button>
        </div>
        {error && <p style={{ color: '#f87171', fontSize: 12, marginBottom: 12, textAlign: 'center' }}>{error}</p>}
        <button onClick={handleLogin} disabled={isLoading} style={{ ...PB(), width: '100%', padding: '12px', fontSize: 14, fontWeight: 700, opacity: isLoading ? 0.6 : 1 } as React.CSSProperties}>
          {isLoading ? 'Đang đăng nhập...' : 'Đăng nhập'}
        </button>
        <p style={{ textAlign: 'center', color: '#6b7280', fontSize: 13, marginTop: 18 }}>
          Chưa có tài khoản?{' '}
          <button onClick={() => setMode('register')} style={{ background: 'none', border: 'none', color: '#818cf8', cursor: 'pointer', fontSize: 13 }}>Tạo tổ chức</button>
        </p>
      </div>
    </div>
  );

  // REGISTER
  return (
    <div style={{ ...D, minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div style={{ ...box, maxWidth: 520 }}>
        <button onClick={goBack} style={{ display: 'flex', alignItems: 'center', gap: 6, background: 'none', border: 'none', color: '#9ca3af', fontSize: 12, cursor: 'pointer', marginBottom: 20, padding: 0 }}>
          <ArrowLeft size={13} />Trang chủ
        </button>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 26, justifyContent: 'center' }}>
          {[1, 2].map(s => (
            <span key={s} style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
              <div style={{ width: 28, height: 28, borderRadius: '50%', background: step >= s ? theme.primary : '#2a2f45', display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#fff', fontSize: 13, fontWeight: 600 }}>
                {step > s ? <Check size={13} /> : s}
              </div>
              {s < 2 && <div style={{ width: 40, height: 2, background: step > 1 ? theme.primary : '#2a2f45' }} />}
            </span>
          ))}
        </div>
        {step === 1 ? (
          <>
            <h2 style={{ color: '#f9fafb', fontSize: 19, fontWeight: 700, marginBottom: 4 }}>Tạo tài khoản</h2>
            <p style={{ color: '#9ca3af', fontSize: 13, marginBottom: 22 }}>Bước 1: Thông tin cá nhân</p>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 13, marginBottom: 13 }}>
              <div><Lbl t="Họ và tên" /><Inp placeholder="Nguyễn Văn A" /></div>
              <div><Lbl t="Email" /><Inp type="email" placeholder="you@company.com" /></div>
              <div><Lbl t="Mật khẩu" /><Inp type="password" placeholder="Tối thiểu 8 ký tự" /></div>
              <div><Lbl t="Xác nhận mật khẩu" /><Inp type="password" placeholder="Nhập lại" /></div>
            </div>
            <button onClick={() => setStep(2)} style={{ ...PB(), width: '100%', padding: '11px' } as React.CSSProperties}>Tiếp theo →</button>
          </>
        ) : (
          <>
            <h2 style={{ color: '#f9fafb', fontSize: 19, fontWeight: 700, marginBottom: 4 }}>Tạo tổ chức</h2>
            <p style={{ color: '#9ca3af', fontSize: 13, marginBottom: 22 }}>Bước 2: Thông tin tổ chức</p>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 13, marginBottom: 13 }}>
              <div style={{ gridColumn: '1/-1' }}><Lbl t="Tên tổ chức" /><Inp placeholder="VinFast Sản Xuất" /></div>
              <div><Lbl t="Lĩnh vực" /><select style={inp}><option>Sản xuất</option><option>Bán lẻ</option><option>Công nghệ</option><option>Bất động sản</option><option>Khác</option></select></div>
              <div><Lbl t="Số nhân viên" /><select style={inp}><option>1-5</option><option>6-20</option><option>21-50</option><option>50+</option></select></div>
              <div><Lbl t="Gói dịch vụ" /><select style={inp}><option>Starter — 990K/tháng</option><option>Pro — 2.9M/tháng</option><option>Enterprise</option></select></div>
              <div><Lbl t="Mã giới thiệu" /><Inp placeholder="REF-XXXX (nếu có)" /></div>
            </div>
            <button onClick={() => setMode('success')} style={{ ...PB(), width: '100%', padding: '11px', fontWeight: 700 } as React.CSSProperties}>Tạo tổ chức →</button>
            <button onClick={() => setStep(1)} style={{ display: 'block', background: 'none', border: 'none', color: '#9ca3af', fontSize: 12, cursor: 'pointer', margin: '12px auto 0' }}>← Quay lại</button>
          </>
        )}
      </div>
    </div>
  );
}
