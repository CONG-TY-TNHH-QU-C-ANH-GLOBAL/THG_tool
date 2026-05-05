'use client';

import { useEffect, useMemo, useState } from 'react';
import { Check, ExternalLink, RefreshCw, Trash2, X } from 'lucide-react';
import {
  approveOutbox,
  deleteOutbox,
  getOutbox,
  type OutboundMessage,
  rejectOutbox,
} from '../../services/outboxService';
import { useLang } from '../../i18n/useLang';

interface CommentingViewProps {
  orgId: string;
}

type CommentFilter = 'all' | 'draft' | 'approved' | 'sent' | 'failed' | 'rejected';

function statusTag(status: string): string {
  switch (status) {
    case 'sent':
      return 'tag tag-ok';
    case 'draft':
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

export default function CommentingView({ orgId }: CommentingViewProps) {
  void orgId;
  const { lang, t } = useLang();
  const [messages, setMessages] = useState<OutboundMessage[]>([]);
  const [filter, setFilter] = useState<CommentFilter>('all');
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const [errorMsg, setErrorMsg] = useState('');

  const FILTERS: Array<{ label: string; value: CommentFilter }> = [
    { label: lang === 'vi' ? 'Tất cả' : 'All', value: 'all' },
    { label: 'Draft', value: 'draft' },
    { label: lang === 'vi' ? 'Đã duyệt' : 'Approved', value: 'approved' },
    { label: lang === 'vi' ? 'Đã gửi' : 'Sent', value: 'sent' },
    { label: lang === 'vi' ? 'Lỗi' : 'Failed', value: 'failed' },
    { label: lang === 'vi' ? 'Từ chối' : 'Rejected', value: 'rejected' },
  ];

  const load = async () => {
    setLoading(true);
    setErrorMsg('');
    try {
      const response = await getOutbox({ type: 'comment', limit: 200 });
      setMessages(response.messages ?? []);
    } catch (error) {
      setErrorMsg(error instanceof Error ? error.message : (lang === 'vi' ? 'Không tải được outbox comment.' : 'Failed to load comment outbox.'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const filtered = useMemo(
    () => (filter === 'all' ? messages : messages.filter((message) => message.status === filter)),
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

  const transition = async (id: number, action: 'approve' | 'reject' | 'delete') => {
    setErrorMsg('');
    try {
      if (action === 'approve') await approveOutbox(id);
      if (action === 'reject') await rejectOutbox(id);
      if (action === 'delete') await deleteOutbox(id);
      await load();
    } catch (error) {
      setErrorMsg(error instanceof Error ? error.message : (lang === 'vi' ? 'Không cập nhật được comment.' : 'Could not update comment.'));
    }
  };

  const today = new Date().toISOString().slice(0, 10);
  const stats = [
    { label: lang === 'vi' ? 'ĐÃ GỬI' : 'SENT', value: messages.filter((message) => message.status === 'sent').length },
    { label: lang === 'vi' ? 'HÔM NAY' : 'TODAY', value: messages.filter((message) => message.created_at?.startsWith(today)).length },
    { label: lang === 'vi' ? 'CHỜ DUYỆT' : 'PENDING', value: messages.filter((message) => message.status === 'draft' || message.status === 'approved').length },
    { label: lang === 'vi' ? 'TỔNG' : 'TOTAL', value: messages.length },
  ];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', gap: 16, flexWrap: 'wrap' }}>
        <div>
          <div className="eyebrow">
            <span className="dot" />
            OUTBOX
          </div>
          <h2 style={{ fontSize: 28, marginTop: 8 }}>{t.views.commentingTitle}</h2>
          <p style={{ color: 'var(--text-mute)', fontSize: 13.5, marginTop: 6 }}>{t.views.commentingSub}</p>
        </div>
        <div style={{ flex: 1 }} />
        <button type="button" className="btn btn-ghost btn-sm" onClick={() => void load()}>
          <RefreshCw size={13} />
          {t.common.refresh}
        </button>
      </div>

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
          <div style={{ padding: 16 }}>
            <div className="sidebar-section">{lang === 'vi' ? 'BỘ LỌC' : 'FILTERS'}</div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {FILTERS.map((item) => {
                const count = item.value === 'all' ? messages.length : messages.filter((message) => message.status === item.value).length;
                return (
                  <button
                    key={item.value}
                    type="button"
                    className={`nav-item ${filter === item.value ? 'is-active' : ''}`}
                    style={{ width: '100%', background: 'transparent', border: 0, textAlign: 'left' }}
                    onClick={() => setFilter(item.value)}
                  >
                    <span>{item.label}</span>
                    <span className="badge-num badge">{count}</span>
                  </button>
                );
              })}
            </div>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column' }}>
            <div style={{ padding: 16, borderBottom: '1px solid var(--line)' }}>
              <div className="eyebrow">{lang === 'vi' ? 'HÀNG ĐỢI COMMENT' : 'COMMENT QUEUE'}</div>
              <div style={{ marginTop: 6, fontSize: 13, color: 'var(--text-mute)' }}>
                {lang === 'vi'
                  ? `${filtered.length} comment đang được theo dõi`
                  : `${filtered.length} comments in the current queue`}
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
                  <div className="eyebrow">
                    <span className="dot" />
                    EMPTY
                  </div>
                  <h3>{lang === 'vi' ? 'Chưa có comment nào' : 'No comments yet'}</h3>
                  <p>
                    {lang === 'vi'
                      ? 'Khi agent draft comment cho leads, item sẽ vào hàng đợi tại đây.'
                      : 'Drafted comments from the agent will appear here.'}
                  </p>
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
                      <span className={statusTag(message.status)}>{message.status.toUpperCase()}</span>
                    </div>
                    <div style={{ color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {message.content || <span style={{ color: 'var(--text-faint)' }}>(empty)</span>}
                    </div>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                      <span className="mono" style={{ fontSize: 11, color: 'var(--text-faint)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {message.target_name || (lang === 'vi' ? 'Chưa có target' : 'No target')}
                      </span>
                      <span className="mono" style={{ fontSize: 11, color: 'var(--text-faint)' }}>
                        {message.created_at?.slice(5, 16) ?? '-'}
                      </span>
                    </div>
                  </button>
                ))
              )}
            </div>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column' }}>
            {selectedMessage ? (
              <>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: 16, borderBottom: '1px solid var(--line)' }}>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontWeight: 500, color: 'var(--text)' }}>
                      {selectedMessage.target_name || (lang === 'vi' ? 'Comment target' : 'Comment target')}
                    </div>
                    <div className="mono" style={{ fontSize: 11, color: 'var(--text-faint)', marginTop: 4 }}>
                      #{selectedMessage.account_id}
                    </div>
                  </div>
                  <span className={statusTag(selectedMessage.status)}>{selectedMessage.status.toUpperCase()}</span>
                </div>

                <div style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 16 }}>
                  <div className="card" style={{ padding: 16 }}>
                    <div className="eyebrow" style={{ marginBottom: 10 }}>{lang === 'vi' ? 'NỘI DUNG COMMENT' : 'COMMENT CONTENT'}</div>
                    <div style={{ color: 'var(--text)', lineHeight: 1.6, whiteSpace: 'pre-wrap' }}>
                      {selectedMessage.content || <span style={{ color: 'var(--text-faint)' }}>(empty)</span>}
                    </div>
                  </div>

                  <div className="card" style={{ padding: 16 }}>
                    <div className="eyebrow" style={{ marginBottom: 10 }}>{lang === 'vi' ? 'NGỮ CẢNH GỬI' : 'DELIVERY CONTEXT'}</div>
                    <dl style={{ display: 'grid', gap: 10 }}>
                      <div style={{ display: 'grid', gap: 4 }}>
                        <dt className="field-label">{lang === 'vi' ? 'Target' : 'Target'}</dt>
                        <dd style={{ margin: 0 }}>{selectedMessage.target_name || '-'}</dd>
                      </div>
                      <div style={{ display: 'grid', gap: 4 }}>
                        <dt className="field-label">{lang === 'vi' ? 'Context' : 'Context'}</dt>
                        <dd style={{ margin: 0, color: 'var(--text-mute)', lineHeight: 1.5 }}>
                          {selectedMessage.context || (lang === 'vi' ? 'Chưa có context mở rộng cho comment này.' : 'No extra context stored for this comment yet.')}
                        </dd>
                      </div>
                      <div style={{ display: 'grid', gap: 4 }}>
                        <dt className="field-label">{lang === 'vi' ? 'Ảnh media' : 'Media'}</dt>
                        <dd style={{ margin: 0 }}>{selectedMessage.image_path || '-'}</dd>
                      </div>
                    </dl>
                  </div>

                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                    {selectedMessage.status === 'draft' && (
                      <>
                        <button type="button" className="btn btn-primary btn-sm" onClick={() => void transition(selectedMessage.id, 'approve')}>
                          <Check size={13} />
                          {lang === 'vi' ? 'Duyệt gửi' : 'Approve'}
                        </button>
                        <button type="button" className="btn btn-ghost btn-sm" onClick={() => void transition(selectedMessage.id, 'reject')}>
                          <X size={13} />
                          {lang === 'vi' ? 'Từ chối' : 'Reject'}
                        </button>
                      </>
                    )}
                    {selectedMessage.target_url && (
                      <a className="btn btn-ghost btn-sm" href={selectedMessage.target_url} target="_blank" rel="noopener noreferrer">
                        <ExternalLink size={13} />
                        {lang === 'vi' ? 'Mở target' : 'Open target'}
                      </a>
                    )}
                    <button type="button" className="btn btn-ghost btn-sm" onClick={() => void transition(selectedMessage.id, 'delete')} style={{ color: 'var(--hot)' }}>
                      <Trash2 size={13} />
                      {lang === 'vi' ? 'Xóa item' : 'Delete item'}
                    </button>
                  </div>
                </div>
              </>
            ) : (
              <div className="empty" style={{ margin: 16 }}>
                <div className="eyebrow">
                  <span className="dot" />
                  SELECT
                </div>
                <h3>{lang === 'vi' ? 'Chọn một comment' : 'Select a comment'}</h3>
                <p>
                  {lang === 'vi'
                    ? 'Chọn item bên trái để duyệt, từ chối hoặc mở target thật.'
                    : 'Pick a queue item to approve, reject, or open the real target.'}
                </p>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
