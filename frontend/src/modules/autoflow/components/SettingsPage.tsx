import { useEffect, useRef, useState } from 'react';
import type { ComponentType, CSSProperties } from 'react';
import type { Organization } from '../types';
import { Avatar, Badge, Row } from './ui';
import { theme, cardStyle, primaryBtn, secondaryBtn, inputStyle as baseInputStyle } from '../constants/styles';
import { useStaff } from '../hooks/useStaff';
import { changePassword } from '../services/authService';
import { searchInviteCandidates, type InviteCandidate } from '../services/staffService';
import {
  AgentToken,
  AuditLog,
  BillingSummary,
  createAgentToken,
  getAgentTokens,
  getAuditLogs,
  getBillingSummary,
  getOrgBrand,
  updateOrgBrand,
  uploadOrgAsset,
  revokeAgentToken,
} from '../services/settingsService';
import {
  AlertTriangle,
  Check,
  Copy,
  CreditCard,
  KeyRound,
  Mail,
  Palette,
  RefreshCw,
  Shield,
  Upload,
  UserPlus,
  Users,
  X,
  Zap,
} from 'lucide-react';

interface SettingsPageProps { org: Organization; orgId: string; isAdmin: boolean; }

type SettingsTab = 'brand' | 'security' | 'staff' | 'agents' | 'billing';

const inputStyle: CSSProperties = baseInputStyle;

const Label = ({ text }: { text: string }) => (
  <p style={{ color: theme.textFaint, fontSize: 12, marginBottom: 5 }}>{text}</p>
);

const TABS: { id: SettingsTab; label: string; Icon: ComponentType<{ size?: number | string }> }[] = [
  { id: 'brand', label: 'Thương hiệu', Icon: Palette },
  { id: 'security', label: 'Bảo mật', Icon: Shield },
  { id: 'staff', label: 'Nhân viên', Icon: Users },
  { id: 'agents', label: 'AI Agents', Icon: Zap },
  { id: 'billing', label: 'Thanh toán', Icon: CreditCard },
];

