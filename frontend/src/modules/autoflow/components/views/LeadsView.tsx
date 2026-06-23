'use client';

import { useEffect, useMemo, useState } from 'react';
import { ExternalLink, RefreshCw, Search, Trash2, Wand2 } from 'lucide-react';
import type { Lead, LeadEngagementBadge, LeadEngagementState, LeadStatus, LeadThreadRole, LifecycleTab } from '../../types';
import { LeadFacebookInteractions } from '../leads/LeadFacebookInteractions';
import { LifecycleTabs } from '../leads/LifecycleTabs';
import { useLeads } from '../../hooks/useLeads';
import { useArchivedLeads } from '../../hooks/useArchivedLeads';
import { useLang } from '../../i18n/useLang';
import {
  type ClassificationEntry,
  deleteAllLeads,
  getRecentClassifications,
  reclassifyLeads,
} from '../../services/leadsService';

interface LeadsViewProps {
  orgId: string;
  isAdmin: boolean;
}

const RECLASSIFY_TARGET_ROLES: Array<{ value: string; label: string }> = [
  { value: '', label: 'AI tự quyết' },
  { value: 'candidate', label: 'Ứng viên / Tuyển dụng' },
  { value: 'potential_customer', label: 'Khách quan tâm' },
  { value: 'partner', label: 'Đối tác / Reseller' },
  { value: 'provider_ad', label: 'Provider quảng cáo' },
];

const FILTERS: Array<LeadStatus | 'All'> = ['All', 'Hot', 'Warm', 'Cold'];

type IntentKey = 'all' | 'candidate' | 'potential_customer' | 'partner' | 'provider_ad' | 'spam' | 'unknown';
const INTENT_FILTERS: Array<{ key: IntentKey; label: string }> = [
  { key: 'all', label: 'Tất cả tệp' },
  { key: 'potential_customer', label: 'Khách quan tâm' },
  { key: 'candidate', label: 'Ứng viên / Tuyển dụng' },
  { key: 'partner', label: 'Đối tác / Reseller' },
  { key: 'provider_ad', label: 'Provider quảng cáo' },
  { key: 'spam', label: 'Spam' },
  { key: 'unknown', label: 'Chưa phân loại' },
];

function intentDisplay(raw?: string): { label: string; className: string } {
  const v = (raw ?? '').toLowerCase().trim();
  switch (v) {
    case 'candidate':
      return { label: 'Ứng viên', className: 'tag tag-info' };
    case 'potential_customer':
      return { label: 'Khách quan tâm', className: 'tag tag-ok' };
    case 'partner':
      return { label: 'Đối tác', className: 'tag tag-warm' };
    case 'provider_ad':
      return { label: 'Provider', className: 'tag tag-mute' };
    case 'not_relevant':
      return { label: 'Không liên quan', className: 'tag tag-mute' };
    case 'spam':
      return { label: 'Spam', className: 'tag tag-hot' };
    default:
      return { label: 'Chưa rõ', className: 'tag tag-mute' };
  }
}

function leadIntentKey(lead: Lead): IntentKey {
  const v = (lead.agent ?? '').toLowerCase().trim();
  if (v === 'candidate' || v === 'potential_customer' || v === 'partner' || v === 'provider_ad' || v === 'spam') {
    return v;
  }
  return 'unknown';
}

// Thread role — the participant's structural position in the FB thread.
// See project_thread_role_architecture.md. NOT a CRM status: it is derived
// deterministically at ingest from source_type + intent + vendor-speak.
type RoleKey = 'all' | 'leads' | LeadThreadRole;
const ROLE_FILTERS: Array<{ key: RoleKey; label: string }> = [
  { key: 'leads', label: 'Chỉ leads thật' },
  { key: 'all', label: 'Tất cả vai trò' },
  { key: 'intent_originator', label: 'Người đăng tin' },
  { key: 'buyer_responder', label: 'Khách bình luận' },
  { key: 'supplier_responder', label: 'Nhà cung cấp' },
  { key: 'competitor', label: 'Đối thủ' },
  { key: 'noise', label: 'Nhiễu / Spam' },
];

function threadRoleDisplay(role: LeadThreadRole | undefined): { label: string; className: string } {
  switch (role) {
    case 'intent_originator':
      return { label: 'NGƯỜI ĐĂNG TIN', className: 'tag tag-ok' };
    case 'buyer_responder':
      return { label: 'KHÁCH BÌNH LUẬN', className: 'tag tag-info' };
    case 'supplier_responder':
      return { label: 'NHÀ CUNG CẤP', className: 'tag tag-warm' };
    case 'competitor':
      return { label: 'ĐỐI THỦ', className: 'tag tag-hot' };
    case 'noise':
      return { label: 'NHIỄU', className: 'tag tag-mute' };
    default:
      return { label: 'NGƯỜI ĐĂNG TIN', className: 'tag tag-ok' };
  }
}

// A "real lead" is someone with buying intent — the originator or a
// secondary buyer. Vendors / competitors / noise are not leads. Mirrors
// models.LeadThreadRole.IsLeadRole.
function isLeadRole(role: LeadThreadRole | undefined): boolean {
  return role === 'intent_originator' || role === 'buyer_responder' || role === undefined;
}

// Lead Engagement badge → tag class + Vietnamese label.
// Derived state from the Action Ledger; see feedback_battlefield_badge_framing.md.
// NOT a CRM status — do not let staff edit it; the orchestrator owns it.
function engagementBadgeDisplay(badge: LeadEngagementBadge | undefined): { label: string; className: string } {
  switch (badge) {
    case 'priority':
      return { label: 'CHƯA AI CHẠM', className: 'tag tag-ok' };
    case 'protected':
      return { label: 'ĐANG XỬ LÝ', className: 'tag tag-warm' };
    case 'followup_pending':
      return { label: 'CHỜ REPLY', className: 'tag tag-info' };
    case 'visible':
      return { label: 'ĐÃ CHẠM', className: 'tag tag-mute' };
    case 'closed':
      return { label: 'ĐÃ ĐÓNG', className: 'tag tag-mute' };
    default:
      // No engagement loaded yet — render the same neutral state as untouched.
      return { label: 'CHƯA AI CHẠM', className: 'tag tag-ok' };
  }
}

