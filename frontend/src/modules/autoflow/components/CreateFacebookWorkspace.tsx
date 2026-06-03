'use client';
import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { ArrowLeft, ArrowRight, Building2, Check, Inbox, User } from 'lucide-react';
import type { AuthUser } from '../services/authService';
import { useAuthStore } from '../stores/authStore';
import { useLang } from '../i18n/useLang';
import { getMyPendingInvites, acceptInviteToken, type PendingInvite } from '../services/staffService';
import { facebookWorkspaceIdOf } from '../service';
import styles from '../../../platform/onboarding.module.css';

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

  // ---------- success ----------
  if (done && createdWorkspaceId) {
    return (
      <div className={styles.canvas}>
        <div className={styles.successWrap}>
          <div className={styles.successCard}>
            <div className={styles.successIcon}><Check size={30} /></div>
            <h2>{lang === 'vi' ? 'Workspace Facebook đã sẵn sàng' : 'Facebook workspace ready'}</h2>
            <p>
              {lang === 'vi'
                ? <>Workspace <strong>{orgName}</strong> đã được khởi tạo cho Facebook Automation.</>
                : <>Workspace <strong>{orgName}</strong> initialised for Facebook Automation.</>}
            </p>
            <button
              className={`${styles.btnPrimary} ${styles.btnBlock}`}
              onClick={() => navigateToWorkspace(createdWorkspaceId)}
            >
              {lang === 'vi' ? 'Vào workspace' : 'Open workspace'} <ArrowRight size={16} />
            </button>
          </div>
        </div>
      </div>
    );
  }

  // ---------- choice ----------
  if (view === 'choice') {
    return (
      <div className={styles.canvas}>
        <div className={`${styles.wrap} ${styles.narrow}`}>
          <button type="button" className={styles.back} onClick={() => router.push('/services')}>
            <ArrowLeft size={14} /> {lang === 'vi' ? 'Tất cả dịch vụ' : 'All services'}
          </button>

          <div className={styles.eyebrow} style={{ marginTop: 22 }}>
            <span className={styles.dot} />Facebook Automation
          </div>
          <h1 className={styles.h1}>
            {lang === 'vi'
              ? <>Khởi tạo workspace <span className={styles.mono}>Facebook</span></>
              : <>Initialise your <span className={styles.mono}>Facebook</span> workspace</>}
          </h1>
          <p className={styles.lead}>
            {lang === 'vi'
              ? 'Workspace là không gian vận hành riêng cho dịch vụ này — chứa tài khoản, khách hàng, phiên trình duyệt và đội ngũ. Bạn có thể tạo mới hoặc nhận lời mời từ đội có sẵn.'
              : 'A workspace is the operations space for this service — accounts, leads, browser sessions, and team. Create your own or accept an invite from an existing team.'}
          </p>

          {invites.length > 0 && (
            <div className={styles.invites}>
              <div className={styles.invitesHead}>
                <Inbox size={15} color="var(--accent)" />
                <span>
                  {lang === 'vi' ? `${invites.length} LỜI MỜI ĐANG CHỜ` : `${invites.length} PENDING INVITE${invites.length > 1 ? 'S' : ''}`}
                </span>
              </div>
              {invites.map(inv => (
                <div key={inv.id} className={styles.inviteRow}>
                  <div style={{ minWidth: 0 }}>
                    <b>{inv.org_name || `Workspace #${inv.org_id}`}</b>
                    <small>{lang === 'vi' ? `Vai trò: ${inv.role}` : `Role: ${inv.role}`}</small>
                  </div>
                  <button
                    type="button"
                    className={`${styles.btnPrimary} ${styles.btnSm}`}
                    onClick={() => void handleAcceptInvite(inv)}
                    disabled={acceptingId !== null}
                  >
                    {acceptingId === inv.id ? (lang === 'vi' ? 'Đang nhận…' : 'Accepting…') : (lang === 'vi' ? 'Nhận lời mời' : 'Accept')}
                    <ArrowRight size={13} />
                  </button>
                </div>
              ))}
            </div>
          )}

          <button type="button" className={styles.choice} onClick={() => setView('form')}>
            <span className={styles.choiceIcon}><Building2 size={22} /></span>
            <span style={{ flex: 1 }}>
              <b>{lang === 'vi' ? 'Tạo workspace Facebook mới' : 'Create a new Facebook workspace'}</b>
              <small>
                {lang === 'vi' ? 'Đặt tên, ngành, dịch vụ & tệp khách mục tiêu. Bạn sẽ là quản trị viên.' : 'Set name, industry, services & target audience. You become admin.'}
              </small>
            </span>
            <ArrowRight size={18} color="var(--text-faint)" />
          </button>

          {error && <div className={styles.error} style={{ marginTop: 14 }}>{error}</div>}
        </div>
      </div>
    );
  }

  // ---------- form ----------
  const typeOptions = [
    { value: 'team' as const, icon: Building2, label: lang === 'vi' ? 'Team / Doanh nghiệp' : 'Team / Business', desc: lang === 'vi' ? 'Nhiều thành viên, quản lý tập trung' : 'Multiple members, central admin' },
    { value: 'personal' as const, icon: User, label: lang === 'vi' ? 'Cá nhân' : 'Personal', desc: lang === 'vi' ? 'Dùng một mình hoặc freelancer' : 'Solo operator or freelancer' },
  ];

  return (
    <div className={styles.canvas}>
      <div className={`${styles.wrap} ${styles.narrow}`}>
        <button type="button" className={styles.back} onClick={() => setView('choice')}>
          <ArrowLeft size={14} /> {lang === 'vi' ? 'Quay lại' : 'Back'}
        </button>

        <div className={styles.steps}>
          <span className={styles.stepDot}><i>1</i>{lang === 'vi' ? 'Chọn loại' : 'Choose type'}</span>
          <span className={styles.stepLine} />
          <span className={`${styles.stepDot} ${styles.on}`}><i>2</i>{lang === 'vi' ? 'Định vị doanh nghiệp' : 'Position business'}</span>
        </div>

        <div className={styles.eyebrow} style={{ marginTop: 22 }}>
          <span className={styles.dot} />Facebook Automation
        </div>
        <h1 className={styles.h1}>
          {lang === 'vi' ? 'Định vị workspace để AI làm đúng việc của bạn' : 'Position your workspace so AI runs your playbook'}
        </h1>
        <p className={styles.lead}>
          {lang === 'vi'
            ? 'Càng rõ định vị, AI phân loại khách & nhắn tin càng đúng tệp. Bạn có thể chỉnh lại sau trong phần Dữ liệu.'
            : 'The clearer you are, the better the AI matches your target. Editable later in Data.'}
        </p>

        <div className={styles.typeGrid}>
          {typeOptions.map(opt => {
            const Icon = opt.icon;
            const active = orgType === opt.value;
            return (
              <button
                key={opt.value}
                type="button"
                onClick={() => setOrgType(opt.value)}
                className={`${styles.typeCard} ${active ? styles.active : ''}`}
              >
                <Icon size={20} />
                <b>{opt.label}</b>
                <small>{opt.desc}</small>
              </button>
            );
          })}
        </div>

        <div className={styles.formCard}>
          <div className={styles.fields}>
            <label className={styles.field}>
              <span className={styles.label}>
                {orgType === 'personal'
                  ? (lang === 'vi' ? 'Tên bạn / Thương hiệu' : 'Your name / Brand')
                  : (lang === 'vi' ? 'Tên workspace' : 'Workspace name')}
              </span>
              <input className={styles.input} placeholder={orgType === 'personal' ? 'Nguyễn Văn A' : 'Công ty TNHH ABC'} value={orgName} onChange={e => setOrgName(e.target.value)} />
            </label>
            <label className={styles.field}>
              <span className={styles.label}>{lang === 'vi' ? 'Ngành / Mô hình kinh doanh' : 'Industry / Business model'}</span>
              <input className={styles.input} placeholder={lang === 'vi' ? 'VD: POD, fulfillment, logistics, BĐS, tuyển dụng' : 'e.g. POD, fulfillment, logistics, real estate, recruitment'} value={businessIndustry} onChange={e => setBusinessIndustry(e.target.value)} />
            </label>
            <label className={styles.field}>
              <span className={styles.label}>{lang === 'vi' ? 'Sản phẩm / Dịch vụ' : 'Products / Services'}</span>
              <input className={styles.input} placeholder={lang === 'vi' ? 'Bạn cung cấp gì cho khách' : 'What you offer customers'} value={services} onChange={e => setServices(e.target.value)} />
            </label>
            <label className={styles.field}>
              <span className={styles.label}>{lang === 'vi' ? 'Tệp khách mục tiêu' : 'Target customers'}</span>
              <input className={styles.input} placeholder={lang === 'vi' ? 'Ai là người bạn muốn tìm trên Facebook' : 'Who you want to find on Facebook'} value={targetCustomers} onChange={e => setTargetCustomers(e.target.value)} />
            </label>
            <label className={styles.field}>
              <span className={styles.label}>{lang === 'vi' ? 'Mô tả tự do (tuỳ chọn)' : 'Free-form description (optional)'}</span>
              <textarea className={styles.input} rows={3} placeholder={lang === 'vi' ? 'Lợi thế, vùng phục vụ, điều cấm với automation…' : 'USP, region, automation guardrails…'} value={businessProfile} onChange={e => setBusinessProfile(e.target.value)} />
            </label>
            <label className={styles.field}>
              <span className={styles.label}>{lang === 'vi' ? 'Website / Domain (tuỳ chọn)' : 'Website / Domain (optional)'}</span>
              <input className={styles.input} placeholder="abc.vn" value={domain} onChange={e => setDomain(e.target.value)} />
            </label>
          </div>

          {error && <div className={styles.error} style={{ marginTop: 16 }}>{error}</div>}

          <button
            type="button"
            className={`${styles.btnPrimary} ${styles.btnBlock}`}
            style={{ marginTop: 18 }}
            onClick={handleSetup}
            disabled={loading}
          >
            {loading
              ? (lang === 'vi' ? 'Đang tạo workspace…' : 'Creating workspace…')
              : (lang === 'vi' ? 'Tạo workspace Facebook' : 'Create Facebook workspace')}
            <ArrowRight size={16} />
          </button>
        </div>
      </div>
    </div>
  );
}
