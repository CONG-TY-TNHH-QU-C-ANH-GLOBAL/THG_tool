'use client';
import { useEffect, useState } from 'react';
import { ArrowRight, Building2, Check, Inbox, LogOut, User } from 'lucide-react';
import { getMe, isPlatformRole, refreshToken } from '../services/authService';
import type { AuthUser } from '../services/authService';
import { useAuthStore } from '../stores/authStore';
import { LangSwitch } from './ds/LangSwitch';
import { useLang } from '../i18n/useLang';
import { getMyPendingInvites, acceptInviteToken, type PendingInvite } from '../services/staffService';

interface OnboardingProps {
  onComplete: (role: 'admin' | 'staff' | 'founder' | 'superadmin') => void;
  onSignOut?: () => void;
}

type View = 'choice' | 'form';

function routeRoleFor(user?: Partial<AuthUser> | null): 'admin' | 'staff' | 'founder' | 'superadmin' {
  if (isPlatformRole(user?.role)) return 'founder';
  return user?.role === 'admin' ? 'admin' : 'staff';
}

export default function Onboarding({ onComplete, onSignOut }: OnboardingProps) {
  const { lang } = useLang();
  const [view, setView] = useState<View>('choice');
  const [orgType, setOrgType] = useState<'team' | 'personal'>('team');
  const [orgName, setOrgName] = useState('');
  const [domain, setDomain] = useState('');
  const [businessIndustry, setBusinessIndustry] = useState('');
  const [services, setServices] = useState('');
  const [targetCustomers, setTargetCustomers] = useState('');
  const [businessProfile, setBusinessProfile] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [done, setDone] = useState(false);
  const [invites, setInvites] = useState<PendingInvite[]>([]);
  const [acceptingId, setAcceptingId] = useState<number | null>(null);

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

  useEffect(() => {
    let cancelled = false;
    getMyPendingInvites()
      .then(list => { if (!cancelled) setInvites(list); })
      .catch(() => {});
    return () => { cancelled = true; };
  }, []);

  async function handleAcceptInvite(invite: PendingInvite) {
    setAcceptingId(invite.id);
    setError('');
    try {
      const data = await acceptInviteToken(invite.token);
      useAuthStore.getState().setAuth(data.access_token, data.user as AuthUser);
      onComplete(routeRoleFor(data.user as AuthUser));
    } catch (err) {
      setError(err instanceof Error ? err.message : (lang === 'vi' ? 'Không nhận được invite.' : 'Could not accept invite.'));
    } finally {
      setAcceptingId(null);
    }
  }

  async function handleSetup() {
    setError('');
    if (!orgName.trim()) { setError(lang === 'vi' ? 'Vui lòng nhập tên tổ chức' : 'Please enter the workspace name'); return; }
    setLoading(true);
    try {
      const api = await import('../services/api');
      const data = await api.post<{ access_token: string; user: AuthUser }>(
        '/onboarding/setup',
        {
          org_name: orgName.trim(),
          domain: domain.trim(),
          type: orgType,
          business_industry: businessIndustry.trim(),
          services: services.trim(),
          target_customers: targetCustomers.trim(),
          business_profile: businessProfile.trim(),
        },
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

  if (view === 'choice') {
    return (
      <main style={{ minHeight: '100vh', display: 'grid', placeItems: 'center', padding: 24 }}>
        <div className="card" style={{ maxWidth: 560, width: '100%', padding: 36 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 18 }}>
            <div className="brand">
              <div className="brand-mark">A</div>
              <span className="brand-name">AutoFlow<span className="dim">.thg</span></span>
            </div>
            <LangSwitch />
          </div>

          <div className="eyebrow" style={{ marginBottom: 8 }}><span className="dot" />{lang === 'vi' ? 'BẮT ĐẦU' : 'GET STARTED'}</div>
          <h2 style={{ fontSize: 26, marginBottom: 6 }}>
            {lang === 'vi' ? <>Bạn đang là một <span className="title-mono">tài khoản trong hệ thống.</span></> : <>You're a <span className="title-mono">platform account.</span></>}
          </h2>
          <p style={{ color: 'var(--text-mute)', marginBottom: 18, fontSize: 13.5 }}>
            {lang === 'vi'
              ? 'Workspace là không gian làm việc riêng có Facebook automation, leads, và team. Bạn có thể tạo của riêng hoặc tham gia workspace bạn được mời.'
              : 'A workspace is your private operations layer for Facebook automation, leads, and team. You can create your own or join one you were invited to.'}
          </p>

          {invites.length > 0 && (
            <div style={{ background: 'var(--accent-soft)', border: '1px solid var(--accent)', borderRadius: 10, padding: 16, marginBottom: 18 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 7, marginBottom: 10 }}>
                <Inbox size={14} color="var(--accent)" />
                <span style={{ fontSize: 12, fontWeight: 700, color: 'var(--accent)' }}>
                  {lang === 'vi' ? `${invites.length} LỜI MỜI ĐANG CHỜ` : `${invites.length} PENDING INVITE${invites.length > 1 ? 'S' : ''}`}
                </span>
              </div>
              {invites.map(inv => (
                <div key={inv.id} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 12, padding: '8px 0', borderTop: '1px solid var(--line-strong)' }}>
                  <div style={{ minWidth: 0 }}>
                    <div style={{ fontSize: 13, color: 'var(--text)', fontWeight: 600 }}>{inv.org_name || `Workspace #${inv.org_id}`}</div>
                    <div style={{ fontSize: 11, color: 'var(--text-faint)' }}>{lang === 'vi' ? `Vai trò: ${inv.role}` : `Role: ${inv.role}`}</div>
                  </div>
                  <button
                    type="button"
                    className="btn btn-primary btn-sm"
                    onClick={() => void handleAcceptInvite(inv)}
                    disabled={acceptingId !== null}
                  >
                    {acceptingId === inv.id ? (lang === 'vi' ? 'Đang nhận…' : 'Accepting…') : (lang === 'vi' ? 'Nhận invite' : 'Accept')}
                    <ArrowRight size={12} />
                  </button>
                </div>
              ))}
            </div>
          )}

          <div style={{ display: 'grid', gridTemplateColumns: '1fr', gap: 12, marginBottom: 14 }}>
            <button
              type="button"
              className="card"
              onClick={() => setView('form')}
              style={{ padding: 18, textAlign: 'left', cursor: 'pointer', background: 'var(--bg-elev)', display: 'flex', alignItems: 'center', gap: 14, color: 'var(--text)' }}
            >
              <Building2 size={22} color="var(--accent)" />
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: 14, fontWeight: 600 }}>{lang === 'vi' ? 'Tạo workspace mới' : 'Create a new workspace'}</div>
                <div style={{ fontSize: 12, color: 'var(--text-faint)', marginTop: 3 }}>
                  {lang === 'vi' ? 'Đặt tên, ngành, dịch vụ + tệp khách mục tiêu. Bạn sẽ là admin.' : 'Set name, industry, services & target audience. You become admin.'}
                </div>
              </div>
              <ArrowRight size={16} color="var(--text-faint)" />
            </button>
          </div>

          {error && <div className="auth-error" style={{ marginBottom: 12 }}>{error}</div>}

          {onSignOut && (
            <button
              type="button"
              className="btn btn-ghost btn-sm"
              style={{ width: '100%', justifyContent: 'center' }}
              onClick={onSignOut}
            >
              <LogOut size={12} /> {lang === 'vi' ? 'Tạo sau — đăng xuất' : 'Decide later — sign out'}
            </button>
          )}
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
      <div className="card" style={{ maxWidth: 580, width: '100%', padding: 36 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 18 }}>
          <button type="button" className="auth-back" onClick={() => setView('choice')}>
            ← {lang === 'vi' ? 'Quay lại' : 'Back'}
          </button>
          <LangSwitch />
        </div>

        <div className="eyebrow" style={{ marginBottom: 8 }}><span className="dot" />{lang === 'vi' ? 'TẠO WORKSPACE' : 'CREATE WORKSPACE'}</div>
        <h2 style={{ fontSize: 24, marginBottom: 6 }}>
          {lang === 'vi' ? 'Định vị workspace để AI làm đúng việc của bạn' : 'Position your workspace so AI runs your playbook'}
        </h2>
        <p style={{ color: 'var(--text-mute)', marginBottom: 18, fontSize: 13 }}>
          {lang === 'vi'
            ? 'Càng rõ định vị, classifier + comment + outbound càng đúng tệp. Có thể chỉnh lại sau ở Data Private.'
            : 'The clearer you are, the better classifier + outbound match your target. Editable later in Data Private.'}
        </p>

        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10, marginBottom: 16 }}>
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
                  padding: 14,
                  textAlign: 'left',
                  cursor: 'pointer',
                  background: active ? 'var(--accent-soft)' : 'var(--bg-elev)',
                  borderColor: active ? 'var(--accent)' : 'var(--line)',
                  color: 'var(--text)',
                }}
              >
                <Icon size={18} style={{ color: active ? 'var(--accent)' : 'var(--text-faint)', marginBottom: 6 }} />
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
                ? (lang === 'vi' ? 'TÊN BẠN / THƯƠNG HIỆU' : 'YOUR NAME / BRAND')
                : (lang === 'vi' ? 'TÊN TỔ CHỨC' : 'WORKSPACE NAME')}
            </span>
            <input className="input" placeholder={orgType === 'personal' ? 'Nguyễn Văn A' : 'Công ty TNHH ABC'} value={orgName} onChange={e => setOrgName(e.target.value)} />
          </label>
          <label className="field">
            <span className="field-label">{lang === 'vi' ? 'NGÀNH / MÔ HÌNH KINH DOANH' : 'INDUSTRY / BUSINESS MODEL'}</span>
            <input className="input" placeholder={lang === 'vi' ? 'VD: POD, fulfillment, logistics, BĐS, recruitment' : 'e.g. POD, fulfillment, logistics, real estate, recruitment'} value={businessIndustry} onChange={e => setBusinessIndustry(e.target.value)} />
          </label>
          <label className="field">
            <span className="field-label">{lang === 'vi' ? 'SẢN PHẨM / DỊCH VỤ' : 'PRODUCTS / SERVICES'}</span>
            <input className="input" placeholder={lang === 'vi' ? 'Bạn cung cấp gì cho khách' : 'What you offer customers'} value={services} onChange={e => setServices(e.target.value)} />
          </label>
          <label className="field">
            <span className="field-label">{lang === 'vi' ? 'TỆP KHÁCH MỤC TIÊU' : 'TARGET CUSTOMERS'}</span>
            <input className="input" placeholder={lang === 'vi' ? 'Ai là người bạn muốn tìm trên Facebook' : 'Who you want to find on Facebook'} value={targetCustomers} onChange={e => setTargetCustomers(e.target.value)} />
          </label>
          <label className="field">
            <span className="field-label">{lang === 'vi' ? 'MÔ TẢ TỰ DO (TUỲ CHỌN)' : 'FREE-FORM DESCRIPTION (OPTIONAL)'}</span>
            <textarea className="input" rows={3} placeholder={lang === 'vi' ? 'USP, vùng phục vụ, điều cấm với automation…' : 'USP, region, automation guardrails…'} value={businessProfile} onChange={e => setBusinessProfile(e.target.value)} />
          </label>
          <label className="field">
            <span className="field-label">{lang === 'vi' ? 'WEBSITE / DOMAIN (TUỲ CHỌN)' : 'WEBSITE / DOMAIN (OPTIONAL)'}</span>
            <input className="input" placeholder="abc.vn" value={domain} onChange={e => setDomain(e.target.value)} />
          </label>
        </div>

        {error && <div className="auth-error" style={{ marginTop: 14 }}>{error}</div>}

        <button
          type="button"
          className="btn btn-primary btn-lg"
          style={{ width: '100%', justifyContent: 'center', marginTop: 18, opacity: loading ? 0.6 : 1 }}
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
