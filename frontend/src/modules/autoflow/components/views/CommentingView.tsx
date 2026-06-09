'use client';

import { useEffect, useMemo, useState } from 'react';
import { ExternalLink, RefreshCw, ShieldCheck, Trash2 } from 'lucide-react';
import { ActorVerdictChip } from '../ActorVerdictChip';
import {
  clearActorBlock,
  deleteAllOutboundComments,
  deleteOutbox,
  getOutbox,
  type ActorIdentity,
  type OutboundMessage,
} from '../../services/outboxService';
import { useLang } from '../../i18n/useLang';

interface CommentingViewProps {
  orgId: string;
  isAdmin: boolean;
}

// AUTONOMOUS-VERIFIED-EXECUTION (project goal, May-2026): the
// human-approval flow is gone. Outbound rows go directly from queue
// to executor with no draft/approve/reject gate.
//
// PR-1 (verified-state-centric): the filter surface reads the
// (execution_state, verification_outcome) pair directly, not the
// legacy `status` string. The pair-aware predicate below is the
// single source of truth — every status pill, label and filter band
// derives from it.
type CommentFilter = 'all' | 'planned' | 'executing' | 'verified' | 'failed';

// matchesFilter projects the dual-column state pair onto the
// autonomous filter bands the operator picks from. `failed` covers
// every non-verified terminal outcome plus the no-observation
// expired state, since from the operator's point of view all of
// those are "didn't land".
function matchesFilter(msg: Pick<OutboundMessage, 'execution_state' | 'verification_outcome'>, filter: CommentFilter): boolean {
  const state = msg.execution_state;
  const outcome = msg.verification_outcome ?? '';
  switch (filter) {
    case 'all':
      return true;
    case 'planned':
      return state === 'planned';
    case 'executing':
      return state === 'executing';
    case 'verified':
      return state === 'finished' && outcome === 'verified_success';
    case 'failed':
      return state === 'expired' || (state === 'finished' && outcome !== 'verified_success' && outcome !== '');
  }
}

function statusTag(msg: Pick<OutboundMessage, 'execution_state' | 'verification_outcome'>): string {
  const state = msg.execution_state;
  const outcome = msg.verification_outcome ?? '';
  if (state === 'finished' && outcome === 'verified_success') return 'tag tag-ok';
  if (state === 'executing') return 'tag tag-warm';
  if (state === 'planned') return 'tag tag-cold';
  if (state === 'finished' || state === 'expired') return 'tag tag-hot';
  return 'tag tag-mute';
}

// Operator-facing label. Surfaces the specific verification outcome
// when finished so the dashboard can distinguish "verified" from
// "context_drift" / "rate_limited" / "blocked" at a glance.
function statusLabel(msg: Pick<OutboundMessage, 'execution_state' | 'verification_outcome'>, lang: 'vi' | 'en'): string {
  const state = msg.execution_state;
  const outcome = msg.verification_outcome ?? '';
  if (lang === 'vi') {
    if (state === 'planned')   return 'ĐÃ LÊN KẾ HOẠCH';
    if (state === 'executing') return 'ĐANG THỰC THI';
    if (state === 'expired')   return 'HẾT HẠN';
    if (state === 'finished') {
      switch (outcome) {
        case 'verified_success': return 'ĐÃ XÁC NHẬN';
        case 'context_drift':    return 'SAI MỤC TIÊU';
        case 'rate_limited':     return 'BỊ GIỚI HẠN';
        case 'blocked':          return 'BỊ CHẶN';
        case 'captcha':          return 'CẦN XỬ LÝ THỦ CÔNG';
        case 'shadow_rejected':  return 'BỊ FB ẨN';
        case 'execution_failed': return 'LỖI THỰC THI';
        default:                 return 'THẤT BẠI';
      }
    }
    return String(state).toUpperCase();
  }
  if (state === 'planned')   return 'PLANNED';
  if (state === 'executing') return 'EXECUTING';
  if (state === 'expired')   return 'EXPIRED';
  if (state === 'finished') {
    switch (outcome) {
      case 'verified_success': return 'VERIFIED';
      case 'context_drift':    return 'CONTEXT DRIFT';
      case 'rate_limited':     return 'RATE LIMITED';
      case 'blocked':          return 'BLOCKED';
      case 'captcha':          return 'CAPTCHA';
      case 'shadow_rejected':  return 'SHADOW REJECTED';
      case 'execution_failed': return 'EXECUTION FAILED';
      default:                 return 'FAILED';
    }
  }
  return String(state).toUpperCase();
}

