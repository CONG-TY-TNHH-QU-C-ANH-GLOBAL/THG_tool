'use client';

import { useEffect, useMemo, useState } from 'react';
import { ExternalLink, RefreshCw, Trash2 } from 'lucide-react';
import {
  deleteAllOutboundComments,
  deleteOutbox,
  getOutbox,
  type OutboundMessage,
} from '../../services/outboxService';
import { useLang } from '../../i18n/useLang';

interface CommentingViewProps {
  orgId: string;
  isAdmin: boolean;
}

// AUTONOMOUS-VERIFIED-EXECUTION (project goal, May-2026): the
// human-approval flow is gone. Outbound rows go directly from queue
// to executor with no draft/approve/reject gate. The filter surface
// only shows execution lifecycle states:
//   - planned   (was: approved on the wire — queued, waiting to run)
//   - executing (was: sending on the wire — extension claimed it)
//   - verified  (was: sent on the wire — DOM-verified success)
//   - failed    (everything else: context_drift, blocked, expired …)
type CommentFilter = 'all' | 'planned' | 'executing' | 'verified' | 'failed';

// matchesFilter maps the on-disk status string onto the autonomous
// filter band. The wire still uses 'approved' / 'sending' / 'sent' /
// 'failed' (constants alias on Go side) — when the DB column flips to
// discrete autonomous values this mapping collapses to direct equality.
function matchesFilter(status: string, filter: CommentFilter): boolean {
  switch (filter) {
    case 'all':
      return true;
    case 'planned':
      return status === 'approved';
    case 'executing':
      return status === 'sending';
    case 'verified':
      return status === 'sent';
    case 'failed':
      return status === 'failed' || status === 'rejected';
  }
}

function statusTag(status: string): string {
  switch (status) {
    case 'sent':
      return 'tag tag-ok';
    case 'sending':
      return 'tag tag-warm';
    case 'approved':
      return 'tag tag-cold';
    case 'failed':
    case 'rejected':
      return 'tag tag-hot';
    default:
      return 'tag tag-mute';
  }
}

// Operator-facing label for a status pill. Replaces the legacy
// SENT/DRAFT/APPROVED uppercase strings.
function statusLabel(status: string, lang: 'vi' | 'en'): string {
  if (lang === 'vi') {
    switch (status) {
      case 'approved': return 'ĐÃ LÊN KẾ HOẠCH';
      case 'sending':  return 'ĐANG THỰC THI';
      case 'sent':     return 'ĐÃ XÁC NHẬN';
      case 'failed':   return 'THẤT BẠI';
      case 'rejected': return 'TỪ CHỐI';
      default:         return status.toUpperCase();
    }
  }
  switch (status) {
    case 'approved': return 'PLANNED';
    case 'sending':  return 'EXECUTING';
    case 'sent':     return 'VERIFIED';
    case 'failed':   return 'FAILED';
    case 'rejected': return 'REJECTED';
    default:         return status.toUpperCase();
  }
}

export default function CommentingView({ orgId, isAdmin }: CommentingViewProps) {
  void orgId;
  const { lang, t } = useLang();
  const tv = t.commentingView;
  const [messages, setMessages] = useState<OutboundMessage[]>([]);
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
    () => (filter === 'all' ? messages : messages.filter((message) => matchesFilter(message.status, filter))),
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
                      <span className="mono" style={{ fontSize: 12, color: 'var(--text-mute)' }}>#{message.account_id}</span>
                      <span className={statusTag(message.status)}>{statusLabel(message.status, lang)}</span>
                    </div>
                    <div style={{ color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {message.content || <span style={{ color: 'var(--text-faint)' }}>{tv.emptyValue}</span>}
                    </div>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                      <span className="mono" style={{ fontSize: 11, color: 'var(--text-faint)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {message.target_name || tv.noTarget}
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
                      {selectedMessage.target_name || tv.targetFallback}
                    </div>
                    <div className="mono" style={{ fontSize: 11, color: 'var(--text-faint)', marginTop: 4 }}>
                      #{selectedMessage.account_id}
                    </div>
                  </div>
                  <span className={statusTag(selectedMessage.status)}>{statusLabel(selectedMessage.status, lang)}</span>
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
                        <dd style={{ margin: 0 }}>{selectedMessage.target_name || '—'}</dd>
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