function formatDate(value?: string | null) {
  if (!value) return 'Chưa kết nối';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString('vi-VN', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
}

function usagePercent(current: number, max: number) {
  if (!max || max < 0) return 0;
  return Math.min(Math.round((current / max) * 100), 100);
}

export default function SettingsPage({ org, orgId, isAdmin }: SettingsPageProps) {
  const [activeTab, setActiveTab] = useState<SettingsTab>('brand');
  const { staff, invites, isLoading, invite, resendInvite, revokeInvite, toggleStatus, remove } = useStaff(orgId);
  const [showAdd, setShowAdd] = useState(false);
  const [newStaff, setNewStaff] = useState({ email: '', role: 'sales' });
  const [staffMsg, setStaffMsg] = useState('');
  const [inviteCandidates, setInviteCandidates] = useState<InviteCandidate[]>([]);
  const [showCandidates, setShowCandidates] = useState(false);
  const inviteSearchTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const [brandName, setBrandName] = useState(org.name || '');
  const [brandDomain, setBrandDomain] = useState('');
  const [abbr, setAbbr] = useState(org.abbr || 'ORG');
  const [color, setColor] = useState(org.color || theme.primary);
  const [logoUrl, setLogoUrl] = useState(org.logo_url || '');
  const [avatarUrl, setAvatarUrl] = useState(org.avatar_url || '');
  const [planTier, setPlanTier] = useState<string>(org.plan || 'Starter');
  const [maxAccounts, setMaxAccounts] = useState(1);
  const [orgSaving, setOrgSaving] = useState(false);
  const [orgMsg, setOrgMsg] = useState('');

  const [pw, setPw] = useState({ current: '', next: '', confirm: '' });
  const [pwMsg, setPwMsg] = useState('');
  const [auditLogs, setAuditLogs] = useState<AuditLog[]>([]);

  const [agents, setAgents] = useState<AgentToken[]>([]);
  const [newAgentName, setNewAgentName] = useState('');
  const [newAgentToken, setNewAgentToken] = useState('');
  const [agentMsg, setAgentMsg] = useState('');

  const [billing, setBilling] = useState<BillingSummary | null>(null);

  const logoInputRef = useRef<HTMLInputElement>(null);
  const avatarInputRef = useRef<HTMLInputElement>(null);

  const refreshAdminData = async () => {
    if (!isAdmin) return;
    const [tokens, logs, bill] = await Promise.allSettled([
      getAgentTokens(),
      getAuditLogs(),
      getBillingSummary(),
    ]);
    if (tokens.status === 'fulfilled') setAgents(tokens.value);
    if (logs.status === 'fulfilled') setAuditLogs(logs.value);
    if (bill.status === 'fulfilled') setBilling(bill.value);
  };

  useEffect(() => {
    getOrgBrand()
      .then(data => {
        if (!data) return;
        setBrandName(data.name || org.name || '');
        setBrandDomain(data.domain || '');
        setAbbr(data.abbr || org.abbr || 'ORG');
        setColor(data.color || org.color || theme.primary);
        setLogoUrl(data.logo_url || '');
        setAvatarUrl(data.avatar_url || '');
        setPlanTier(data.plan_tier || org.plan || 'Starter');
        setMaxAccounts(data.max_accounts || 1);
      })
      .catch(() => {});
    refreshAdminData();
  }, [orgId, isAdmin]);

  const saveBrand = async () => {
    if (!isAdmin) return;
    setOrgSaving(true);
    setOrgMsg('');
    try {
      const saved = await updateOrgBrand({ name: brandName.trim(), domain: brandDomain.trim(), abbr, color });
      setBrandName(saved.name);
      setBrandDomain(saved.domain || '');
      setAbbr(saved.abbr || abbr);
      setColor(saved.color || color);
      setOrgMsg('Đã lưu nhận diện thương hiệu.');
    } catch (err) {
      setOrgMsg(err instanceof Error ? err.message : 'Không lưu được thương hiệu.');
    } finally {
      setOrgSaving(false);
    }
  };

  const uploadAsset = async (kind: 'logo' | 'avatar', file?: File) => {
    if (!file || !isAdmin) return;
    setOrgMsg('');
    try {
      const url = await uploadOrgAsset(kind, file);
      const freshUrl = `${url}${url.includes('?') ? '&' : '?'}t=${Date.now()}`;
      if (kind === 'logo') setLogoUrl(freshUrl);
      else setAvatarUrl(freshUrl);
      setOrgMsg(kind === 'logo' ? 'Đã upload logo.' : 'Đã upload avatar.');
    } catch (err) {
      setOrgMsg(err instanceof Error ? err.message : 'Upload thất bại.');
    }
  };

  const savePassword = async () => {
    setPwMsg('');
    if (!pw.current || !pw.next || pw.next !== pw.confirm) {
      setPwMsg('Kiểm tra lại mật khẩu hiện tại và phần xác nhận.');
      return;
    }
    try {
      await changePassword(pw.current, pw.next);
      setPw({ current: '', next: '', confirm: '' });
      setPwMsg('Đã cập nhật mật khẩu.');
      if (isAdmin) getAuditLogs().then(setAuditLogs).catch(() => {});
    } catch (err) {
      setPwMsg(err instanceof Error ? err.message : 'Không đổi được mật khẩu.');
    }
  };

  const handleInviteEmailChange = (value: string) => {
    setNewStaff(p => ({ ...p, email: value }));
    if (inviteSearchTimer.current) clearTimeout(inviteSearchTimer.current);
    if (value.trim().length < 2) {
      setInviteCandidates([]);
      setShowCandidates(false);
      return;
    }
    inviteSearchTimer.current = setTimeout(async () => {
      try {
        const results = await searchInviteCandidates(value);
        setInviteCandidates(results);
        setShowCandidates(results.length > 0);
      } catch {
        setInviteCandidates([]);
        setShowCandidates(false);
      }
    }, 200);
  };

  const pickInviteCandidate = (candidate: InviteCandidate) => {
    setNewStaff({ email: candidate.email, role: candidate.role === 'admin' ? 'admin' : 'sales' });
    setShowCandidates(false);
    setInviteCandidates([]);
  };

  const handleInviteStaff = async () => {
    setStaffMsg('');
    if (!isAdmin) return;
    if (!newStaff.email.trim()) {
      setStaffMsg('Nhap email nhan vien de gui invite.');
      return;
    }
    try {
      const pending = await invite({ ...newStaff, email: newStaff.email.trim() });
      const inviteUrl = pending.inviteFullUrl || `${window.location.origin}${pending.inviteUrl}`;
      await navigator.clipboard?.writeText(inviteUrl).catch(() => {});
      setNewStaff({ email: '', role: 'sales' });
      setShowAdd(false);
      if (pending.emailStatus === 'sent') {
        setStaffMsg(`Da gui email invite cho ${pending.email}. Link cung da duoc copy.`);
      } else if (pending.emailStatus === 'failed') {
        setStaffMsg(`Invite da tao nhung email gui that bai. Link da duoc copy de gui thu cong.`);
      } else {
        setStaffMsg(`Invite da tao. SMTP chua cau hinh nen link da duoc copy de gui thu cong.`);
      }
    } catch (err) {
      setStaffMsg(err instanceof Error ? err.message : 'Khong gui duoc invite.');
    }
  };

  const addAgent = async () => {
    setAgentMsg('');
    if (!isAdmin || !newAgentName.trim()) return;
    try {
      const created = await createAgentToken(newAgentName.trim());
      setNewAgentToken(created.token);
      setNewAgentName('');
      setAgents(await getAgentTokens());
    } catch (err) {
      setAgentMsg(err instanceof Error ? err.message : 'Không tạo được agent token.');
    }
  };

  const revokeAgent = async (id: number) => {
    setAgentMsg('');
    try {
      await revokeAgentToken(id);
      setAgents(prev => prev.map(a => a.id === id ? { ...a, active: false } : a));
    } catch (err) {
      setAgentMsg(err instanceof Error ? err.message : 'Không thu hồi được token.');
    }
  };

  const outboxCounts = billing?.outbox_counts ?? {};
  const usageRows = [
    { label: 'Facebook accounts', current: billing?.account_count ?? 0, max: billing?.max_accounts ?? maxAccounts },
    { label: 'Nhân viên', current: billing?.staff_count ?? staff.length, max: Math.max((billing?.max_accounts ?? maxAccounts) * 5, staff.length || 1) },
    { label: 'Outbound đã gửi', current: outboxCounts.sent ?? 0, max: Math.max((outboxCounts.sent ?? 0) + (outboxCounts.draft ?? 0) + (outboxCounts.approved ?? 0), 1) },
  ];

  return (
    <div>
      <div style={{ display: 'flex', gap: 6, marginBottom: 22, flexWrap: 'wrap' }}>
        {TABS.map(({ id, label, Icon }) => (
          <button
            key={id}
            onClick={() => setActiveTab(id)}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              padding: '7px 13px',
              borderRadius: 9,
              border: 'none',
              cursor: 'pointer',
              fontSize: 12,
              background: activeTab === id ? theme.primary : theme.surface,
              color: activeTab === id ? '#fff' : theme.textMuted,
            }}
          >
            <Icon size={12} />{label}
          </button>
        ))}
      </div>

      {activeTab === 'brand' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          <div style={cardStyle()}>
            <p style={{ color: theme.text, fontWeight: 600, fontSize: 13, marginBottom: 18 }}>Nhận diện workspace</p>
            <div style={{ display: 'grid', gridTemplateColumns: '160px 1fr', gap: 22, alignItems: 'start' }}>
              <div>
                <div style={{ width: 92, height: 92, background: color, borderRadius: 18, display: 'flex', alignItems: 'center', justifyContent: 'center', color: '#fff', fontSize: 28, fontWeight: 900, marginBottom: 10, overflow: 'hidden', border: `3px solid ${color}55` }}>
                  {avatarUrl ? <img src={avatarUrl} alt="Org avatar" style={{ width: '100%', height: '100%', objectFit: 'cover' }} /> : abbr}
                </div>
                <input ref={avatarInputRef} type="file" accept="image/png,image/jpeg,image/webp,image/svg+xml" style={{ display: 'none' }} onChange={e => uploadAsset('avatar', e.target.files?.[0])} />
                <button disabled={!isAdmin} onClick={() => avatarInputRef.current?.click()} style={secondaryBtn({ padding: '6px 12px', fontSize: 12, opacity: isAdmin ? 1 : 0.55 })}>
                  <Upload size={12} /> Avatar
                </button>
              </div>

              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                <div><Label text="Tên tổ chức" /><input style={inputStyle} disabled={!isAdmin} value={brandName} onChange={e => setBrandName(e.target.value)} /></div>
                <div><Label text="Domain" /><input style={inputStyle} disabled={!isAdmin} value={brandDomain} onChange={e => setBrandDomain(e.target.value)} placeholder="company.vn" /></div>
                <div><Label text="Viết tắt" /><input style={inputStyle} disabled={!isAdmin} value={abbr} onChange={e => setAbbr(e.target.value.slice(0, 4).toUpperCase())} /></div>
                <div>
                  <Label text="Màu thương hiệu" />
                  <Row style={{ gap: 8 }}>
                    <input disabled={!isAdmin} type="color" value={color} onChange={e => setColor(e.target.value)} style={{ width: 42, height: 37, border: 'none', background: 'transparent', cursor: isAdmin ? 'pointer' : 'default' }} />
                    <input style={{ ...inputStyle, flex: 1 }} disabled={!isAdmin} value={color} onChange={e => setColor(e.target.value)} />
                  </Row>
                </div>
              </div>
            </div>
          </div>

          <div style={cardStyle()}>
            <Row style={{ justifyContent: 'space-between', gap: 18, alignItems: 'center' }}>
              <div>
                <p style={{ color: theme.text, fontWeight: 600, fontSize: 13, marginBottom: 6 }}>Logo workspace</p>
                <p style={{ color: theme.textFaint, fontSize: 12 }}>Ảnh nhận diện riêng của tổ chức.</p>
              </div>
              <div style={{ minWidth: 220, height: 72, border: `1px dashed ${theme.border}`, borderRadius: 10, display: 'flex', alignItems: 'center', justifyContent: 'center', overflow: 'hidden', background: theme.surfaceAlt }}>
                {logoUrl ? <img src={logoUrl} alt="Org logo" style={{ maxWidth: '100%', maxHeight: '100%', objectFit: 'contain' }} /> : <span style={{ color: theme.textFaint, fontSize: 12 }}>Chưa có logo</span>}
              </div>
              <input ref={logoInputRef} type="file" accept="image/png,image/jpeg,image/webp,image/svg+xml" style={{ display: 'none' }} onChange={e => uploadAsset('logo', e.target.files?.[0])} />
              <button disabled={!isAdmin} onClick={() => logoInputRef.current?.click()} style={secondaryBtn({ padding: '8px 14px', fontSize: 12, opacity: isAdmin ? 1 : 0.55 })}>
                <Upload size={13} /> Chọn file
              </button>
            </Row>
          </div>

          <Row style={{ gap: 10, justifyContent: 'flex-end' }}>
            {orgMsg && <span style={{ color: orgMsg.includes('Không') || orgMsg.includes('thất bại') ? '#fca5a5' : '#4ade80', fontSize: 12 }}>{orgMsg}</span>}
            <button disabled={!isAdmin || orgSaving} onClick={saveBrand} style={primaryBtn({ padding: '10px 24px', opacity: isAdmin && !orgSaving ? 1 : 0.55 })}>
              {orgSaving ? 'Đang lưu...' : 'Lưu thay đổi'}
            </button>
          </Row>
        </div>
      )}

      {activeTab === 'security' && (
        <div style={{ display: 'grid', gridTemplateColumns: 'minmax(320px, 480px) 1fr', gap: 14 }}>
          <div style={cardStyle()}>
            <Row style={{ gap: 9, marginBottom: 18 }}>
              <KeyRound size={16} color={theme.primaryLight} />
              <p style={{ color: theme.text, fontWeight: 600, fontSize: 13 }}>Đổi mật khẩu</p>
            </Row>
            <Label text="Mật khẩu hiện tại" />
            <input type="password" style={{ ...inputStyle, marginBottom: 12 }} value={pw.current} onChange={e => setPw(p => ({ ...p, current: e.target.value }))} />
            <Label text="Mật khẩu mới" />
            <input type="password" style={{ ...inputStyle, marginBottom: 12 }} value={pw.next} onChange={e => setPw(p => ({ ...p, next: e.target.value }))} />
            <Label text="Xác nhận mật khẩu mới" />
            <input type="password" style={{ ...inputStyle, marginBottom: 16 }} value={pw.confirm} onChange={e => setPw(p => ({ ...p, confirm: e.target.value }))} />
            <Row style={{ justifyContent: 'space-between', gap: 10 }}>
              <button onClick={savePassword} style={primaryBtn({ padding: '9px 18px', fontSize: 13 })}>Cập nhật</button>
              {pwMsg && <span style={{ color: pwMsg.startsWith('Đã') ? '#4ade80' : '#fca5a5', fontSize: 12 }}>{pwMsg}</span>}
            </Row>
          </div>

          <div style={cardStyle()}>
            <Row style={{ justifyContent: 'space-between', marginBottom: 14 }}>
              <p style={{ color: theme.text, fontWeight: 600, fontSize: 13 }}>Audit bảo mật</p>
              {isAdmin && (
                <button onClick={refreshAdminData} style={secondaryBtn({ padding: '6px 10px', fontSize: 11 })}>
                  <RefreshCw size={12} /> Làm mới
                </button>
              )}
            </Row>
            {!isAdmin ? (
              <Row style={{ gap: 8, color: theme.textMuted, fontSize: 13 }}>
                <AlertTriangle size={15} color={theme.yellow} />
                Chỉ admin workspace xem được audit log.
              </Row>
            ) : auditLogs.length === 0 ? (
              <p style={{ color: theme.textMuted, fontSize: 13 }}>Chưa có sự kiện bảo mật.</p>
            ) : (
              auditLogs.slice(0, 8).map(log => (
                <div key={log.id} style={{ padding: '9px 0', borderBottom: `1px solid ${theme.border}` }}>
                  <Row style={{ justifyContent: 'space-between', gap: 12 }}>
                    <span style={{ color: theme.text, fontSize: 13 }}>{log.action}</span>
                    <span style={{ color: theme.textFaint, fontSize: 11 }}>{formatDate(log.timestamp)}</span>
                  </Row>
                  <p style={{ color: theme.textFaint, fontSize: 11, marginTop: 3 }}>User #{log.user_id} · {log.ip || 'unknown'}</p>
                </div>
              ))
            )}
          </div>
        </div>
      )}

      {activeTab === 'staff' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          <Row style={{ justifyContent: 'space-between' }}>
            <p style={{ color: theme.textMuted, fontSize: 13 }}>{isLoading ? 'Đang tải...' : `${staff.length} member thật · ${invites.length} invite đang chờ`}</p>
            {isAdmin && (
              <button onClick={() => setShowAdd(v => !v)} style={{ ...primaryBtn({ padding: '8px 15px', fontSize: 12 }), display: 'flex', alignItems: 'center', gap: 6 }}>
                <UserPlus size={13} />Mời nhân viên
              </button>
            )}
          </Row>

          {showAdd && (
            <div style={{ ...cardStyle(), border: `1px solid ${theme.primary}44` }}>
              <p style={{ color: theme.primaryPale, fontWeight: 500, fontSize: 13, marginBottom: 14 }}>Invite nhân viên vào workspace</p>
              <div style={{ display: 'grid', gridTemplateColumns: 'minmax(220px, 1fr) 130px auto', gap: 10, alignItems: 'end' }}>
                <div style={{ position: 'relative' }}>
                  <Label text="Email" />
                  <input
                    style={inputStyle}
                    value={newStaff.email}
                    placeholder="user@email.com"
                    autoComplete="off"
                    onChange={e => handleInviteEmailChange(e.target.value)}
                    onFocus={() => { if (inviteCandidates.length > 0) setShowCandidates(true); }}
                    onBlur={() => setTimeout(() => setShowCandidates(false), 150)}
                  />
                  {showCandidates && inviteCandidates.length > 0 && (
                    <div style={{ position: 'absolute', top: '100%', left: 0, right: 0, marginTop: 4, background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 8, boxShadow: '0 8px 24px rgba(0,0,0,0.18)', zIndex: 10, maxHeight: 240, overflowY: 'auto' }}>
                      {inviteCandidates.map(cand => {
                        const inOtherOrg = cand.org_id > 0 && String(cand.org_id) !== orgId;
                        return (
                          <button
                            key={cand.id}
                            type="button"
                            onMouseDown={e => { e.preventDefault(); pickInviteCandidate(cand); }}
                            style={{ display: 'block', width: '100%', textAlign: 'left', padding: '8px 12px', border: 'none', background: 'transparent', cursor: 'pointer', borderBottom: `1px solid ${theme.borderAlt}`, color: theme.text }}
                          >
                            <div style={{ fontSize: 12.5, fontWeight: 600 }}>{cand.email}</div>
                            <div style={{ fontSize: 11, color: theme.textFaint, marginTop: 2 }}>
                              {cand.name || '—'} · {cand.role}
                              {inOtherOrg && <span style={{ color: theme.yellow, marginLeft: 6 }}>· đang ở workspace #{cand.org_id}, mời sẽ chuyển họ qua đây</span>}
                              {cand.org_id === 0 && <span style={{ color: '#4ade80', marginLeft: 6 }}>· chưa thuộc workspace nào</span>}
                            </div>
                          </button>
                        );
                      })}
                    </div>
                  )}
                </div>
                <div><Label text="Vai trò" /><select style={inputStyle} value={newStaff.role} onChange={e => setNewStaff(p => ({ ...p, role: e.target.value }))}><option value="sales">Sales</option><option value="admin">Admin</option></select></div>
                <button onClick={handleInviteStaff} style={primaryBtn({ padding: '10px 14px' })}>Gửi invite</button>
              </div>
              <p style={{ color: theme.textFaint, fontSize: 11, marginTop: 10 }}>Gõ ≥ 2 ký tự để tự gợi ý user đã đăng ký. Có thể mời cả người đang ở workspace khác — khi họ chấp nhận, họ sẽ chuyển sang workspace này.</p>
            </div>
          )}

          {staffMsg && <p style={{ color: staffMsg.startsWith('Đã') || staffMsg.startsWith('Da ') || staffMsg.startsWith('Invite da tao') ? '#4ade80' : '#fca5a5', fontSize: 12 }}>{staffMsg}</p>}

          {isAdmin && invites.length > 0 && (
            <div style={cardStyle()}>
              <Row style={{ justifyContent: 'space-between', marginBottom: 12 }}>
                <p style={{ color: theme.text, fontWeight: 600, fontSize: 13 }}>Invite đang chờ</p>
                <span style={{ color: theme.textFaint, fontSize: 11 }}>{invites.length} pending</span>
              </Row>
              {invites.map(inv => {
                const inviteUrl = inv.inviteFullUrl || `${window.location.origin}${inv.inviteUrl}`;
                const statusColor = inv.emailStatus === 'sent' ? '#4ade80' : inv.emailStatus === 'failed' ? '#fca5a5' : theme.yellow;
                return (
                  <Row key={inv.id} style={{ justifyContent: 'space-between', gap: 12, padding: '9px 0', borderTop: `1px solid ${theme.borderAlt}` }}>
                    <div style={{ minWidth: 0 }}>
                      <p style={{ color: theme.text, fontSize: 13, fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis' }}>{inv.email}</p>
                      <p style={{ color: theme.textFaint, fontSize: 11 }}>Invite #{inv.id} · {inv.role} · hết hạn {formatDate(inv.expiresAt)}</p>
                      <p style={{ color: statusColor, fontSize: 11, marginTop: 2 }}>Email: {inv.emailStatus || 'pending'}{inv.emailError ? ` · ${inv.emailError}` : ''}</p>
                    </div>
                    <Row style={{ gap: 6, flexShrink: 0 }}>
                      <button
                        onClick={async () => {
                          try {
                            const updated = await resendInvite(inv.id);
                            setStaffMsg(updated.emailStatus === 'sent' ? `Da gui lai email invite cho ${updated.email}.` : `Chua gui duoc email invite cho ${updated.email}.`);
                          } catch (err) {
                            setStaffMsg(err instanceof Error ? err.message : 'Khong gui lai duoc invite.');
                          }
                        }}
                        style={secondaryBtn({ padding: '5px 9px', fontSize: 11 })}
                      >
                        <Mail size={12} /> Gửi lại
                      </button>
                      <button onClick={() => navigator.clipboard?.writeText(inviteUrl)} style={secondaryBtn({ padding: '5px 9px', fontSize: 11 })}><Copy size={12} /> Copy link</button>
                      <button onClick={() => revokeInvite(inv.id)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: theme.textFaint }}><X size={13} /></button>
                    </Row>
                  </Row>
                );
              })}
            </div>
          )}

          <div style={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 12, overflow: 'hidden' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
              <thead>
                <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                  {['User ID', 'Nhân viên', 'Email', 'Org', 'Vai trò', 'Convs', 'Chốt', 'Comments', 'Status', ''].map(h => (
                    <th key={h} style={{ padding: '10px 13px', textAlign: 'left', color: theme.textFaint, fontWeight: 500, fontSize: 11 }}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {staff.length === 0 ? (
                  <tr><td colSpan={10} style={{ padding: 22, textAlign: 'center', color: theme.textMuted }}>Chưa có tài khoản nhân viên trong workspace.</td></tr>
                ) : staff.map(s => (
                  <tr key={s.id} style={{ borderBottom: `1px solid ${theme.borderAlt}` }}>
                    <td style={{ padding: '10px 13px', color: theme.primaryPale, fontFamily: 'monospace', fontWeight: 700 }}>#{s.id}</td>
                    <td style={{ padding: '10px 13px' }}>
                      <Row style={{ gap: 8 }}>
                        <Avatar text={s.name[0] || 'U'} size={26} />
                        <div>
                          <p style={{ color: theme.text, fontWeight: 500 }}>{s.name}</p>
                          <p style={{ color: theme.textFaint, fontSize: 10 }}>Joined {s.joined || '-'}</p>
                        </div>
                      </Row>
                    </td>
                    <td style={{ padding: '10px 13px', color: theme.textMuted }}>{s.email}</td>
                    <td style={{ padding: '10px 13px', color: theme.textFaint, fontFamily: 'monospace' }}>{s.orgId || org.id}</td>
                    <td style={{ padding: '10px 13px', color: '#d1d5db' }}>{s.role}</td>
                    <td style={{ padding: '10px 13px', color: '#d1d5db' }}>{s.convs}</td>
                    <td style={{ padding: '10px 13px', color: '#4ade80' }}>{s.converted}</td>
                    <td style={{ padding: '10px 13px', color: '#d1d5db' }}>{s.cmts}</td>
                    <td style={{ padding: '10px 13px' }}><Badge label={s.status} /></td>
                    <td style={{ padding: '10px 13px' }}>
                      {isAdmin && (
                        <Row style={{ gap: 6 }}>
                          <button onClick={() => toggleStatus(s.id)} style={secondaryBtn({ padding: '4px 8px', fontSize: 10 })}>{s.status === 'Active' ? 'Tạm dừng' : 'Kích hoạt'}</button>
                          <button onClick={() => remove(s.id)} style={{ background: 'none', border: 'none', cursor: 'pointer', color: theme.textFaint }}><X size={13} /></button>
                        </Row>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {activeTab === 'agents' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          {isAdmin && (
            <div style={cardStyle()}>
              <Row style={{ gap: 10, alignItems: 'end' }}>
                <div style={{ flex: 1 }}><Label text="Tên connector / agent" /><input style={inputStyle} value={newAgentName} onChange={e => setNewAgentName(e.target.value)} placeholder="Chrome Extension HCM 01" /></div>
                <button onClick={addAgent} style={primaryBtn({ padding: '10px 16px', fontSize: 13 })}>Tạo token</button>
              </Row>
              {newAgentToken && (
                <div style={{ marginTop: 14, padding: 12, border: `1px solid ${theme.green}55`, borderRadius: 10, background: '#052e1b' }}>
                  <Row style={{ justifyContent: 'space-between', gap: 10 }}>
                    <code style={{ color: '#bbf7d0', fontSize: 12, wordBreak: 'break-all' }}>{newAgentToken}</code>
                    <button onClick={() => navigator.clipboard?.writeText(newAgentToken)} style={secondaryBtn({ padding: '6px 10px', fontSize: 11 })}><Copy size={12} /> Copy</button>
                  </Row>
                </div>
              )}
              {agentMsg && <p style={{ color: '#fca5a5', fontSize: 12, marginTop: 10 }}>{agentMsg}</p>}
            </div>
          )}

          <div style={cardStyle()}>
            <p style={{ color: theme.text, fontWeight: 600, fontSize: 13, marginBottom: 14 }}>Connector / agent đã đăng ký</p>
            {agents.length === 0 ? (
              <p style={{ color: theme.textMuted, fontSize: 13 }}>Chưa có agent token.</p>
            ) : agents.map(a => (
              <div key={a.id} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '11px 0', borderBottom: `1px solid ${theme.border}` }}>
                <Row style={{ gap: 10 }}>
                  <div style={{ width: 8, height: 8, background: a.online ? '#4ade80' : (a.active ? theme.yellow : theme.red), borderRadius: '50%' }} />
                  <div>
                    <p style={{ color: '#d1d5db', fontSize: 13, fontWeight: 500 }}>{a.name}</p>
                    <p style={{ color: theme.textFaint, fontSize: 11 }}>{a.hostname || 'No heartbeat'} · {a.os || 'unknown'} · {a.version || 'unknown'} · {formatDate(a.last_seen)}</p>
                  </div>
                </Row>
                <Row style={{ gap: 8 }}>
                  <Badge label={a.online ? 'Online' : (a.active ? 'Active' : 'Suspended')} />
                  {isAdmin && a.active && <button onClick={() => revokeAgent(a.id)} style={secondaryBtn({ padding: '6px 12px', fontSize: 11, color: theme.red })}>Thu hồi</button>}
                </Row>
              </div>
            ))}
          </div>
        </div>
      )}

      {activeTab === 'billing' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          <div style={{ ...cardStyle(), border: `1px solid ${theme.primary}44` }}>
            <Row style={{ justifyContent: 'space-between', marginBottom: 14 }}>
              <div>
                <p style={{ color: theme.textMuted, fontSize: 11, marginBottom: 3 }}>Gói hiện tại</p>
                <p style={{ color: theme.primaryPale, fontSize: 18, fontWeight: 700 }}>{billing?.plan_tier || planTier} Plan</p>
              </div>
              <Badge label={billing?.payment_status === 'manual' ? 'Manual billing' : 'Active'} />
            </Row>
            {[
              ['Facebook accounts', `${billing?.account_count ?? 0} / ${billing?.max_accounts ?? maxAccounts}`],
              ['Nhân viên', String(billing?.staff_count ?? staff.length)],
              ['Groups đã học', String(billing?.groups ?? 0)],
              ['Leads hôm nay', String(billing?.leads_today ?? 0)],
              ['Outbox draft/approved/sent', `${outboxCounts.draft ?? 0} / ${outboxCounts.approved ?? 0} / ${outboxCounts.sent ?? 0}`],
            ].map(([label, value]) => (
              <Row key={label} style={{ justifyContent: 'space-between', padding: '8px 0', borderBottom: `1px solid ${theme.border}` }}>
                <span style={{ color: theme.textMuted, fontSize: 13 }}>{label}</span>
                <span style={{ color: theme.text, fontSize: 13 }}>{value}</span>
              </Row>
            ))}
          </div>

          <div style={cardStyle()}>
            <p style={{ color: theme.text, fontWeight: 600, fontSize: 13, marginBottom: 14 }}>Mức sử dụng thực tế</p>
            {usageRows.map(u => {
              const pct = usagePercent(u.current, u.max);
              return (
                <div key={u.label} style={{ marginBottom: 13 }}>
                  <Row style={{ justifyContent: 'space-between', marginBottom: 5 }}>
                    <span style={{ color: theme.textMuted, fontSize: 12 }}>{u.label}</span>
                    <span style={{ color: theme.text, fontSize: 12 }}>{u.current.toLocaleString()} / {u.max.toLocaleString()}</span>
                  </Row>
                  <div style={{ height: 5, background: '#2a2f45', borderRadius: 99 }}>
                    <div style={{ width: `${pct}%`, height: '100%', background: pct > 85 ? theme.red : theme.primary, borderRadius: 99 }} />
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