export default function CommentingView({ orgId, isAdmin }: CommentingViewProps) {
  void orgId;
  const { lang, t } = useLang();
  const tv = t.commentingView;
  const [messages, setMessages] = useState<OutboundMessage[]>([]);
  const [actors, setActors] = useState<Record<string, ActorIdentity>>({});
  const [filter, setFilter] = useState<CommentFilter>('all');
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const [errorMsg, setErrorMsg] = useState('');
  const [deletingAll, setDeletingAll] = useState(false);

  const handleDeleteAll = async () => {
    if (deletingAll) return;
    if (typeof window !== 'undefined') {
      const ok = window.confirm(
        lang === 'vi'
          ? `Xoá TẤT CẢ ${messages.length} comment trong hàng đợi? Không thể hoàn tác.`
          : `Delete ALL ${messages.length} queued comments? This cannot be undone.`,
      );
      if (!ok) return;
    }
    setDeletingAll(true);
    setErrorMsg('');
    try {
      const res = await deleteAllOutboundComments();
      await load();
      if (typeof window !== 'undefined') {
        window.alert(lang === 'vi' ? `Đã xoá ${res.deleted} comment.` : `Deleted ${res.deleted} comments.`);
      }
    } catch (error) {
      setErrorMsg(error instanceof Error ? error.message : tv.updateError);
    } finally {
      setDeletingAll(false);
    }
  };

  // Autonomous-first filter set. The legacy draft/approved/rejected
  // bands are gone; what remains is the execution lifecycle.
  const FILTERS: Array<{ label: string; value: CommentFilter }> = [
    { label: tv.filterAll, value: 'all' },
    { label: lang === 'vi' ? 'Đã lên kế hoạch' : 'Planned', value: 'planned' },
    { label: lang === 'vi' ? 'Đang thực thi' : 'Executing', value: 'executing' },
    { label: lang === 'vi' ? 'Đã xác nhận' : 'Verified', value: 'verified' },
    { label: lang === 'vi' ? 'Thất bại' : 'Failed', value: 'failed' },
  ];

  const load = async () => {
    setLoading(true);
    setErrorMsg('');
    try {
      const response = await getOutbox({ type: 'comment', limit: 200 });
      setMessages(response.messages ?? []);
      setActors(response.actors ?? {});
    } catch (error) {
      setErrorMsg(error instanceof Error ? error.message : tv.loadError);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const filtered = useMemo(
    () => (filter === 'all' ? messages : messages.filter((message) => matchesFilter(message, filter))),
    [filter, messages],
  );

  useEffect(() => {
    if (filtered.length === 0) {
      setSelectedId(null);
      return;
    }
    if (!filtered.some((message) => message.id === selectedId)) {
      setSelectedId(filtered[0].id);
    }
  }, [filtered, selectedId]);

  const selectedMessage = filtered.find((message) => message.id === selectedId) ?? null;

  // Executor attribution (P1a). The acting Facebook identity for a row's
  // account_id — distinct from the initiating principal (created_by). Falls
  // back to the local account name, then the bare #id, when FB identity is
  // not yet resolved. See specs/COMMENT_INTELLIGENCE_PIPELINE.md §7a.
  const actorOf = (accountId: number): ActorIdentity | undefined => actors[String(accountId)];
  const executorName = (accountId: number): string => {
    const a = actorOf(accountId);
    return a?.fb_display_name || a?.account_name || `#${accountId}`;
  };
  // "Anonymous participant" is a real Facebook value for anonymous group
  // posters (the TARGET, not our actor) — relabel it so it doesn't read as
  // a missing field. See ROOT_CAUSE_REPORT.md note + pipeline doc §8.
  const anonLabel = lang === 'vi' ? '(người đăng ẩn danh)' : '(anonymous poster)';
  const targetLabel = (name: string): string =>
    name === 'Anonymous participant' ? anonLabel : name;

  // Verified-Actor chip (P1b). Surfaces whether the executing FB identity
  // matched the account's expected one. A `blocked` account (actor mismatch)
  // is denied further auto-execute until an operator clears it — that is the
  // integrity gate, not just a label. See pipeline doc §7b.
  // Delegates to the shared ActorVerdictChip so Comment + Lead tabs never diverge.
  const actorVerdictChip = (a: ActorIdentity | undefined) =>
    a ? <ActorVerdictChip actorVerdict={a.actor_verdict} actorBlocked={a.actor_blocked} lang={lang} /> : null;

  // The only operator-driven row-level action left is delete. Approve
  // / reject went away with the draft/approval flow — every queued
  // outbound runs autonomously.
  const transition = async (id: number, action: 'delete') => {
    setErrorMsg('');
    try {
      if (action === 'delete') await deleteOutbox(id);
      await load();
    } catch (error) {
      setErrorMsg(error instanceof Error ? error.message : tv.updateError);
    }
  };

  // Operator override for a Verified-Actor block (P1b). Admin-only; closes the
  // operational loop so a mis-logged account isn't stuck blocked forever.
  const handleClearActorBlock = async (accountId: number) => {
    setErrorMsg('');
    if (typeof window !== 'undefined') {
      const ok = window.confirm(
        lang === 'vi'
          ? `Gỡ chặn actor cho account #${accountId}? Chỉ làm sau khi đã xác nhận đúng tài khoản Facebook đang đăng nhập.`
          : `Clear the actor block for account #${accountId}? Only do this after confirming the correct Facebook identity is logged in.`,
      );
      if (!ok) return;
    }
    try {
      await clearActorBlock(accountId);
      await load();
    } catch (error) {
      setErrorMsg(error instanceof Error ? error.message : tv.updateError);
    }
  };

  const today = new Date().toISOString().slice(0, 10);
  const stats = [
    { label: tv.statSent, value: messages.filter((message) => message.status === 'sent').length },
    { label: tv.statToday, value: messages.filter((message) => message.created_at?.startsWith(today)).length },
    // statPending now means "planned or executing" — both are pre-terminal autonomous states.
    { label: tv.statPending, value: messages.filter((message) => message.status === 'approved' || message.status === 'sending').length },
    { label: tv.statTotal, value: messages.length },
  ];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
      <header style={{ display: 'flex', alignItems: 'flex-start', gap: 16, flexWrap: 'wrap' }}>
        <div>
          <div className="eyebrow"><span className="dot" />{tv.eyebrow}</div>
          <h2 style={{ fontSize: 28, marginTop: 8 }}>{t.views.commentingTitle}</h2>
          <p style={{ color: 'var(--text-mute)', fontSize: 13.5, marginTop: 6 }}>{t.views.commentingSub}</p>
        </div>
        <div style={{ flex: 1 }} />
        {isAdmin && (
          <button
            type="button"
            className="btn btn-ghost btn-sm"
            style={{ color: 'var(--danger)' }}
            disabled={deletingAll || messages.length === 0}
            onClick={() => void handleDeleteAll()}
            title={lang === 'vi' ? 'Xoá toàn bộ comment trong hàng đợi' : 'Delete every queued comment'}
          >
            <Trash2 size={13} />
            {deletingAll
              ? (lang === 'vi' ? 'Đang xoá…' : 'Deleting…')
              : (lang === 'vi' ? 'Xoá tất cả' : 'Delete all')}
          </button>
        )}
        <button type="button" className="btn btn-ghost btn-sm" onClick={() => void load()}>
          <RefreshCw size={13} />
          {t.common.refresh}
        </button>
      </header>

      <div className="stats-grid">
        {stats.map((stat) => (
          <div className="stat" key={stat.label}>
            <div className="stat-label">{stat.label}</div>
            <div className="stat-value tabular">{stat.value}</div>
          </div>
        ))}
      </div>

      {errorMsg && <div className="banner banner-error">{errorMsg}</div>}

      <div className="card" style={{ padding: 0, overflow: 'hidden', minHeight: 560 }}>
        <div className="three-pane" style={{ minHeight: 560 }}>
          <aside style={{ padding: 16 }}>
            <div className="sidebar-section">{tv.filtersLabel}</div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {FILTERS.map((item) => {
                const count = item.value === 'all' ? messages.length : messages.filter((message) => message.status === item.value).length;
                return (
                  <button
                    key={item.value}
                    type="button"
                    className={`filter-pill ${filter === item.value ? 'is-active' : ''}`}
                    style={{ justifyContent: 'space-between', display: 'flex', textAlign: 'left' }}
                    onClick={() => setFilter(item.value)}
                  >
                    <span>{item.label}</span>
                    <span style={{ opacity: 0.7 }}>{count}</span>
                  </button>
                );
              })}
            </div>
          </aside>

          <section style={{ display: 'flex', flexDirection: 'column' }}>
            <div style={{ padding: 16, borderBottom: '1px solid var(--line)' }}>
              <div className="eyebrow">{tv.listTitle}</div>
              <div style={{ marginTop: 6, fontSize: 13, color: 'var(--text-mute)' }}>
                {tv.listCount(filtered.length)}
              </div>
            </div>

            <div style={{ flex: 1, overflowY: 'auto' }}>
              {loading ? (
                <div style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 10 }}>
                  {[0, 1, 2, 3].map((item) => (
                    <div key={item} className="skeleton" style={{ height: 54 }} />
                  ))}
                </div>
              ) : filtered.length === 0 ? (
                <div className="empty" style={{ margin: 16 }}>
                  <div className="eyebrow"><span className="dot" />{t.common.empty}</div>
                  <h3>{tv.emptyTitle}</h3>
                  <p>{tv.emptyDesc}</p>
                </div>
              ) : (
                filtered.map((message) => (
                  <button
                    key={message.id}
                    type="button"
                    onClick={() => setSelectedId(message.id)}
                    className={`nav-item ${selectedId === message.id ? 'is-active' : ''}`}
                    style={{
                      width: '100%',
                      flexDirection: 'column',
                      alignItems: 'stretch',
                      gap: 8,
                      padding: 14,
                      background: 'transparent',
                      border: 0,
                      borderBottom: '1px solid var(--line)',
                      borderRadius: 0,
                      textAlign: 'left',
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                      <span style={{ fontSize: 12, color: 'var(--text-mute)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', display: 'flex', alignItems: 'center', gap: 4 }}>
                        {actorOf(message.account_id)?.actor_blocked && (
                          <ShieldCheck size={12} color="var(--hot)" aria-label={lang === 'vi' ? 'Account bị chặn (sai actor)' : 'Account blocked (actor mismatch)'} />
                        )}
                        {executorName(message.account_id)}
                      </span>
                      <span className={statusTag(message)}>{statusLabel(message, lang)}</span>
                    </div>
                    <div style={{ color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {message.content || <span style={{ color: 'var(--text-faint)' }}>{tv.emptyValue}</span>}
                    </div>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                      <span className="mono" style={{ fontSize: 11, color: 'var(--text-faint)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {message.target_name ? targetLabel(message.target_name) : tv.noTarget}
                      </span>
                      <span className="mono" style={{ fontSize: 11, color: 'var(--text-faint)' }}>
                        {message.created_at?.slice(5, 16) ?? '—'}
                      </span>
                    </div>
                  </button>
                ))
              )}
            </div>
          </section>

          <section style={{ display: 'flex', flexDirection: 'column' }}>
            {selectedMessage ? (
              <>
                <header style={{ display: 'flex', alignItems: 'center', gap: 12, padding: 16, borderBottom: '1px solid var(--line)' }}>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontWeight: 500, color: 'var(--text)' }}>
                      {selectedMessage.target_name ? targetLabel(selectedMessage.target_name) : tv.targetFallback}
                    </div>
                    {(() => {
                      const a = actorOf(selectedMessage.account_id);
                      const postedBy = lang === 'vi' ? 'Đăng bởi' : 'Posted by';
                      return (
                        <div style={{ fontSize: 11, color: 'var(--text-faint)', marginTop: 4, display: 'flex', gap: 6, alignItems: 'center', flexWrap: 'wrap' }}>
                          <span>{postedBy}: <span style={{ color: 'var(--text-mute)' }}>{executorName(selectedMessage.account_id)}</span></span>
                          {a?.fb_profile_url && (
                            <a href={a.fb_profile_url} target="_blank" rel="noopener noreferrer" style={{ color: 'var(--text-mute)' }}>
                              <ExternalLink size={11} />
                            </a>
                          )}
                          <span className="mono">· Account #{selectedMessage.account_id}</span>
                          {actorVerdictChip(a)}
                        </div>
                      );
                    })()}
                  </div>
                  <span className={statusTag(selectedMessage)}>{statusLabel(selectedMessage, lang)}</span>
                </header>

                <div style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 16 }}>
                  <div className="card" style={{ padding: 16 }}>
                    <div className="eyebrow" style={{ marginBottom: 10 }}>{tv.contentTitle}</div>
                    <div style={{ color: 'var(--text)', lineHeight: 1.6, whiteSpace: 'pre-wrap' }}>
                      {selectedMessage.content || <span style={{ color: 'var(--text-faint)' }}>{tv.emptyValue}</span>}
                    </div>
                  </div>

                  <div className="card" style={{ padding: 16 }}>
                    <div className="eyebrow" style={{ marginBottom: 10 }}>{tv.contextTitle}</div>
                    <dl style={{ display: 'grid', gap: 10 }}>
                      <div style={{ display: 'grid', gap: 4 }}>
                        <dt className="field-label">{tv.fieldTarget}</dt>
                        <dd style={{ margin: 0 }}>{selectedMessage.target_name ? targetLabel(selectedMessage.target_name) : '—'}</dd>
                      </div>
                      <div style={{ display: 'grid', gap: 4 }}>
                        <dt className="field-label">{tv.fieldContext}</dt>
                        <dd style={{ margin: 0, color: 'var(--text-mute)', lineHeight: 1.5 }}>
                          {selectedMessage.context || tv.contextEmpty}
                        </dd>
                      </div>
                      <div style={{ display: 'grid', gap: 4 }}>
                        <dt className="field-label">{tv.fieldMedia}</dt>
                        <dd style={{ margin: 0 }}>{selectedMessage.image_path || '—'}</dd>
                      </div>
                    </dl>
                  </div>

                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                    {selectedMessage.target_url && (
                      <a className="btn btn-ghost btn-sm" href={selectedMessage.target_url} target="_blank" rel="noopener noreferrer">
                        <ExternalLink size={13} />
                        {tv.actionOpenTarget}
                      </a>
                    )}
                    {isAdmin && actorOf(selectedMessage.account_id)?.actor_blocked && (
                      <button
                        type="button"
                        className="btn btn-ghost btn-sm"
                        onClick={() => void handleClearActorBlock(selectedMessage.account_id)}
                        style={{ color: 'var(--ok, #16a34a)' }}
                        title={lang === 'vi' ? 'Gỡ chặn auto-execute cho account này (sau khi đã đăng nhập đúng Facebook)' : 'Lift the auto-execute block on this account (after the correct Facebook is logged in)'}
                      >
                        <ShieldCheck size={13} />
                        {lang === 'vi' ? 'Gỡ chặn actor' : 'Clear actor block'}
                      </button>
                    )}
                    <button
                      type="button"
                      className="btn btn-ghost btn-sm"
                      onClick={() => void transition(selectedMessage.id, 'delete')}
                      style={{ color: 'var(--hot)' }}
                    >
                      <Trash2 size={13} />
                      {tv.actionDelete}
                    </button>
                  </div>
                </div>
              </>
            ) : (
              <div className="empty" style={{ margin: 16 }}>
                <h3>{tv.selectTitle}</h3>
                <p>{tv.selectDesc}</p>
              </div>
            )}
          </section>
        </div>
      </div>
    </div>
  );
}
