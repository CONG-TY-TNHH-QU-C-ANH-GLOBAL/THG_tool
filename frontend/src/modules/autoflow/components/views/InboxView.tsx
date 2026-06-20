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

export default function InboxView({ orgId }: Readonly<InboxViewProps>) {
  const { t } = useLang();
  const tv = t.inboxView;
  const { threads, activeThread, setActiveId, messages, send, isSending, refetch } = useThreads(orgId);
  const [draft, setDraft] = useState('');
  const [filter, setFilter] = useState<ThreadFilter>('all');

  const filteredThreads = useMemo(() => {
    if (filter === 'all') return threads;
    return threads.filter((thread) => thread.status === filter);
  }, [filter, threads]);

  const stats = {
    total: threads.length,
    active: threads.filter((thread) => thread.status === 'Active').length,
    pending: threads.filter((thread) => thread.status === 'Pending').length,
    converted: threads.filter((thread) => thread.status === 'Converted').length,
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
      <header style={{ display: 'flex', alignItems: 'flex-start', gap: 16, flexWrap: 'wrap' }}>
        <div>
          <div className="eyebrow"><span className="dot" />{tv.eyebrow}</div>
          <h2 style={{ fontSize: 24, marginTop: 6 }}>{t.views.inboxTitle}</h2>
          <p style={{ color: 'var(--text-mute)', fontSize: 13 }}>{t.views.inboxSub}</p>
        </div>
        <div style={{ flex: 1 }} />
        <button type="button" className="btn btn-ghost btn-sm" onClick={() => void refetch()}>
          <RefreshCw size={13} />
          {t.common.refresh}
        </button>
      </header>

      <div className="card" style={{ padding: 0, overflow: 'hidden', flex: 1, minHeight: 520 }}>
        <div style={{ display: 'grid', gridTemplateColumns: '320px 1fr', height: '100%' }}>
          <div style={{ borderRight: '1px solid var(--line)', overflowY: 'auto' }}>
            {threads.map((thread) => (
              <div
                key={thread.id}
                onClick={() => setActiveId(thread.id)}
                className={`table-row ${activeThread?.id === thread.id ? 'is-active' : ''}`}
                style={{ gridTemplateColumns: '1fr', cursor: 'pointer', padding: '14px 16px', borderTop: '1px solid var(--line)' }}
              >
                <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                  <span className="avatar">{(thread.lead[0] || 'L').toUpperCase()}</span>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      <span style={{ fontSize: 13.5, color: 'var(--text)', fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {thread.lead}
                      </span>
                      <span className={statusTag(thread.status)}>{thread.status.toUpperCase()}</span>
                      <span style={{ flex: 1 }} />
                      <span className="mono" style={{ fontSize: 10.5, color: 'var(--text-faint)' }}>{thread.time}</span>
                    </div>
                    <div style={{ fontSize: 12.5, color: 'var(--text-mute)', marginTop: 4, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {thread.last || tv.noRecentMessage}
                    </div>
                  </div>
                  {thread.unread > 0 && <span className="badge-num">{thread.unread}</span>}
                </div>
              </div>
            ))}
            {threads.length === 0 && (
              <div className="empty" style={{ margin: 16 }}>
                <div className="eyebrow"><span className="dot" />{tv.emptyEyebrow}</div>
                <h3>{tv.emptyTitle}</h3>
                <p style={{ fontSize: 12 }}>{tv.emptyDesc}</p>
              </div>
            )}
          </div>

          <div style={{ display: 'flex', flexDirection: 'column' }}>
            {activeThread ? (
              <>
                <div style={{ padding: '14px 20px', borderBottom: '1px solid var(--line)', display: 'flex', alignItems: 'center', gap: 12 }}>
                  <span className="avatar avatar-lg">{(activeThread.lead[0] || 'L').toUpperCase()}</span>
                  <div>
                    <div style={{ fontSize: 15, color: 'var(--text)', fontWeight: 500 }}>{activeThread.lead}</div>
                    <div className="mono" style={{ fontSize: 11, color: 'var(--text-faint)', marginTop: 2 }}>
                      {activeThread.agent ? `via ${activeThread.agent}` : tv.threadKind}
                    </div>
                  </div>
                  <div style={{ flex: 1 }} />
                  <span className={statusTag(activeThread.status)}>{activeThread.status.toUpperCase()}</span>
                </div>

                <div style={{ flex: 1, padding: 20, display: 'flex', flexDirection: 'column', gap: 14, overflowY: 'auto' }}>
                  {messages.length === 0 ? (
                    <div className="empty" style={{ margin: 'auto 0' }}>
                      <div className="eyebrow"><span className="dot" />{tv.conversationEyebrow}</div>
                      <h3>{tv.conversationEmptyTitle}</h3>
                      <p>{tv.conversationEmptyDesc}</p>
                    </div>
                  ) : (
                    messages.map((message, index) => {
                      const mine = message.from !== 'lead';
                      return (
                        <div key={`${message.time}-${index}`} style={{ display: 'flex', justifyContent: mine ? 'flex-end' : 'flex-start' }}>
                          <div
                            style={{
                              maxWidth: '70%',
                              padding: '10px 14px',
                              borderRadius: 12,
                              fontSize: 13.5,
                              lineHeight: 1.5,
                              background: 'var(--bg-elev-2)',
                              border: '1px solid var(--line)',
                              color: 'var(--text)',
                            }}
                          >
                            <div className="mono" style={{ fontSize: 10, color: 'var(--text-faint)', letterSpacing: '0.08em', marginBottom: 4 }}>
                              {message.from === 'agent' ? 'AGENT' : 'YOU'} · {message.time}
                            </div>
                            {message.text}
                          </div>
                        </div>
                      );
                    })
                  )}
                </div>

                <div style={{ borderTop: '1px solid var(--line)', padding: 14, display: 'flex', gap: 10 }}>
                  <input
                    className="input"
                    value={draft}
                    onChange={(event) => setDraft(event.target.value)}
                    onKeyDown={(event) => event.key === 'Enter' && !event.shiftKey && (event.preventDefault(), void handleSend())}
                    placeholder={tv.placeholderInput}
                    disabled={isSending}
                  />
                  <button
                    type="button"
                    className="btn btn-primary btn-sm"
                    onClick={() => void handleSend()}
                    disabled={isSending || !draft.trim()}
                  >
                    Phê duyệt & gửi
                  </button>
                </div>
              </>
            ) : (
              <div className="empty" style={{ margin: 24 }}>
                <div className="eyebrow"><span className="dot" />{tv.selectEyebrow}</div>
                <h3>{tv.selectTitle}</h3>
                <p>{tv.selectDesc}</p>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
