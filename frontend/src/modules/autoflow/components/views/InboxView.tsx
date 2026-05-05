'use client';

import { useMemo, useState } from 'react';
import { RefreshCw, Send } from 'lucide-react';
import { useThreads } from '../../hooks/useThreads';
import type { ThreadStatus } from '../../types';
import { useLang } from '../../i18n/useLang';

interface InboxViewProps {
  orgId: string;
}

type ThreadFilter = 'all' | ThreadStatus;

function statusTag(status: string): string {
  switch (status) {
    case 'Hot':
      return 'tag tag-hot';
    case 'Warm':
      return 'tag tag-warm';
    case 'Cold':
      return 'tag tag-cold';
    case 'Active':
      return 'tag tag-ok';
    case 'Converted':
      return 'tag tag-cold';
    case 'Pending':
      return 'tag tag-warm';
    default:
      return 'tag tag-mute';
  }
}

function threadFilterLabel(filter: ThreadFilter, lang: 'vi' | 'en') {
  if (filter === 'all') return lang === 'vi' ? 'Tất cả' : 'All';
  return filter;
}

export default function InboxView({ orgId }: InboxViewProps) {
  const { lang, t } = useLang();
  const { threads, activeThread, setActiveId, messages, send, isSending, refetch } = useThreads(orgId);
  const [draft, setDraft] = useState('');
  const [filter, setFilter] = useState<ThreadFilter>('all');

  const filteredThreads = useMemo(() => {
    if (filter === 'all') return threads;
    return threads.filter(thread => thread.status === filter);
  }, [filter, threads]);

  const stats = {
    total: threads.length,
    active: threads.filter(thread => thread.status === 'Active').length,
    pending: threads.filter(thread => thread.status === 'Pending').length,
    converted: threads.filter(thread => thread.status === 'Converted').length,
  };

  const filters: ThreadFilter[] = ['all', 'Active', 'Pending', 'Converted'];

  const handleSend = async () => {
    const text = draft.trim();
    if (!text || isSending) return;
    setDraft('');
    await send(text);
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16, height: 'calc(100vh - 56px - 48px)' }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', gap: 16, flexWrap: 'wrap' }}>
        <div>
          <div className="eyebrow">
            <span className="dot" />
            MESSENGER
          </div>
          <h2 style={{ fontSize: 24, marginTop: 6 }}>{t.views.inboxTitle}</h2>
          <p style={{ color: 'var(--text-mute)', fontSize: 13 }}>{t.views.inboxSub}</p>
        </div>
        <div style={{ flex: 1 }} />
        <button type="button" className="btn btn-ghost btn-sm" onClick={() => void refetch()}>
          <RefreshCw size={13} />
          {t.common.refresh}
        </button>
      </div>

      <div className="stats-grid">
        <div className="stat">
          <div className="stat-label">{lang === 'vi' ? 'TỔNG THREAD' : 'TOTAL THREADS'}</div>
          <div className="stat-value tabular">{stats.total}</div>
        </div>
        <div className="stat">
          <div className="stat-label">ACTIVE</div>
          <div className="stat-value tabular" style={{ color: 'var(--ok)' }}>{stats.active}</div>
        </div>
        <div className="stat">
          <div className="stat-label">PENDING</div>
          <div className="stat-value tabular" style={{ color: 'var(--warn)' }}>{stats.pending}</div>
        </div>
        <div className="stat">
          <div className="stat-label">CONVERTED</div>
          <div className="stat-value tabular" style={{ color: 'var(--info)' }}>{stats.converted}</div>
        </div>
      </div>

      <div className="card" style={{ padding: 0, overflow: 'hidden', flex: 1, minHeight: 520 }}>
        <div className="three-pane" style={{ height: '100%' }}>
          <div style={{ padding: 16 }}>
            <div className="sidebar-section">{lang === 'vi' ? 'BỘ LỌC' : 'FILTERS'}</div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              {filters.map(item => {
                const count = item === 'all' ? threads.length : threads.filter(thread => thread.status === item).length;
                return (
                  <button
                    key={item}
                    type="button"
                    className={`nav-item ${filter === item ? 'is-active' : ''}`}
                    style={{ background: 'transparent', border: 0, textAlign: 'left' }}
                    onClick={() => setFilter(item)}
                  >
                    <span>{threadFilterLabel(item, lang)}</span>
                    <span className="badge-num badge">{count}</span>
                  </button>
                );
              })}
            </div>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column' }}>
            <div style={{ padding: 16, borderBottom: '1px solid var(--line)' }}>
              <div className="eyebrow">{lang === 'vi' ? 'DANH SÁCH THREAD' : 'THREAD LIST'}</div>
              <div style={{ marginTop: 6, fontSize: 13, color: 'var(--text-mute)' }}>
                {lang === 'vi'
                  ? `${filteredThreads.length} hội thoại đang được quan sát`
                  : `${filteredThreads.length} conversations in view`}
              </div>
            </div>

            <div style={{ overflowY: 'auto', flex: 1 }}>
              {filteredThreads.length === 0 ? (
                <div className="empty" style={{ margin: 16 }}>
                  <div className="eyebrow">
                    <span className="dot" />
                    EMPTY
                  </div>
                  <h3>{lang === 'vi' ? 'Chưa có thread' : 'No threads yet'}</h3>
                  <p style={{ fontSize: 12 }}>
                    {lang === 'vi'
                      ? 'Khi agent inbox lead, conversation sẽ xuất hiện ở đây.'
                      : 'Threads will appear once the agent starts an outreach.'}
                  </p>
                </div>
              ) : (
                filteredThreads.map(thread => (
                  <button
                    key={thread.id}
                    type="button"
                    onClick={() => setActiveId(thread.id)}
                    className={`nav-item ${activeThread?.id === thread.id ? 'is-active' : ''}`}
                    style={{
                      width: '100%',
                      flexDirection: 'column',
                      alignItems: 'stretch',
                      gap: 6,
                      padding: 14,
                      background: 'transparent',
                      border: 0,
                      borderBottom: '1px solid var(--line)',
                      borderRadius: 0,
                      textAlign: 'left',
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      <span className="avatar avatar-sm">{(thread.lead[0] || 'L').toUpperCase()}</span>
                      <span style={{ flex: 1, fontWeight: 500, color: 'var(--text)', fontSize: 13, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {thread.lead}
                      </span>
                      {thread.unread > 0 && <span className="badge-num badge">{thread.unread}</span>}
                    </div>
                    <div style={{ fontSize: 12, color: 'var(--text-mute)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', paddingLeft: 30 }}>
                      {thread.last || (lang === 'vi' ? 'Chưa có tin nhắn gần đây' : 'No recent message')}
                    </div>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', paddingLeft: 30 }}>
                      <span className={statusTag(thread.status)}>{thread.status}</span>
                      <span className="mono" style={{ fontSize: 11, color: 'var(--text-faint)' }}>{thread.time}</span>
                    </div>
                  </button>
                ))
              )}
            </div>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column' }}>
            {activeThread ? (
              <>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: 16, borderBottom: '1px solid var(--line)' }}>
                  <span className="avatar">{(activeThread.lead[0] || 'L').toUpperCase()}</span>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontWeight: 500, color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {activeThread.lead}
                    </div>
                    <div className="mono" style={{ fontSize: 11, color: 'var(--text-faint)' }}>
                      {activeThread.agent ? `via ${activeThread.agent}` : (lang === 'vi' ? 'Facebook thread' : 'Facebook thread')}
                    </div>
                  </div>
                  <span className={statusTag(activeThread.status)}>{activeThread.status}</span>
                </div>

                <div style={{ flex: 1, overflowY: 'auto', padding: 16, display: 'flex', flexDirection: 'column', gap: 10 }}>
                  {messages.length === 0 ? (
                    <div className="empty" style={{ margin: 'auto 0' }}>
                      <div className="eyebrow">
                        <span className="dot" />
                        THREAD
                      </div>
                      <h3>{lang === 'vi' ? 'Chưa có tin nhắn' : 'No messages yet'}</h3>
                      <p>
                        {lang === 'vi'
                          ? 'Khi lead phản hồi hoặc agent bắt đầu hội thoại, transcript sẽ xuất hiện tại đây.'
                          : 'The transcript will appear here once the lead or agent starts the conversation.'}
                      </p>
                    </div>
                  ) : (
                    messages.map((message, index) => (
                      <div key={`${message.time}-${index}`} style={{ display: 'flex', justifyContent: message.from === 'agent' ? 'flex-end' : 'flex-start' }}>
                        <div
                          style={{
                            maxWidth: '72%',
                            padding: '10px 14px',
                            borderRadius: 12,
                            background: message.from === 'agent' ? 'var(--accent)' : 'var(--bg-elev-2)',
                            color: message.from === 'agent' ? 'var(--accent-ink)' : 'var(--text)',
                            border: message.from === 'agent' ? 'none' : '1px solid var(--line)',
                            fontSize: 13.5,
                          }}
                        >
                          {message.from === 'agent' && (
                            <div className="mono" style={{ fontSize: 10, marginBottom: 4, opacity: 0.7 }}>
                              {activeThread.agent || 'Agent'}
                            </div>
                          )}
                          <div>{message.text}</div>
                          <div className="mono" style={{ fontSize: 10, marginTop: 4, opacity: 0.6, textAlign: 'right' }}>{message.time}</div>
                        </div>
                      </div>
                    ))
                  )}
                </div>

                <div style={{ display: 'flex', gap: 8, padding: 16, borderTop: '1px solid var(--line)' }}>
                  <input
                    className="input"
                    value={draft}
                    onChange={event => setDraft(event.target.value)}
                    onKeyDown={event => event.key === 'Enter' && !event.shiftKey && (event.preventDefault(), void handleSend())}
                    placeholder={lang === 'vi' ? 'Nhập tin nhắn...' : 'Type a message...'}
                    disabled={isSending}
                  />
                  <button type="button" className="btn btn-primary btn-icon" onClick={() => void handleSend()} aria-label="Send" disabled={isSending || !draft.trim()}>
                    <Send size={14} />
                  </button>
                </div>
              </>
            ) : (
              <div className="empty" style={{ margin: 24 }}>
                <div className="eyebrow">
                  <span className="dot" />
                  SELECT
                </div>
                <h3>{lang === 'vi' ? 'Chọn hội thoại' : 'Select a thread'}</h3>
                <p>{lang === 'vi' ? 'Chọn một thread bên trái để bắt đầu.' : 'Pick a thread on the left to start.'}</p>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