// Relative time helper for badge context lines ("Alice 4m", "Bob 2h").
function relativeTime(iso: string): string {
  if (!iso) return '';
  const d = new Date(iso);
  if (isNaN(d.getTime())) return '';
  const sec = Math.floor((Date.now() - d.getTime()) / 1000);
  if (sec < 60) return `${Math.max(1, sec)}s`;
  if (sec < 3600) return `${Math.floor(sec / 60)}m`;
  if (sec < 86_400) return `${Math.floor(sec / 3600)}h`;
  return `${Math.floor(sec / 86_400)}d`;
}

// Context line: surfaces who/when next to the badge so the list row reads
// like a battlefield occupancy map ("Alice inbox 4m") instead of an
// abstract status pill.
function engagementContext(state: LeadEngagementState | undefined): string {
  if (!state?.last_engaged_at) return 'chưa ai chạm — ưu tiên';
  // Prefer the Facebook ACCOUNT attribution (execution is owned per account) over a
  // mutable assigned-user name, and surface amplification ("N account"). This is
  // observability — the lead stays shared.
  const accNames = Array.from(new Map((state.entries ?? [])
    .filter(e => e.account_id > 0)
    .map(e => [e.account_id, e.fb_display_name || e.account_name || `Account #${e.account_id}`] as const))
    .values());
  const who = accNames.length === 0 ? (state.last_engaged_by || '(chưa rõ account)')
    : accNames.length === 1 ? accNames[0]
    : `${accNames.length} account`;
  const when = relativeTime(state.last_engaged_at);
  if (state.badge === 'followup_pending') return `${who} • chờ reply ${when}`;
  if (state.badge === 'closed') return `${who} • đã đóng`;
  const action = state.last_engaged_action || 'engaged';
  const actionLabel = action === 'inbox' ? 'inbox' :
                      action === 'comment' ? 'comment' :
                      action === 'group_post' ? 'post' :
                      action === 'profile_post' ? 'post' :
                      action;
  return `${who} • ${actionLabel} ${when}`;
}

// FE defensive: only treat a URL as openable for "Mở bài viết" when it
// carries a Facebook post identifier. A profile or group shell would
// otherwise route the user to the newsfeed (the routing-collapse bug
// described in project_thread_role_architecture.md).
function isFacebookPostURL(u: string | undefined): boolean {
  if (!u) return false;
  return /\/posts\/|\/permalink\/|story_fbid=|multi_permalinks=|[?&]fbid=/.test(u);
}

function statusTagClass(status: string): string {
  switch (status) {
    case 'Hot':
      return 'tag tag-hot';
    case 'Warm':
      return 'tag tag-warm';
    case 'Cold':
      return 'tag tag-cold';
    case 'Active':
    case 'Converted':
      return 'tag tag-ok';
    default:
      return 'tag tag-mute';
  }
}

function leadSearchValue(lead: Lead) {
  return [lead.name, lead.group, lead.agent, lead.phone, lead.facebookUrl ?? '', lead.postUrl ?? '']
    .join(' ')
    .toLowerCase();
}

