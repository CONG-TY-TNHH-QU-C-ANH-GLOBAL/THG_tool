import { useState } from 'react';
import { Avatar, Badge, Row } from '../ui';
import { theme } from '../../constants/styles';
import { useThreads } from '../../hooks/useThreads';
import { Send } from 'lucide-react';

interface InboxViewProps { orgId: string; }

export default function InboxView({ orgId }: InboxViewProps) {
  const { threads, activeThread, setActiveId, messages, send } = useThreads(orgId);
  const [draft, setDraft] = useState('');

  const handleSend = async () => {
    const text = draft.trim();
    if (!text) return;
    setDraft('');
    await send(text);
  };

  return (
    <div style={{ display: 'flex', gap: 14, height: 420 }}>
      {/* Thread list */}
      <div style={{ width: 228, background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 12, overflowY: 'auto', flexShrink: 0 }}>
        <div style={{ padding: '11px 13px', borderBottom: `1px solid ${theme.border}` }}>
          <p style={{ color: theme.text, fontWeight: 500, fontSize: 13 }}>Tất cả hội thoại</p>
        </div>
        {threads.map(t => (
          <div key={t.id} onClick={() => setActiveId(t.id)} style={{
            padding: '11px 13px', borderBottom: `1px solid ${theme.borderAlt}`,
            cursor: 'pointer', background: activeThread?.id === t.id ? theme.border : 'transparent',
          }}>
            <Row style={{ gap: 7, marginBottom: 4 }}>
              <Avatar text={t.lead[0]} size={24} />
              <div style={{ flex: 1, minWidth: 0 }}>
                <Row style={{ justifyContent: 'space-between' }}>
                  <p style={{ color: theme.text, fontSize: 12, fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: 120 }}>{t.lead}</p>
                  {t.unread > 0 && (
                    <span style={{ background: theme.primary, color: '#fff', fontSize: 10, fontWeight: 700, width: 15, height: 15, borderRadius: '50%', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>{t.unread}</span>
                  )}
                </Row>
                <p style={{ color: theme.textFaint, fontSize: 10 }}>{t.agent}</p>
              </div>
            </Row>
            <p style={{ color: theme.textMuted, fontSize: 11, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', marginBottom: 5 }}>{t.last}</p>
            <Row style={{ justifyContent: 'space-between' }}>
              <Badge label={t.status} />
              <span style={{ color: '#4b5563', fontSize: 11 }}>{t.time}</span>
            </Row>
          </div>
        ))}
      </div>

      {/* Chat panel */}
      <div style={{ flex: 1, background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 12, display: 'flex', flexDirection: 'column' }}>
        {activeThread ? (
          <>
            <Row style={{ gap: 10, padding: '11px 15px', borderBottom: `1px solid ${theme.border}` }}>
              <Avatar text={activeThread.lead[0]} size={30} />
              <div>
                <p style={{ color: theme.text, fontWeight: 500, fontSize: 13 }}>{activeThread.lead}</p>
                <p style={{ color: theme.textFaint, fontSize: 11 }}>via {activeThread.agent}</p>
              </div>
              <div style={{ marginLeft: 8 }}><Badge label={activeThread.status} /></div>
            </Row>

            <div style={{ flex: 1, overflowY: 'auto', padding: 14, display: 'flex', flexDirection: 'column', gap: 10 }}>
              {messages.map((m, i) => (
                <div key={i} style={{ display: 'flex', justifyContent: m.from === 'agent' ? 'flex-end' : 'flex-start' }}>
                  <div style={{ maxWidth: '72%', padding: '9px 13px', borderRadius: 13, background: m.from === 'agent' ? theme.primary : theme.border, color: '#fff' }}>
                    {m.from === 'agent' && <p style={{ color: '#a5b4fc', fontSize: 10, marginBottom: 3 }}>{activeThread.agent}</p>}
                    <p style={{ fontSize: 13 }}>{m.text}</p>
                    <p style={{ fontSize: 10, color: m.from === 'agent' ? '#a5b4fc' : theme.textFaint, marginTop: 3, textAlign: 'right' }}>{m.time}</p>
                  </div>
                </div>
              ))}
            </div>

            <Row style={{ gap: 9, padding: '11px 14px', borderTop: `1px solid ${theme.border}` }}>
              <input
                value={draft}
                onChange={e => setDraft(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleSend()}
                placeholder="Nhập tin nhắn..."
                style={{ background: theme.border, border: `1px solid #374151`, borderRadius: 9, padding: '8px 12px', color: '#fff', fontSize: 13, outline: 'none', flex: 1 }}
              />
              <button onClick={handleSend} style={{ background: theme.primary, border: 'none', borderRadius: 9, padding: '8px 11px', cursor: 'pointer' }}>
                <Send size={14} color="#fff" />
              </button>
            </Row>
          </>
        ) : (
          <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <p style={{ color: theme.textMuted, fontSize: 13 }}>Chọn hội thoại để bắt đầu</p>
          </div>
        )}
      </div>
    </div>
  );
}
