import { useEffect, useState } from 'react';
import { cardStyle, inputStyle, primaryBtn, rootStyle, theme } from '../constants/styles';
import { Building2, User, Check, Zap } from 'lucide-react';
import { getMe, isPlatformRole, refreshToken } from '../services/authService';
import type { AuthUser } from '../services/authService';
import { useAuthStore } from '../stores/authStore';

interface OnboardingProps {
  onComplete: (role: 'admin' | 'staff' | 'founder' | 'superadmin') => void;
}

const D = rootStyle;
const card = (x: Record<string, unknown> = {}) => cardStyle({ padding: 32, ...x });
const PB = (p: Record<string, unknown> = {}) => primaryBtn({ padding: '12px 20px', fontSize: 14, ...p });

const Lbl = ({ t }: { t: string }) => <p style={{ color: theme.textFaint, fontSize: 12, fontWeight: 700, marginBottom: 5 }}>{t}</p>;
const Inp = (p: React.InputHTMLAttributes<HTMLInputElement>) => <input style={inputStyle} {...p} />;

const typeOptions = [
  { value: 'team', icon: <Building2 size={28} color={theme.primary} />, label: 'Team / Doanh nghiệp', desc: 'Nhiều thành viên, quản lý tập trung' },
  { value: 'personal', icon: <User size={28} color="#4ade80" />, label: 'Cá nhân', desc: 'Sử dụng một mình hoặc freelancer' },
];

export default function Onboarding({ onComplete }: OnboardingProps) {
  const [orgType, setOrgType] = useState<'team' | 'personal'>('team');
  const [orgName, setOrgName] = useState('');
  const [domain, setDomain] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [done, setDone] = useState(false);

  useEffect(() => {
    let cancelled = false;
    getMe()
      .then(async user => {
        if (cancelled) return;
        if (isPlatformRole(user.role)) {
          onComplete('founder');
          return;
        }
        if (user.org_id === 0) return;
        try {
          const token = await refreshToken();
          useAuthStore.getState().setAuth(token, user);
        } catch {
          useAuthStore.getState().setUser(user);
        }
        onComplete(user.role === 'admin' ? 'admin' : 'staff');
      })
      .catch(() => {});
    return () => { cancelled = true; };
  }, []);

  async function handleSetup() {
    setError('');
    if (!orgName.trim()) { setError('Vui lòng nhập tên tổ chức'); return; }
    setLoading(true);
    try {
      // Phase 4b: route through api.post so we get cookie auth +
      // automatic refresh-on-401 behaviour. The previous direct
      // fetch hardcoded `Authorization: Bearer ${token}` which sent
      // `Bearer null` after the localStorage migration — and because
      // the server prefers the Authorization header over the cookie,
      // a valid cookie was being overridden by a junk header.
      const api = await import('../services/api');
      const data = await api.post<{ access_token: string; user: AuthUser }>(
        '/onboarding/setup',
        { org_name: orgName.trim(), domain: domain.trim(), type: orgType },
      );
      const { useAuthStore } = await import('../stores/authStore');
      useAuthStore.getState().setAuth(data.access_token, data.user);
      setDone(true);
    } catch (err) {
      setError(err instanceof Error && err.message ? err.message : 'Lỗi kết nối, thử lại sau');
    } finally {
      setLoading(false);
    }
  }

  if (done) return (
    <div className="onboarding-shell" style={{ ...D, minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div className="onboarding-card onboarding-success-card" style={{ ...card({ padding: 44 }), textAlign: 'center', maxWidth: 440 }}>
        <div style={{ width: 72, height: 72, background: '#16a34a22', border: '2px solid #16a34a', borderRadius: '50%', display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '0 auto 20px' }}>
          <Check size={34} color="#4ade80" />
        </div>
        <h2 style={{ color: '#f9fafb', fontSize: 22, fontWeight: 700, marginBottom: 8 }}>Workspace đã sẵn sàng!</h2>
        <p style={{ color: '#9ca3af', fontSize: 13, marginBottom: 28 }}>Tổ chức <strong style={{ color: '#e5e7eb' }}>{orgName}</strong> đã được tạo thành công.</p>
        <button className="onboarding-submit-btn" onClick={() => onComplete('admin')} style={{ ...PB(), width: '100%', fontSize: 15 } as React.CSSProperties}>
          Vào AutoFlow →
        </button>
      </div>
    </div>
  );

  return (
    <div className="onboarding-shell" style={{ ...D, minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 20 }}>
      <div className="onboarding-card" style={{ ...card(), maxWidth: 520, width: '100%' }}>
        <div style={{ textAlign: 'center', marginBottom: 32 }}>
          <div style={{ width: 44, height: 44, background: theme.primary, borderRadius: 12, display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '0 auto 14px' }}>
            <Zap size={20} color="#fff" />
          </div>
          <h1 style={{ color: '#f9fafb', fontSize: 22, fontWeight: 700, marginBottom: 6 }}>Tạo workspace của bạn</h1>
          <p style={{ color: '#9ca3af', fontSize: 13 }}>Chỉ mất 30 giây — bắt đầu ngay</p>
        </div>

        {/* Type selector */}
        <div className="onboarding-type-grid" style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, marginBottom: 24 }}>
          {typeOptions.map(opt => (
            <button
              key={opt.value}
              className="onboarding-type-btn"
              onClick={() => setOrgType(opt.value as 'team' | 'personal')}
              style={{
                padding: '16px 14px',
                borderRadius: 12,
                border: `2px solid ${orgType === opt.value ? theme.primary : '#374151'}`,
                background: orgType === opt.value ? '#312e8122' : '#1a1f35',
                cursor: 'pointer',
                textAlign: 'left',
              }}
            >
              <div style={{ marginBottom: 8 }}>{opt.icon}</div>
              <div style={{ color: '#f9fafb', fontSize: 13, fontWeight: 600 }}>{opt.label}</div>
              <div style={{ color: '#9ca3af', fontSize: 11, marginTop: 3 }}>{opt.desc}</div>
            </button>
          ))}
        </div>

        <div style={{ marginBottom: 14 }}>
          <Lbl t={orgType === 'personal' ? 'Tên của bạn / thương hiệu' : 'Tên tổ chức'} />
          <Inp
            placeholder={orgType === 'personal' ? 'Nguyễn Văn A' : 'Công ty TNHH ABC'}
            value={orgName}
            onChange={e => setOrgName(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && handleSetup()}
          />
        </div>

        <div style={{ marginBottom: 22 }}>
          <Lbl t="Website / Domain (tuỳ chọn)" />
          <Inp
            placeholder="abc.vn"
            value={domain}
            onChange={e => setDomain(e.target.value)}
          />
        </div>

        {error && <p style={{ color: '#f87171', fontSize: 12, marginBottom: 12, textAlign: 'center' }}>{error}</p>}

        <button
          className="onboarding-submit-btn"
          onClick={handleSetup}
          disabled={loading}
          style={{ ...PB(), width: '100%', fontSize: 15, opacity: loading ? 0.6 : 1 } as React.CSSProperties}
        >
          {loading ? 'Đang tạo workspace...' : 'Tạo workspace →'}
        </button>
      </div>
    </div>
  );
}