export default function LeadsView({ orgId, isAdmin }: Readonly<LeadsViewProps>) {
  const { lang, t } = useLang();
  const tv = t.leadsView;
  const locale = lang === 'vi' ? 'vi-VN' : 'en-US';
  const [filter, setFilter] = useState<LeadStatus | 'All'>('All');
  const [intentFilter, setIntentFilter] = useState<IntentKey>('all');
  // Default to "real leads only" — the whole point of Phase B is to keep
  // vendors / competitors / noise out of the primary lead surface.
  const [roleFilter, setRoleFilter] = useState<RoleKey>('leads');
  const [query, setQuery] = useState('');
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const { leads, isLoading, error, refetch, remove } = useLeads(orgId, filter);
  // Lead Lifecycle (PR-4): work-management tab — default "Cần xử lý" hides archived + stale.
  const [lifecycleTab, setLifecycleTab] = useState<LifecycleTab>('active');
  const { archived: archivedLeads } = useArchivedLeads(lifecycleTab === 'archived');
  const [deletingId, setDeletingId] = useState<number | null>(null);
  const [deletingAll, setDeletingAll] = useState(false);
  const [classifyDebugOpen, setClassifyDebugOpen] = useState(false);
  const [classifyEntries, setClassifyEntries] = useState<ClassificationEntry[]>([]);
  const [classifyFilter, setClassifyFilter] = useState<'all' | 'rejected' | 'kept' | 'cold' | 'error'>('rejected');
  const [classifyBusy, setClassifyBusy] = useState(false);
  const [classifyErr, setClassifyErr] = useState('');

  const loadClassifications = async (filter: 'all' | 'rejected' | 'kept' | 'cold' | 'error') => {
    setClassifyBusy(true);
    setClassifyErr('');
    try {
      const res = await getRecentClassifications({
        decision: filter === 'all' ? undefined : filter,
        limit: 100,
      });
      setClassifyEntries(res.classifications ?? []);
    } catch (err) {
      setClassifyErr(err instanceof Error ? err.message : String(err));
    } finally {
      setClassifyBusy(false);
    }
  };

  const openClassifyDebug = () => {
    setClassifyDebugOpen(true);
    void loadClassifications(classifyFilter);
  };

  const [reclassifyOpen, setReclassifyOpen] = useState(false);
  const [reclassifyPrompt, setReclassifyPrompt] = useState('');
  const [reclassifyTargetRole, setReclassifyTargetRole] = useState('');
  const [reclassifyOnlyUnknown, setReclassifyOnlyUnknown] = useState(true);
  const [reclassifyLimit, setReclassifyLimit] = useState(50);
  const [reclassifyBusy, setReclassifyBusy] = useState(false);
  const [reclassifyMsg, setReclassifyMsg] = useState('');
  const [reclassifyError, setReclassifyError] = useState(false);

  const handleReclassify = async () => {
    if (reclassifyBusy) return;
    if (!reclassifyPrompt.trim()) {
      setReclassifyError(true);
      setReclassifyMsg(lang === 'vi' ? 'Mô tả mục tiêu phân loại trước khi chạy.' : 'Describe the intent before reclassifying.');
      return;
    }
    setReclassifyBusy(true);
    setReclassifyMsg('');
    setReclassifyError(false);
    try {
      const res = await reclassifyLeads(orgId, {
        user_prompt: reclassifyPrompt.trim(),
        target_role: reclassifyTargetRole || undefined,
        only_unknown: reclassifyOnlyUnknown,
        limit: Math.max(1, Math.min(200, reclassifyLimit)),
      });
      const summary = lang === 'vi'
        ? `Đã phân loại lại ${res.reclassified}/${res.matched} lead${res.failed ? ` · ${res.failed} lỗi` : ''}.`
        : `Reclassified ${res.reclassified}/${res.matched} leads${res.failed ? ` · ${res.failed} failed` : ''}.`;
      setReclassifyMsg(summary);
      setReclassifyError(false);
      void refetch();
    } catch (err) {
      setReclassifyError(true);
      setReclassifyMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setReclassifyBusy(false);
    }
  };

  const handleDelete = async (lead: Lead) => {
    if (deletingId !== null) return;
    if (typeof window !== 'undefined') {
      const ok = window.confirm(`Xoá lead này?\n\n${lead.name}`);
      if (!ok) return;
    }
    setDeletingId(lead.id);
    try {
      await remove(lead.id, lead.sourceType);
    } catch (err) {
      if (typeof window !== 'undefined') {
        window.alert(err instanceof Error ? err.message : String(err));
      }
    } finally {
      setDeletingId(null);
    }
  };

  const handleDeleteAll = async () => {
    if (deletingAll) return;
    if (typeof window !== 'undefined') {
      const ok = window.confirm(
        lang === 'vi'
          ? `Xoá TẤT CẢ ${leads.length} leads của workspace? Hành động này không thể hoàn tác.`
          : `Delete ALL ${leads.length} leads in this workspace? This cannot be undone.`,
      );
      if (!ok) return;
    }
    setDeletingAll(true);
    try {
      const res = await deleteAllLeads(orgId);
      void refetch();
      if (typeof window !== 'undefined') {
        window.alert(lang === 'vi' ? `Đã xoá ${res.deleted} leads.` : `Deleted ${res.deleted} leads.`);
      }
    } catch (err) {
      if (typeof window !== 'undefined') {
        window.alert(err instanceof Error ? err.message : String(err));
      }
    } finally {
      setDeletingAll(false);
    }
  };

  // Lead Lifecycle (PR-4): the active tab picks the SOURCE. Archived comes from its own
  // lazy fetch; every other tab filters the live list by freshness_state (missing →
  // 'active'). stale never appears — it is not a tab.
  const lifecycleSource = useMemo(() => {
    if (lifecycleTab === 'archived') return archivedLeads;
    return leads.filter((lead) => (lead.lifecycle?.freshness_state ?? 'active') === lifecycleTab);
  }, [leads, archivedLeads, lifecycleTab]);

  const lifecycleCounts = useMemo(() => {
    const counts: Record<LifecycleTab, number> = { active: 0, waiting_reply: 0, followup_due: 0, archived: archivedLeads.length };
    for (const lead of leads) {
      const state = lead.lifecycle?.freshness_state ?? 'active';
      if (state === 'active' || state === 'waiting_reply' || state === 'followup_due') counts[state] += 1;
    }
    return counts;
  }, [leads, archivedLeads.length]);

  const filteredLeads = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return lifecycleSource.filter((lead) => {
      if (roleFilter === 'leads' && !isLeadRole(lead.threadRole)) return false;
      if (roleFilter !== 'all' && roleFilter !== 'leads' && (lead.threadRole ?? 'intent_originator') !== roleFilter) return false;
      if (intentFilter !== 'all' && leadIntentKey(lead) !== intentFilter) return false;
      if (normalized && !leadSearchValue(lead).includes(normalized)) return false;
      return true;
    });
  }, [lifecycleSource, query, intentFilter, roleFilter]);

  const intentCounts = useMemo(() => {
    const counts: Record<IntentKey, number> = {
      all: leads.length,
      potential_customer: 0,
      candidate: 0,
      partner: 0,
      provider_ad: 0,
      spam: 0,
      unknown: 0,
    };
    for (const lead of leads) {
      const key = leadIntentKey(lead);
      counts[key] = (counts[key] ?? 0) + 1;
    }
    return counts;
  }, [leads]);

  const roleCounts = useMemo(() => {
    const counts: Record<RoleKey, number> = {
      all: leads.length,
      leads: 0,
      intent_originator: 0,
      buyer_responder: 0,
      supplier_responder: 0,
      competitor: 0,
      noise: 0,
    };
    for (const lead of leads) {
      const role = lead.threadRole ?? 'intent_originator';
      counts[role] = (counts[role] ?? 0) + 1;
      if (isLeadRole(lead.threadRole)) counts.leads += 1;
    }
    return counts;
  }, [leads]);

  useEffect(() => {
    if (filteredLeads.length === 0) {
      setSelectedId(null);
      return;
    }
    if (!filteredLeads.some((lead) => lead.id === selectedId)) {
      setSelectedId(filteredLeads[0].id);
    }
  }, [filteredLeads, selectedId]);

  const selectedLead = filteredLeads.find((lead) => lead.id === selectedId) ?? null;
  const totals = {
    all: leads.length,
    hot: leads.filter((lead) => lead.status === 'Hot').length,
    warm: leads.filter((lead) => lead.status === 'Warm').length,
    avgScore: leads.length ? Math.round(leads.reduce((sum, lead) => sum + lead.score, 0) / leads.length) : 0,
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
      <header style={{ display: 'flex', alignItems: 'flex-start', gap: 16, flexWrap: 'wrap' }}>
        <div>
          <div className="eyebrow">
            <span className="dot" />
            {tv.eyebrowSales}
          </div>
          <h2 style={{ fontSize: 28, marginTop: 8 }}>{t.views.leadsTitle}</h2>
          <p style={{ color: 'var(--text-mute)', fontSize: 13.5, marginTop: 6 }}>{t.views.leadsSub}</p>
        </div>
        <div style={{ flex: 1 }} />
        {isAdmin && (
          <button
            className="btn btn-ghost btn-sm"
            type="button"
            onClick={openClassifyDebug}
            title={lang === 'vi' ? 'Xem AI loại bài như thế nào (kept / rejected + lý do)' : 'See how the AI classifies posts (kept / rejected + reason)'}
          >
            <Wand2 size={13} />
            {lang === 'vi' ? 'Debug AI' : 'AI debug'}
          </button>
        )}
        {isAdmin && (
          <button
            className="btn btn-ghost btn-sm"
            type="button"
            onClick={() => { setReclassifyOpen(true); setReclassifyMsg(''); setReclassifyError(false); }}
            title={lang === 'vi' ? 'Chạy lại AI phân loại trên các lead cũ' : 'Re-run AI classifier on existing leads'}
          >
            <Wand2 size={13} />
            {lang === 'vi' ? 'Phân loại lại' : 'Reclassify'}
          </button>
        )}
        {isAdmin && (
          <button
            className="btn btn-ghost btn-sm"
            type="button"
            style={{ color: 'var(--danger)' }}
            disabled={deletingAll || leads.length === 0}
            onClick={() => void handleDeleteAll()}
            title={lang === 'vi' ? 'Xoá toàn bộ leads của workspace' : 'Delete every lead in this workspace'}
          >
            <Trash2 size={13} />
            {deletingAll
              ? (lang === 'vi' ? 'Đang xoá…' : 'Deleting…')
              : (lang === 'vi' ? 'Xoá tất cả' : 'Delete all')}
          </button>
        )}
        <button className="btn btn-ghost btn-sm" type="button" onClick={() => void refetch()}>
          <RefreshCw size={13} />
          {t.common.refresh}
        </button>
      </header>

      {classifyDebugOpen && (
        <div
          role="dialog"
          aria-modal="true"
          style={{ position: 'fixed', inset: 0, background: 'var(--modal-scrim)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 60 }}
          onClick={() => setClassifyDebugOpen(false)}
        >
          <div
            className="card"
            style={{ width: 900, maxWidth: '94vw', maxHeight: '86vh', padding: 20, display: 'flex', flexDirection: 'column', gap: 12 }}
            onClick={e => e.stopPropagation()}
          >
            <div style={{ display: 'flex', alignItems: 'baseline', gap: 12 }}>
              <div>
                <div className="eyebrow"><span className="dot" />AI DEBUG</div>
                <h3 style={{ marginTop: 4, fontSize: 18 }}>
                  {lang === 'vi' ? 'AI đã loại bài như thế nào' : 'How the AI classified posts'}
                </h3>
                <p style={{ color: 'var(--text-mute)', fontSize: 12.5, marginTop: 4 }}>
                  {lang === 'vi'
                    ? 'Mỗi bài đi qua classifier đều được log lại — cả bài được giữ lẫn bài bị loại — kèm intent + lý do của AI.'
                    : 'Every post that hits the classifier is logged — kept and rejected alike — with the AI intent and reason.'}
                </p>
              </div>
              <div style={{ flex: 1 }} />
              <button
                type="button"
                className="btn btn-ghost btn-sm"
                onClick={() => setClassifyDebugOpen(false)}
              >
                {lang === 'vi' ? 'Đóng' : 'Close'}
              </button>
            </div>

            <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
              {(['rejected', 'cold', 'kept', 'error', 'all'] as const).map(f => (
                <button
                  key={f}
                  type="button"
                  className={`filter-pill ${classifyFilter === f ? 'is-active' : ''}`}
                  onClick={() => { setClassifyFilter(f); void loadClassifications(f); }}
                >
                  {f === 'all' ? (lang === 'vi' ? 'Tất cả' : 'All')
                    : f === 'kept' ? (lang === 'vi' ? 'Được giữ' : 'Kept')
                    : f === 'rejected' ? (lang === 'vi' ? 'Bị loại' : 'Rejected')
                    : f === 'cold' ? 'Cold'
                    : (lang === 'vi' ? 'Lỗi AI' : 'AI error')}
                </button>
              ))}
              <div style={{ flex: 1 }} />
              <button
                type="button"
                className="btn btn-ghost btn-sm"
                onClick={() => void loadClassifications(classifyFilter)}
                disabled={classifyBusy}
              >
                <RefreshCw size={13} /> {classifyBusy ? (lang === 'vi' ? 'Đang tải…' : 'Loading…') : (lang === 'vi' ? 'Tải lại' : 'Refresh')}
              </button>
            </div>

            {classifyErr && <div className="auth-error">{classifyErr}</div>}

            <div style={{ flex: 1, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 8 }}>
              {classifyEntries.length === 0 && !classifyBusy && (
                <div className="empty" style={{ margin: 20 }}>
                  <div className="eyebrow"><span className="dot" />KHÔNG CÓ DỮ LIỆU</div>
                  <h3>{lang === 'vi' ? 'Chưa có classification nào' : 'No classifications yet'}</h3>
                  <p>{lang === 'vi' ? 'Chạy thử một đợt crawl rồi quay lại đây.' : 'Run a crawl first then come back.'}</p>
                </div>
              )}
              {classifyEntries.map(entry => {
                const decisionColor =
                  entry.decision === 'kept' ? 'var(--ok)'
                  : entry.decision === 'rejected' ? 'var(--hot)'
                  : entry.decision === 'cold' ? 'var(--text-mute)'
                  : entry.decision === 'error' ? 'var(--warn)'
                  : 'var(--text-faint)';
                return (
                  <div
                    key={entry.id}
                    style={{
                      padding: 12,
                      border: '1px solid var(--line)',
                      borderRadius: 10,
                      background: 'var(--bg-elev)',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: 6,
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'baseline', gap: 8, flexWrap: 'wrap' }}>
                      <span style={{ fontSize: 13, fontWeight: 600 }}>{entry.author_name || '(no author)'}</span>
                      <span className="tag" style={{ fontSize: 10, color: decisionColor, borderColor: decisionColor }}>
                        {entry.decision.toUpperCase()}
                      </span>
                      {entry.ai_intent && (
                        <span className="tag tag-mute" style={{ fontSize: 10 }}>
                          intent: {entry.ai_intent}
                        </span>
                      )}
                      {entry.ai_priority && (
                        <span className="tag tag-mute" style={{ fontSize: 10 }}>
                          priority: {entry.ai_priority}
                        </span>
                      )}
                      {entry.target_role && (
                        <span className="tag tag-mute" style={{ fontSize: 10 }}>
                          target: {entry.target_role}
                        </span>
                      )}
                      <span className="mono" style={{ fontSize: 10, color: 'var(--text-faint)' }}>
                        score {entry.ai_score.toFixed(2)}
                      </span>
                      <div style={{ flex: 1 }} />
                      {entry.source_url && (
                        <a
                          href={entry.source_url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="mono"
                          style={{ fontSize: 11, color: 'var(--text-faint)' }}
                        >
                          {entry.source_url.slice(0, 60)}{entry.source_url.length > 60 ? '…' : ''}
                        </a>
                      )}
                    </div>
                    <p style={{ fontSize: 12.5, color: 'var(--text)', margin: 0, lineHeight: 1.5 }}>
                      {entry.content_snippet || '(no content)'}
                    </p>
                    {entry.ai_reason && (
                      <p style={{ fontSize: 12, color: 'var(--text-mute)', margin: 0, fontStyle: 'italic' }}>
                        AI: {entry.ai_reason}
                      </p>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      )}

      {reclassifyOpen && (
        <div
          role="dialog"
          aria-modal="true"
          style={{ position: 'fixed', inset: 0, background: 'var(--modal-scrim)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 60 }}
          onClick={() => { if (!reclassifyBusy) setReclassifyOpen(false); }}
        >
          <div
            className="card"
            style={{ width: 480, maxWidth: '92vw', padding: 20, display: 'flex', flexDirection: 'column', gap: 14 }}
            onClick={e => e.stopPropagation()}
          >
            <div>
              <div className="eyebrow"><span className="dot" />{lang === 'vi' ? 'AI PHÂN LOẠI LẠI' : 'AI RECLASSIFY'}</div>
              <h3 style={{ marginTop: 6, fontSize: 18 }}>{lang === 'vi' ? 'Chạy lại AI phân loại' : 'Re-run AI classifier'}</h3>
              <p style={{ color: 'var(--text-mute)', fontSize: 12.5, marginTop: 6 }}>
                {lang === 'vi'
                  ? 'AI sẽ đọc lại nội dung các lead đã có và gắn lại tệp / điểm dựa theo mục tiêu bạn mô tả bên dưới. Lead đã được gắn nhãn tay sẽ được giữ nguyên khi bật "chỉ phân loại lại lead chưa rõ".'
                  : 'AI will re-read existing leads and retag intent/score using the goal you describe. Manually labelled leads stay intact when "only unclassified" is on.'}
              </p>
            </div>

            <label className="field">
              <span className="field-label">{lang === 'vi' ? 'MÔ TẢ MỤC TIÊU' : 'GOAL DESCRIPTION'}</span>
              <textarea
                className="input"
                rows={3}
                placeholder={lang === 'vi' ? 'VD: cào bài tuyển dụng nhân sự sales POD, ưu tiên người đang đăng tin tuyển' : 'e.g. recruiting sales staff for POD shop, prioritise hiring posts'}
                value={reclassifyPrompt}
                onChange={e => setReclassifyPrompt(e.target.value)}
                disabled={reclassifyBusy}
              />
            </label>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 110px', gap: 10 }}>
              <label className="field">
                <span className="field-label">{lang === 'vi' ? 'TỆP MỤC TIÊU' : 'TARGET INTENT'}</span>
                <select
                  className="input"
                  value={reclassifyTargetRole}
                  onChange={e => setReclassifyTargetRole(e.target.value)}
                  disabled={reclassifyBusy}
                >
                  {RECLASSIFY_TARGET_ROLES.map(opt => (
                    <option key={opt.value} value={opt.value}>{opt.label}</option>
                  ))}
                </select>
              </label>
              <label className="field">
                <span className="field-label">{lang === 'vi' ? 'GIỚI HẠN' : 'LIMIT'}</span>
                <input
                  type="number"
                  className="input"
                  min={1}
                  max={200}
                  value={reclassifyLimit}
                  onChange={e => setReclassifyLimit(Number(e.target.value) || 50)}
                  disabled={reclassifyBusy}
                />
              </label>
            </div>

            <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13, color: 'var(--text-mute)' }}>
              <input
                type="checkbox"
                checked={reclassifyOnlyUnknown}
                onChange={e => setReclassifyOnlyUnknown(e.target.checked)}
                disabled={reclassifyBusy}
              />
              {lang === 'vi' ? 'Chỉ phân loại lại lead "Chưa rõ" (giữ nguyên lead đã có nhãn).' : 'Only re-tag leads with unknown intent.'}
            </label>

            {reclassifyMsg && (
              <div style={{ fontSize: 12.5, color: reclassifyError ? 'var(--hot)' : 'var(--ok)' }}>
                {reclassifyMsg}
              </div>
            )}

            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 4 }}>
              <button
                type="button"
                className="btn btn-ghost btn-sm"
                onClick={() => setReclassifyOpen(false)}
                disabled={reclassifyBusy}
              >
                {lang === 'vi' ? 'Đóng' : 'Close'}
              </button>
              <button
                type="button"
                className="btn btn-primary btn-sm"
                onClick={handleReclassify}
                disabled={reclassifyBusy}
              >
                <Wand2 size={13} />
                {reclassifyBusy
                  ? (lang === 'vi' ? 'Đang chạy…' : 'Running…')
                  : (lang === 'vi' ? 'Chạy phân loại' : 'Run reclassify')}
              </button>
            </div>
          </div>
        </div>
      )}

      <div className="stats-grid">
        <div className="stat">
          <div className="stat-label">{tv.statTotal}</div>
          <div className="stat-value tabular">{totals.all.toLocaleString(locale)}</div>
        </div>
        <div className="stat">
          <div className="stat-label">{tv.statHot}</div>
          <div className="stat-value tabular" style={{ color: 'var(--hot)' }}>{totals.hot}</div>
        </div>
        <div className="stat">
          <div className="stat-label">{tv.statWarm}</div>
          <div className="stat-value tabular" style={{ color: 'var(--warn)' }}>{totals.warm}</div>
        </div>
        <div className="stat">
          <div className="stat-label">{tv.statAvgScore}</div>
          <div className="stat-value tabular">{totals.avgScore}</div>
        </div>
      </div>

      <div className="card" style={{ padding: 0, overflow: 'hidden', minHeight: 560 }}>
        <div className="three-pane" style={{ minHeight: 560 }}>
          <aside style={{ padding: 16 }}>
            <div className="sidebar-section">{lang === 'vi' ? 'VÒNG ĐỜI' : 'LIFECYCLE'}</div>
            <LifecycleTabs active={lifecycleTab} counts={lifecycleCounts} onSelect={setLifecycleTab} lang={lang === 'vi' ? 'vi' : 'en'} />

            <div className="sidebar-section" style={{ marginTop: 16 }}>{tv.filtersLabel}</div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              {FILTERS.map((item) => {
                const count = item === 'All' ? totals.all : leads.filter((lead) => lead.status === item).length;
                const label = item === 'All' ? tv.filterAll : item;
                return (
                  <button
                    key={item}
                    type="button"
                    className={`filter-pill ${filter === item ? 'is-active' : ''}`}
                    style={{ justifyContent: 'space-between', display: 'flex', textAlign: 'left' }}
                    onClick={() => setFilter(item)}
                  >
                    <span>{label}</span>
                    <span style={{ opacity: 0.7 }}>{count}</span>
                  </button>
                );
              })}
            </div>

            <div className="sidebar-section" style={{ marginTop: 16 }}>VAI TRÒ THREAD</div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              {ROLE_FILTERS.map((opt) => (
                <button
                  key={opt.key}
                  type="button"
                  className={`filter-pill ${roleFilter === opt.key ? 'is-active' : ''}`}
                  style={{ justifyContent: 'space-between', display: 'flex', textAlign: 'left' }}
                  onClick={() => setRoleFilter(opt.key)}
                >
                  <span>{opt.label}</span>
                  <span style={{ opacity: 0.7 }}>{roleCounts[opt.key] ?? 0}</span>
                </button>
              ))}
            </div>

            <div className="sidebar-section" style={{ marginTop: 16 }}>TỆP / INTENT</div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              {INTENT_FILTERS.map((opt) => (
                <button
                  key={opt.key}
                  type="button"
                  className={`filter-pill ${intentFilter === opt.key ? 'is-active' : ''}`}
                  style={{ justifyContent: 'space-between', display: 'flex', textAlign: 'left' }}
                  onClick={() => setIntentFilter(opt.key)}
                >
                  <span>{opt.label}</span>
                  <span style={{ opacity: 0.7 }}>{intentCounts[opt.key] ?? 0}</span>
                </button>
              ))}
            </div>

            <div className="sidebar-section" style={{ marginTop: 16 }}>{tv.searchLabel}</div>
            <div style={{ position: 'relative' }}>
              <Search size={13} style={{ position: 'absolute', left: 12, top: 11, color: 'var(--text-faint)' }} />
              <input
                className="input"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={tv.searchPlaceholder}
                style={{ paddingLeft: 34 }}
              />
            </div>
          </aside>

          <div style={{ overflowY: 'auto' }}>
            <div className="table-row table-head" style={{ gridTemplateColumns: '1fr 64px 70px' }}>
              <div>Lead</div><div>SCORE</div><div></div>
            </div>
            {isLoading ? (
              <div style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 10 }}>
                {[0, 1, 2, 3, 4].map((item) => (
                  <div key={item} className="skeleton" style={{ height: 56 }} />
                ))}
              </div>
            ) : error ? (
              <div className="empty" style={{ margin: 16 }}>
                <div className="eyebrow"><span className="dot" />{t.common.error}</div>
                <h3>{tv.errorTitle}</h3>
                <p>{error.message}</p>
              </div>
            ) : filteredLeads.length === 0 ? (
              <div className="empty" style={{ marginTop: 40 }}>
                <div className="eyebrow"><span className="dot" />KHÔNG CÓ LEAD</div>
                <h3>Không có gì khớp bộ lọc</h3>
                <p>Điều chỉnh bộ lọc hoặc chạy đợt crawl tiếp theo.</p>
              </div>
            ) : (
              filteredLeads.map((lead) => {
                const intent = intentDisplay(lead.agent);
                const role = threadRoleDisplay(lead.threadRole);
                const engagement = engagementBadgeDisplay(lead.engagement?.badge);
                const ctx = engagementContext(lead.engagement);
                return (
                  <div
                    key={lead.id}
                    className={`table-row ${selectedId === lead.id ? 'is-active' : ''}`}
                    style={{
                      gridTemplateColumns: '1fr 64px 70px',
                      cursor: 'pointer',
                      // De-emphasise non-lead participants so the eye still
                      // lands on real leads even when "all roles" is on.
                      opacity: isLeadRole(lead.threadRole) ? 1 : 0.62,
                    }}
                    onClick={() => setSelectedId(lead.id)}
                  >
                    <div style={{ minWidth: 0 }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                        <span style={{ fontSize: 13.5, color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          {lead.name}
                        </span>
                        <span className={role.className} style={{ fontSize: 10, padding: '1px 6px', flexShrink: 0 }}>{role.label}</span>
                        <span className={intent.className} style={{ fontSize: 10, padding: '1px 6px', flexShrink: 0 }}>{intent.label}</span>
                      </div>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 3, minWidth: 0 }}>
                        <span className={engagement.className} style={{ fontSize: 10, padding: '1px 6px', flexShrink: 0 }}>{engagement.label}</span>
                        <span className="mono" style={{ fontSize: 11, color: 'var(--text-faint)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          {ctx}
                        </span>
                      </div>
                    </div>
                    <div className="tabular mono" style={{ fontSize: 13.5 }}>{lead.score}</div>
                    <div><span className={statusTagClass(lead.status)}>{lead.status.toUpperCase()}</span></div>
                  </div>
                );
              })
            )}
          </div>

          <div style={{ padding: 24, overflowY: 'auto' }}>
            {selectedLead ? (
              <div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                  <span className="avatar avatar-lg">{(selectedLead.name.trim()[0] || 'L').toUpperCase()}</span>
                  <div>
                    <h3 style={{ fontSize: 18 }}>{selectedLead.name}</h3>
                    <div className="mono" style={{ fontSize: 11, color: 'var(--text-faint)' }}>{selectedLead.group || tv.unknownGroup}</div>
                  </div>
                  <div style={{ flex: 1 }} />
                  <span className={threadRoleDisplay(selectedLead.threadRole).className}>{threadRoleDisplay(selectedLead.threadRole).label}</span>
                  <span className={intentDisplay(selectedLead.agent).className}>{intentDisplay(selectedLead.agent).label}</span>
                  <span className={statusTagClass(selectedLead.status)}>{selectedLead.status.toUpperCase()}</span>
                </div>
                {!isLeadRole(selectedLead.threadRole) && (
                  <div
                    style={{
                      marginTop: 10, padding: '8px 12px', borderRadius: 8,
                      background: 'var(--bg-elev)', border: '1px solid var(--line)',
                      fontSize: 12.5, color: 'var(--text-mute)',
                    }}
                  >
                    {selectedLead.threadRole === 'supplier_responder'
                      ? 'Đây là nhà cung cấp trả lời trong thread — KHÔNG phải khách hàng tiềm năng.'
                      : selectedLead.threadRole === 'competitor'
                        ? 'Đây là đối thủ đăng bài quảng cáo — KHÔNG phải lead.'
                        : 'Đây là nhiễu / spam — KHÔNG phải lead.'}
                  </div>
                )}

                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 1, marginTop: 24, background: 'var(--line)', border: '1px solid var(--line)', borderRadius: 8, overflow: 'hidden' }}>
                  <div className="stat" style={{ background: 'var(--bg-elev)' }}>
                    <div className="stat-label">SCORE</div>
                    <div className="stat-value tabular">{selectedLead.score}</div>
                  </div>
                  <div className="stat" style={{ background: 'var(--bg-elev)' }}>
                    <div className="stat-label">LAST SEEN</div>
                    <div className="mono" style={{ fontSize: 14, color: 'var(--text)', marginTop: 6 }}>{selectedLead.last}</div>
                  </div>
                </div>

                <div className="sidebar-section" style={{ marginTop: 20, paddingLeft: 0 }}>AI PHÂN TÍCH</div>
                <p style={{ fontSize: 13.5, color: 'var(--text)', lineHeight: 1.55 }}>
                  {selectedLead.phone || tv.noteEmpty}
                </p>

                <div style={{ marginTop: 20 }}>
                  <LeadFacebookInteractions entries={selectedLead.engagement?.entries ?? []} eligibility={selectedLead.engagement?.eligibility} />
                </div>

                <div className="sidebar-section" style={{ marginTop: 20, paddingLeft: 0, display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span>HOẠT ĐỘNG WORKSPACE</span>
                  <span className={engagementBadgeDisplay(selectedLead.engagement?.badge).className} style={{ fontSize: 10, padding: '1px 6px' }}>
                    {engagementBadgeDisplay(selectedLead.engagement?.badge).label}
                  </span>
                </div>
                {(() => {
                  const entries = selectedLead.engagement?.entries ?? [];
                  if (entries.length === 0) {
                    return (
                      <p style={{ fontSize: 12.5, color: 'var(--text-faint)', marginTop: 4 }}>
                        Chưa có ai trong workspace tương tác lead này.
                      </p>
                    );
                  }
                  return (
                    <ul style={{ listStyle: 'none', padding: 0, margin: '4px 0 0 0', display: 'flex', flexDirection: 'column', gap: 6 }}>
                      {entries.slice(0, 5).map((entry, idx) => {
                        const who = entry.user_name || entry.fb_display_name || entry.account_name || '(agent tự động)';
                        const acct = entry.account_name && entry.user_name ? ` qua ${entry.account_name}` : '';
                        const when = relativeTime(entry.performed_at);
                        return (
                          <li key={idx} style={{ display: 'flex', alignItems: 'baseline', gap: 8, fontSize: 12.5 }}>
                            <span className="mono" style={{ color: 'var(--text)', minWidth: 32 }}>{when}</span>
                            <span style={{ color: 'var(--text)' }}>{who}</span>
                            <span style={{ color: 'var(--text-faint)' }}>· {entry.action}{acct}</span>
                          </li>
                        );
                      })}
                    </ul>
                  );
                })()}

                <div className="sidebar-section" style={{ marginTop: 20, paddingLeft: 0 }}>CHI TIẾT</div>
                <dl style={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '8px 16px', fontSize: 13, margin: 0 }}>
                  <dt style={{ color: 'var(--text-faint)' }}>Tệp</dt>
                  <dd className="mono" style={{ color: 'var(--text)', margin: 0 }}>{intentDisplay(selectedLead.agent).label}</dd>
                  <dt style={{ color: 'var(--text-faint)' }}>Ngành / nguồn</dt>
                  <dd className="mono" style={{ color: 'var(--text)', margin: 0 }}>{selectedLead.group || '—'}</dd>
                  <dt style={{ color: 'var(--text-faint)' }}>Lần cuối</dt>
                  <dd style={{ color: 'var(--text-mute)', margin: 0 }}>{selectedLead.last}</dd>
                </dl>

                {/* Role-aware routing (Phase D). The primary action depends
                    on the thread role: an originator's battlefield is the
                    post; a responder's exact location is their comment.
                    "Mở bài viết" always opens the canonical post; we never
                    default the primary action to the (unstable) profile URL. */}
                {(() => {
                  const postOpenable = isFacebookPostURL(selectedLead.postUrl);
                  const commentUrl = selectedLead.engagementPermalink;
                  const commentOpenable = isFacebookPostURL(commentUrl);
                  const responderRole = !isLeadRole(selectedLead.threadRole);
                  // For a responder with a real comment permalink, the comment
                  // is the primary surface. Otherwise the post is.
                  const commentIsPrimary = responderRole && commentOpenable;
                  const postBtnClass = commentIsPrimary ? 'btn btn-ghost btn-sm' : 'btn btn-primary btn-sm';
                  return (
                    <div style={{ display: 'flex', gap: 8, marginTop: 24, flexWrap: 'wrap' }}>
                      {commentOpenable && (
                        <a
                          className={commentIsPrimary ? 'btn btn-primary btn-sm' : 'btn btn-ghost btn-sm'}
                          href={commentUrl}
                          target="_blank"
                          rel="noopener noreferrer"
                        >
                          <ExternalLink size={13} style={{ marginRight: 6 }} />
                          Mở bình luận
                        </a>
                      )}
                      {postOpenable ? (
                        <a className={postBtnClass} href={selectedLead.postUrl} target="_blank" rel="noopener noreferrer">
                          <ExternalLink size={13} style={{ marginRight: 6 }} />
                          Mở bài viết
                        </a>
                      ) : selectedLead.postUrl ? (
                        <span
                          className="btn btn-ghost btn-sm"
                          title={`URL không có post id, có thể route về newsfeed: ${selectedLead.postUrl}`}
                          style={{ opacity: 0.5, cursor: 'not-allowed' }}
                        >
                          <ExternalLink size={13} style={{ marginRight: 6 }} />
                          Không có link bài viết
                        </span>
                      ) : null}
                      {selectedLead.facebookUrl && (
                        <a className="btn btn-ghost btn-sm" href={selectedLead.facebookUrl} target="_blank" rel="noopener noreferrer">
                          <ExternalLink size={13} style={{ marginRight: 6 }} />
                          Mở profile
                        </a>
                      )}
                    </div>
                  );
                })()}

                <div style={{ display: 'flex', gap: 8, marginTop: 10, flexWrap: 'wrap' }}>
                  <button type="button" className="btn btn-ghost btn-sm" onClick={() => void refetch()}>
                    <RefreshCw size={13} style={{ marginRight: 6 }} />
                    Đồng bộ
                  </button>
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    style={{ marginLeft: 'auto', color: 'var(--danger)' }}
                    disabled={deletingId === selectedLead.id}
                    onClick={() => void handleDelete(selectedLead)}
                  >
                    <Trash2 size={13} style={{ marginRight: 6 }} />
                    {deletingId === selectedLead.id ? 'Đang xoá…' : 'Xoá lead'}
                  </button>
                </div>
              </div>
            ) : (
              <div className="empty" style={{ marginTop: 40 }}>
                <div className="eyebrow"><span className="dot" />KHÔNG CÓ LEAD</div>
                <h3>Không có gì khớp bộ lọc</h3>
                <p>Điều chỉnh bộ lọc hoặc chạy đợt crawl tiếp theo.</p>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
