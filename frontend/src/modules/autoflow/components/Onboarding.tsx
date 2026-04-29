import { useEffect, useState } from 'react';
import { theme } from '../constants/styles';
import { Building2, User, Check, Zap } from 'lucide-react';
import { getMe, refreshToken } from '../services/authService';
import { useAuthStore } from '../stores/authStore';

interface OnboardingProps {
  onComplete: (role: 'admin' | 'staff' | 'superadmin') => void;
}

const D = { background: theme.bg, color: theme.text, fontFamily: 'system-ui, sans-serif' };
const card = (x: Record<string, unknown> = {}) => ({ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 16, padding: 32, ...x });
const inp = { background: '#2a2f45', border: '1px solid #374151', borderRadius: 9, padding: '10px 14px', color: '#fff', fontSize: 13, outline: 'none', width: '100%', boxSizing: 'border-box' as const };
const PB = (p: Record<string, unknown> = {}) => ({ padding: '12px 20px', borderRadius: 9, border: 'none', cursor: 'pointer', fontSize: 14, fontWeight: 600, background: theme.primary, color: '#fff', ...p });

const Lbl = ({ t }: { t: string }) => <p style={{ color: '#9ca3af', fontSize: 12, marginBottom: 5 }}>{t}</p>;
const Inp = (p: React.InputHTMLAttributes<HTMLInputElement>) => <input style={inp} {...p} />;

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
        if (cancelled || user.org_id === 0) return;
        try {
          const token = await refreshToken();
          useAuthStore.getState().setAuth(token, user);
        } catch {
          useAuthStore.getState().setUser(user);
        }
        onComplete(user.role === 'superadmin' ? 'superadmin' : user.role === 'admin' ? 'admin' : 'staff');
      })
      .catch(() => {});
    return () => { cancelled = true; };
  }, []);

  async function handleSetup() {
    setError('');
    if (!orgName.trim()) { setError('Vui lòng nhập tên tổ chức'); return; }
    setLoading(true);
    try {
      const { useAuthStore } = await import('../stores/authStore');
      const token = useAuthStore.getState().token;
      const res = await fetch('/api/onboarding/setup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` },
        body: JSON.stringify({ org_name: orgName.trim(), domain: domain.trim(), type: orgType }),
      });
      const data = await res.json();
      if (!res.ok) { setError(data.error || 'Không tạo được tổ chức'); return; }
      // Update auth store with new token (now has org_id)
      useAuthStore.getState().setAuth(data.access_token, data.user);
      setDone(true);
    } catch {
      setError('Lỗi kết nối, thử lại sau');
    } finally {
      setLoading(false);
    }
  }

  if (done) return (
    <div style={{ ...D, minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div style={{ ...card({ padding: 44 }), textAlign: 'center', maxWidth: 440 }}>
        <div style={{ width: 72, height: 72, background: '#16a34a22', border: '2px solid #16a34a', borderRadius: '50%', display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '0 auto 20px' }}>
          <Check size={34} color="#4ade80" />
        </div>
        <h2 style={{ color: '#f9fafb', fontSize: 22, fontWeight: 700, marginBottom: 8 }}>Workspace đã sẵn sàng!</h2>
        <p style={{ color: '#9ca3af', fontSize: 13, marginBottom: 28 }}>Tổ chức <strong style={{ color: '#e5e7eb' }}>{orgName}</strong> đã được tạo thành công.</p>
        <button onClick={() => onComplete('admin')} style={{ ...PB(), width: '100%', fontSize: 15 } as React.CSSProperties}>
          Vào AutoFlow →
        </button>
      </div>
    </div>
  );

  return (
    <div style={{ ...D, minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 20 }}>
      <div style={{ ...card(), maxWidth: 520, width: '100%' }}>
        <div style={{ textAlign: 'center', marginBottom: 32 }}>
          <div style={{ width: 44, height: 44, background: theme.primary, borderRadius: 12, display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '0 auto 14px' }}>
            <Zap size={20} color="#fff" />
          </div>
          <h1 style={{ color: '#f9fafb', fontSize: 22, fontWeight: 700, marginBottom: 6 }}>Tạo workspace của bạn</h1>
          <p style={{ color: '#9ca3af', fontSize: 13 }}>Chỉ mất 30 giây — bắt đầu ngay</p>
        </div>

        {/* Type selector */}
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, marginBottom: 24 }}>
          {typeOptions.map(opt => (
            <button
              key={opt.value}
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
