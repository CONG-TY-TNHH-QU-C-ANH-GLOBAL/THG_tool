'use client';
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { ArrowRight, Building2, Check, Inbox, User } from 'lucide-react';
import type { AuthUser } from '../services/authService';
import { useAuthStore } from '../stores/authStore';
import { useLang } from '../i18n/useLang';
import { getMyPendingInvites, acceptInviteToken, type PendingInvite } from '../services/staffService';
import { facebookWorkspaceIdOf } from '../service';

type View = 'choice' | 'form';

export default function CreateFacebookWorkspace() {
  const router = useRouter();
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
  const [createdWorkspaceId, setCreatedWorkspaceId] = useState<string | null>(null);
  const [invites, setInvites] = useState<PendingInvite[]>([]);
  const [acceptingId, setAcceptingId] = useState<number | null>(null);

  useEffect(() => {
    let cancelled = false;
    getMyPendingInvites()
      .then(list => { if (!cancelled) setInvites(list); })
      .catch(() => {});
    return () => { cancelled = true; };
  }, []);

  function navigateToWorkspace(workspaceId: string) {
    router.push(`/services/facebook/workspaces/${workspaceId}`);
  }

  async function handleAcceptInvite(invite: PendingInvite) {
    setAcceptingId(invite.id);
    setError('');
    try {
      const data = await acceptInviteToken(invite.token);
      useAuthStore.getState().setAuth(data.access_token, data.user as AuthUser);
      const workspaceId = facebookWorkspaceIdOf((data.user as AuthUser).org_id);
      if (workspaceId) navigateToWorkspace(workspaceId);
      else router.push('/services');
    } catch (err) {
      setError(err instanceof Error ? err.message : (lang === 'vi' ? 'Không nhận được invite.' : 'Could not accept invite.'));
    } finally {
      setAcceptingId(null);
    }
  }

  async function handleSetup() {
    setError('');
    if (!orgName.trim()) { setError(lang === 'vi' ? 'Vui lòng nhập tên workspace' : 'Please enter the workspace name'); return; }
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
      useAuthStore.getState().setAuth(data.access_token, data.user);
      const workspaceId = facebookWorkspaceIdOf(data.user.org_id);
      setCreatedWorkspaceId(workspaceId ?? null);
      setDone(true);
    } catch (err) {
      setError(err instanceof Error && err.message ? err.message : (lang === 'vi' ? 'Lỗi kết nối, thử lại sau' : 'Connection error, try again'));
    } finally {
      setLoading(false);
    }
  }

  if (done && createdWorkspaceId) {
    return (
      <div style={{ display: 'grid', placeItems: 'center', padding: 24, minHeight: '100%' }}>
        <div className="card" style={{ maxWidth: 440, textAlign: 'center', padding: 40 }}>
          <div className="auth-success-icon"><Check size={28} /></div>
          <h2 style={{ fontSize: 24, marginBottom: 8 }}>
            {lang === 'vi' ? 'Workspace Facebook đã sẵn sàng' : 'Facebook workspace ready'}
          </h2>
          <p style={{ color: 'var(--text-mute)', marginBottom: 28 }}>
            {lang === 'vi'
              ? <>Workspace <strong style={{ color: 'var(--text)' }}>{orgName}</strong> đã được khởi tạo cho Facebook Automation.</>
              : <>Workspace <strong style={{ color: 'var(--text)' }}>{orgName}</strong> initialised for Facebook Automation.</>}
          </p>
          <button
            className="btn btn-primary btn-lg"
            style={{ width: '100%', justifyContent: 'center' }}
            onClick={() => navigateToWorkspace(createdWorkspaceId)}
          >
            {lang === 'vi' ? 'Vào workspace' : 'Open workspace'} <ArrowRight size={14} />
          </button>
        </div>
      </div>
    );
  }

  if (view === 'choice') {
    return (
      <div style={{ display: 'grid', placeItems: 'center', padding: 24, minHeight: '100%' }}>
        <div className="card" style={{ maxWidth: 560, width: '100%', padding: 36 }}>
          <div className="eyebrow" style={{ marginBottom: 8 }}>
            <span className="dot" />{lang === 'vi' ? 'KHỞI TẠO FACEBOOK AUTOMATION' : 'INITIALISE FACEBOOK AUTOMATION'}
          </div>
          <h2 style={{ fontSize: 26, marginBottom: 6 }}>
            {lang === 'vi'
              ? <>Tạo workspace Facebook cho <span className="title-mono">đội của bạn.</span></>
              : <>Create a Facebook workspace for <span className="title-mono">your team.</span></>}
          </h2>
          <p style={{ color: 'var(--text-mute)', marginBottom: 18, fontSize: 13.5 }}>
            {lang === 'vi'
              ? 'Workspace là không gian vận hành Facebook automation — chứa account, leads, browser session, và team. Bạn có thể tạo mới hoặc tham gia workspace bạn được mời.'
              : 'A workspace is your Facebook automation operations layer — accounts, leads, browser sessions, and team. Create your own or join one you were invited to.'}
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
                <div style={{ fontSize: 14, fontWeight: 600 }}>{lang === 'vi' ? 'Tạo workspace Facebook mới' : 'Create a new Facebook workspace'}</div>
                <div style={{ fontSize: 12, color: 'var(--text-faint)', marginTop: 3 }}>
                  {lang === 'vi' ? 'Đặt tên, ngành, dịch vụ + tệp khách mục tiêu. Bạn sẽ là admin.' : 'Set name, industry, services & target audience. You become admin.'}
                </div>
              </div>
              <ArrowRight size={16} color="var(--text-faint)" />
            </button>
          </div>

          {error && <div className="auth-error" style={{ marginBottom: 12 }}>{error}</div>}
        </div>
      </div>
    );
  }

  const typeOptions = [
    { value: 'team' as const, icon: Building2, label: lang === 'vi' ? 'Team / Doanh nghiệp' : 'Team / Business', desc: lang === 'vi' ? 'Nhiều thành viên, quản lý tập trung' : 'Multiple members, central admin' },
    { value: 'personal' as const, icon: User, label: lang === 'vi' ? 'Cá nhân' : 'Personal', desc: lang === 'vi' ? 'Sử dụng một mình hoặc freelancer' : 'Solo operator or freelancer' },
  ];

  return (
    <div style={{ display: 'grid', placeItems: 'center', padding: 24, minHeight: '100%' }}>
      <div className="card" style={{ maxWidth: 580, width: '100%', padding: 36 }}>
        <div style={{ marginBottom: 18 }}>
          <button type="button" className="auth-back" onClick={() => setView('choice')}>
            ← {lang === 'vi' ? 'Quay lại' : 'Back'}
          </button>
        </div>

        <div className="eyebrow" style={{ marginBottom: 8 }}>
          <span className="dot" />{lang === 'vi' ? 'TẠO WORKSPACE FACEBOOK' : 'CREATE FACEBOOK WORKSPACE'}
        </div>
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
                : (lang === 'vi' ? 'TÊN WORKSPACE' : 'WORKSPACE NAME')}
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
            : (lang === 'vi' ? 'Tạo workspace Facebook' : 'Create Facebook workspace')}
          <ArrowRight size={14} />
        </button>
      </div>
    </div>
  );
}
