import { useEffect, useMemo, useRef, useState } from 'react';
import { Bot, CheckCircle, Cpu, RefreshCw, Send, UserRound } from 'lucide-react';
import { theme } from '../../constants/styles';
import { useWorkspaces } from '../../hooks/useWorkspaces';
import { getAgentHistory, sendAgentPrompt } from '../../services/agentChatService';

interface WorkspaceChatViewProps { orgId: string; }

type ChatRole = 'user' | 'assistant' | 'system';

interface ChatMessage {
  id: string;
  role: ChatRole;
  text: string;
  time: string;
  ok?: boolean;
}

function nowLabel() {
  return new Date().toLocaleTimeString('vi', { hour: '2-digit', minute: '2-digit' });
}

export default function WorkspaceChatView({ orgId }: WorkspaceChatViewProps) {
  void orgId;
  const { workspaces, refresh } = useWorkspaces();
  const [accountId, setAccountId] = useState<number | ''>('');
  const [draft, setDraft] = useState('');
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [loadingHistory, setLoadingHistory] = useState(true);
  const [sending, setSending] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  const activeAccounts = useMemo(() => workspaces.filter(w => w.running || w.loggedIn), [workspaces]);
  const selectedAccount = activeAccounts.find(w => w.accountId === accountId);

  useEffect(() => {
    if (accountId !== '' || activeAccounts.length === 0) return;
    const loggedIn = activeAccounts.find(w => w.loggedIn);
    setAccountId((loggedIn ?? activeAccounts[0]).accountId);
  }, [accountId, activeAccounts]);

  useEffect(() => {
    let cancelled = false;
    setLoadingHistory(true);
    getAgentHistory(18)
      .then(items => {
        if (cancelled) return;
        const next: ChatMessage[] = [];
        for (const item of items) {
          const time = new Date(item.createdAt).toLocaleTimeString('vi', { hour: '2-digit', minute: '2-digit' });
          next.push({ id: `${item.id}-u`, role: 'user', text: item.userPrompt, time, ok: item.success });
          next.push({ id: `${item.id}-a`, role: 'assistant', text: item.aiResponse, time, ok: item.success });
        }
        setMessages(next);
      })
      .catch(() => setMessages([]))
      .finally(() => { if (!cancelled) setLoadingHistory(false); });
    return () => { cancelled = true; };
  }, []);

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: 'smooth' });
  }, [messages, sending]);

  const handleSend = async () => {
    const text = draft.trim();
    if (!text || sending) return;
    setDraft('');
    const userMsg: ChatMessage = { id: `u-${Date.now()}`, role: 'user', text, time: nowLabel(), ok: true };
    setMessages(prev => [...prev, userMsg]);
    setSending(true);
    try {
      const response = await sendAgentPrompt(text, accountId === '' ? undefined : accountId);
      setMessages(prev => [...prev, { id: `a-${Date.now()}`, role: 'assistant', text: response, time: nowLabel(), ok: true }]);
      void refresh();
    } catch (e) {
      setMessages(prev => [...prev, {
        id: `e-${Date.now()}`,
        role: 'system',
        text: e instanceof Error ? e.message : 'Agent chưa phản hồi',
        time: nowLabel(),
        ok: false,
      }]);
    } finally {
      setSending(false);
    }
  };

  return (
    <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) 280px', gap: 14, minHeight: 'calc(100vh - 126px)' }}>
      <div style={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 10, display: 'flex', flexDirection: 'column', minHeight: 520 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '12px 14px', borderBottom: `1px solid ${theme.border}` }}>
          <div style={{ width: 30, height: 30, borderRadius: 8, background: theme.primary, display: 'grid', placeItems: 'center', flexShrink: 0 }}>
            <Bot size={16} color="#fff" />
          </div>
          <div style={{ minWidth: 0 }}>
            <p style={{ color: theme.text, fontSize: 14, fontWeight: 700 }}>Agent Chat</p>
            <p style={{ color: theme.textFaint, fontSize: 11, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {selectedAccount ? selectedAccount.accountName : 'Workspace'}
            </p>
          </div>
          <button
            onClick={() => void refresh()}
            style={{ marginLeft: 'auto', background: 'transparent', border: `1px solid ${theme.border}`, borderRadius: 8, color: theme.textMuted, padding: '7px 9px', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 5, fontSize: 12 }}
          >
            <RefreshCw size={12} /> Refresh
          </button>
        </div>

        <div ref={scrollRef} style={{ flex: 1, overflowY: 'auto', padding: 16, display: 'flex', flexDirection: 'column', gap: 12 }}>
          {loadingHistory && (
            <div style={{ color: theme.textMuted, fontSize: 13, display: 'flex', alignItems: 'center', gap: 8 }}>
              <RefreshCw size={13} className="spin" /> Đang tải lịch sử
            </div>
          )}
          {!loadingHistory && messages.length === 0 && (
            <div style={{ height: '100%', display: 'grid', placeItems: 'center', color: theme.textMuted, fontSize: 13 }}>
              Chưa có lệnh nào
            </div>
          )}
          {messages.map(m => {
            const isUser = m.role === 'user';
            const isSystem = m.role === 'system';
            return (
              <div key={m.id} style={{ display: 'flex', justifyContent: isUser ? 'flex-end' : 'flex-start' }}>
                <div style={{
                  maxWidth: '76%',
                  background: isUser ? theme.primary : isSystem ? '#7f1d1d55' : theme.border,
                  border: `1px solid ${isSystem ? '#ef444466' : isUser ? '#6366f1' : '#374151'}`,
                  color: '#fff',
                  borderRadius: 12,
                  padding: '10px 12px',
                  whiteSpace: 'pre-wrap',
                  lineHeight: 1.5,
                  fontSize: 13,
                }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 5, color: isUser ? '#c7d2fe' : theme.textFaint, fontSize: 11 }}>
                    {isUser ? <UserRound size={12} /> : <Bot size={12} />}
                    <span>{isUser ? 'Bạn' : isSystem ? 'System' : 'Agent'}</span>
                    <span style={{ marginLeft: 'auto' }}>{m.time}</span>
                  </div>
                  {m.text}
                </div>
              </div>
            );
          })}
          {sending && (
            <div style={{ color: theme.textMuted, fontSize: 13, display: 'flex', alignItems: 'center', gap: 8 }}>
              <RefreshCw size={13} className="spin" /> Agent đang xử lý
            </div>
          )}
        </div>

        <div style={{ padding: 12, borderTop: `1px solid ${theme.border}`, display: 'flex', gap: 10, alignItems: 'flex-end' }}>
          <textarea
            value={draft}
            onChange={e => setDraft(e.target.value)}
            onKeyDown={e => {
              if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
                e.preventDefault();
                void handleSend();
              }
            }}
            placeholder="Nhập lệnh scraper..."
            rows={3}
            style={{ flex: 1, resize: 'none', background: theme.border, border: '1px solid #374151', borderRadius: 9, color: '#fff', outline: 'none', padding: '10px 12px', fontSize: 13, lineHeight: 1.5 }}
          />
          <button
            onClick={() => void handleSend()}
            disabled={sending || !draft.trim()}
            style={{ width: 42, height: 42, display: 'grid', placeItems: 'center', background: theme.primary, border: 'none', borderRadius: 9, cursor: sending || !draft.trim() ? 'not-allowed' : 'pointer', opacity: sending || !draft.trim() ? 0.55 : 1 }}
          >
            <Send size={16} color="#fff" />
          </button>
        </div>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        <div style={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 10, padding: 12 }}>
          <p style={{ color: theme.text, fontWeight: 700, fontSize: 13, marginBottom: 10 }}>Account</p>
          <select
            value={accountId}
            onChange={e => setAccountId(e.target.value ? Number(e.target.value) : '')}
            style={{ width: '100%', background: theme.border, border: '1px solid #374151', color: '#fff', borderRadius: 8, padding: '9px 10px', outline: 'none', fontSize: 12 }}
          >
            <option value="">Tự chọn</option>
            {activeAccounts.map(w => (
              <option key={w.accountId} value={w.accountId}>{w.email ? `${w.accountName} · ${w.email}` : w.accountName}</option>
            ))}
          </select>
          {activeAccounts.length === 0 && (
            <div style={{ marginTop: 10, border: '1px solid #22d3ee44', background: '#08334433', borderRadius: 8, padding: 10 }}>
              <p style={{ display: 'flex', alignItems: 'center', gap: 6, color: '#67e8f9', fontSize: 11, fontWeight: 800, letterSpacing: '0.08em', marginBottom: 5 }}>
                <Cpu size={13} /> CYBERTECH NOTE
              </p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.5 }}>
                Chưa có Facebook workspace. Tạo một phiên Browser trước để agent có session thật khi chạy crawler.
              </p>
            </div>
          )}
          <div style={{ marginTop: 10, display: 'flex', flexDirection: 'column', gap: 7 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', color: theme.textMuted, fontSize: 12 }}>
              <span>Running</span><span>{selectedAccount?.running ? 'yes' : 'no'}</span>
            </div>
            <div style={{ display: 'flex', justifyContent: 'space-between', color: theme.textMuted, fontSize: 12 }}>
              <span>Session</span><span style={{ color: selectedAccount?.loggedIn ? '#4ade80' : theme.textMuted }}>{selectedAccount?.loggedIn ? 'saved' : 'pending'}</span>
            </div>
            <div style={{ display: 'flex', justifyContent: 'space-between', color: theme.textMuted, fontSize: 12, gap: 8 }}>
              <span>Email</span><span style={{ color: selectedAccount?.email ? theme.text : theme.textFaint, overflow: 'hidden', textOverflow: 'ellipsis' }}>{selectedAccount?.email || 'chưa xác minh'}</span>
            </div>
            <div style={{ display: 'flex', justifyContent: 'space-between', color: theme.textMuted, fontSize: 12, gap: 8 }}>
              <span>FB ID</span><span style={{ color: theme.text, overflow: 'hidden', textOverflow: 'ellipsis' }}>{selectedAccount?.fbUserId ?? '-'}</span>
            </div>
          </div>
        </div>

        <div style={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 10, padding: 12 }}>
          <p style={{ color: theme.text, fontWeight: 700, fontSize: 13, marginBottom: 10 }}>Runtime</p>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, color: theme.textMuted, fontSize: 12 }}>
            <CheckCircle size={14} color={activeAccounts.length > 0 ? '#4ade80' : theme.textMuted} />
            <span>{activeAccounts.length} browser workspace</span>
          </div>
        </div>
      </div>
    </div>
  );
}
