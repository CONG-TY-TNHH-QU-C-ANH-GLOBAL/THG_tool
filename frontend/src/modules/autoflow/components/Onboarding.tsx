'use client';
import { useEffect, useState } from 'react';
import { ArrowRight, Building2, Check, User } from 'lucide-react';
import { getMe, isPlatformRole, refreshToken } from '../services/authService';
import type { AuthUser } from '../services/authService';
import { useAuthStore } from '../stores/authStore';
import { LangSwitch } from './ds/LangSwitch';
import { useLang } from '../i18n/useLang';

interface OnboardingProps {
  onComplete: (role: 'admin' | 'staff' | 'founder' | 'superadmin') => void;
}

export default function Onboarding({ onComplete }: OnboardingProps) {
  const { lang } = useLang();
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
        if (isPlatformRole(user.role)) { onComplete('founder'); return; }
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
  }, [onComplete]);

  async function handleSetup() {
    setError('');
    if (!orgName.trim()) { setError(lang === 'vi' ? 'Vui lòng nhập tên tổ chức' : 'Please enter the workspace name'); return; }
    setLoading(true);
    try {
      const api = await import('../services/api');
      const data = await api.post<{ access_token: string; user: AuthUser }>(
        '/onboarding/setup',
        { org_name: orgName.trim(), domain: domain.trim(), type: orgType },
      );
      const { useAuthStore } = await import('../stores/authStore');
      useAuthStore.getState().setAuth(data.access_token, data.user);
      setDone(true);
    } catch (err) {
      setError(err instanceof Error && err.message ? err.message : (lang === 'vi' ? 'Lỗi kết nối, thử lại sau' : 'Connection error, try again'));
    } finally {
      setLoading(false);
    }
  }

  if (done) {
    return (
      <main style={{ minHeight: '100vh', display: 'grid', placeItems: 'center', padding: 24 }}>
        <div className="card" style={{ maxWidth: 440, textAlign: 'center', padding: 40 }}>
          <div className="auth-success-icon"><Check size={28} /></div>
          <h2 style={{ fontSize: 24, marginBottom: 8 }}>
            {lang === 'vi' ? 'Workspace đã sẵn sàng' : 'Workspace ready'}
          </h2>
          <p style={{ color: 'var(--text-mute)', marginBottom: 28 }}>
            {lang === 'vi' ? <>Tổ chức <strong style={{ color: 'var(--text)' }}>{orgName}</strong> đã được tạo thành công.</> : <>Workspace <strong style={{ color: 'var(--text)' }}>{orgName}</strong> created successfully.</>}
          </p>
          <button className="btn btn-primary btn-lg" style={{ width: '100%', justifyContent: 'center' }} onClick={() => onComplete('admin')}>
            {lang === 'vi' ? 'Vào AutoFlow' : 'Enter AutoFlow'} <ArrowRight size={14} />
          </button>
        </div>
      </main>
    );
  }

  const typeOptions = [
    { value: 'team' as const, icon: Building2, label: lang === 'vi' ? 'Team / Doanh nghiệp' : 'Team / Business', desc: lang === 'vi' ? 'Nhiều thành viên, quản lý tập trung' : 'Multiple members, central admin' },
    { value: 'personal' as const, icon: User, label: lang === 'vi' ? 'Cá nhân' : 'Personal', desc: lang === 'vi' ? 'Sử dụng một mình hoặc freelancer' : 'Solo operator or freelancer' },
  ];

  return (
    <main style={{ minHeight: '100vh', display: 'grid', placeItems: 'center', padding: 24 }}>
      <div className="card" style={{ maxWidth: 520, width: '100%', padding: 40 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
          <div className="brand">
            <div className="brand-mark">A</div>
            <span className="brand-name">AutoFlow<span className="dim">.thg</span></span>
          </div>
          <LangSwitch />
        </div>

        <div className="eyebrow" style={{ marginBottom: 8 }}><span className="dot" />ONBOARDING</div>
        <h2 style={{ fontSize: 28, marginBottom: 8 }}>
          {lang === 'vi' ? <>Tạo <span className="title-mono">workspace của bạn.</span></> : <>Create <span className="title-mono">your workspace.</span></>}
        </h2>
        <p style={{ color: 'var(--text-mute)', marginBottom: 24, fontSize: 14 }}>
          {lang === 'vi' ? 'Chỉ mất 30 giây — bắt đầu ngay.' : 'Takes 30 seconds — start immediately.'}
        </p>

        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, marginBottom: 24 }}>
          {typeOptions.map(opt => {
            const Icon = opt.icon;
            const active = orgType === opt.value;
            return (
              <button
                key={opt.value}
                type="button"
                onClick={() => setOrgType(opt.value)}
                className="card"
                style={{
                  padding: 16,
                  textAlign: 'left',
                  cursor: 'pointer',
                  background: active ? 'var(--accent-soft)' : 'var(--bg-elev)',
                  borderColor: active ? 'var(--accent)' : 'var(--line)',
                  color: 'var(--text)',
                }}
              >
                <Icon size={20} style={{ color: active ? 'var(--accent)' : 'var(--text-faint)', marginBottom: 8 }} />
                <div style={{ fontSize: 13, fontWeight: 600 }}>{opt.label}</div>
                <div style={{ fontSize: 11, color: 'var(--text-faint)', marginTop: 3 }}>{opt.desc}</div>
              </button>
            );
          })}
        </div>

        <div className="auth-fields">
          <label className="field">
            <span className="field-label">
              {orgType === 'personal'
                ? (lang === 'vi' ? 'TÊN CỦA BẠN / THƯƠNG HIỆU' : 'YOUR NAME / BRAND')
                : (lang === 'vi' ? 'TÊN TỔ CHỨC' : 'WORKSPACE NAME')}
            </span>
            <input
              className="input"
              placeholder={orgType === 'personal' ? 'Nguyễn Văn A' : 'Công ty TNHH ABC'}
              value={orgName}
              onChange={e => setOrgName(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && handleSetup()}
            />
          </label>
          <label className="field">
            <span className="field-label">{lang === 'vi' ? 'WEBSITE / DOMAIN (TUỲ CHỌN)' : 'WEBSITE / DOMAIN (OPTIONAL)'}</span>
            <input
              className="input"
              placeholder="abc.vn"
              value={domain}
              onChange={e => setDomain(e.target.value)}
            />
          </label>
        </div>

        {error && <div className="auth-error" style={{ marginTop: 16 }}>{error}</div>}

        <button
          type="button"
          className="btn btn-primary btn-lg"
          style={{ width: '100%', justifyContent: 'center', marginTop: 24, opacity: loading ? 0.6 : 1 }}
          onClick={handleSetup}
          disabled={loading}
        >
          {loading
            ? (lang === 'vi' ? 'Đang tạo workspace…' : 'Creating workspace…')
            : (lang === 'vi' ? 'Tạo workspace' : 'Create workspace')}
          <ArrowRight size={14} />
        </button>
      </div>
    </main>
  );
}
