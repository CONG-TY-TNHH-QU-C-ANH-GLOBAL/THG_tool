import { useEffect, useState } from 'react';
import { Avatar, Badge, Row } from '../ui';
import { theme, cardStyle, secondaryBtn } from '../../constants/styles';
import { Check, ExternalLink, RefreshCw, Trash2, X } from 'lucide-react';
import { approveOutbox, deleteOutbox, getOutbox, OutboundMessage, rejectOutbox } from '../../services/outboxService';

interface CommentingViewProps { orgId: string; }

type CFilter = 'all' | 'draft' | 'approved' | 'sent' | 'failed' | 'rejected';

const FILTERS: { label: string; value: CFilter }[] = [
  { label: 'Tất cả', value: 'all' },
  { label: 'Draft', value: 'draft' },
  { label: 'Đã duyệt', value: 'approved' },
  { label: 'Đã comment', value: 'sent' },
  { label: 'Lỗi', value: 'failed' },
  { label: 'Từ chối', value: 'rejected' },
];

export default function CommentingView({ orgId }: CommentingViewProps) {
  const [messages, setMessages] = useState<OutboundMessage[]>([]);
  const [filter, setFilter] = useState<CFilter>('all');
  const [loading, setLoading] = useState(true);
  const [msg, setMsg] = useState('');
  void orgId;

  const load = async () => {
    setLoading(true);
    try {
      const r = await getOutbox({ type: 'comment', limit: 200 });
      setMessages(r.messages ?? []);
    } catch (err) {
      setMsg(err instanceof Error ? err.message : 'Không tải được outbox comment.');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const transition = async (id: number, action: 'approve' | 'reject' | 'delete') => {
    setMsg('');
    try {
      if (action === 'approve') await approveOutbox(id);
      if (action === 'reject') await rejectOutbox(id);
      if (action === 'delete') await deleteOutbox(id);
      await load();
    } catch (err) {
      setMsg(err instanceof Error ? err.message : 'Không cập nhật được comment.');
    }
  };

  const today = new Date().toISOString().slice(0, 10);
  const stats = [
    { label: 'Đã comment', value: messages.filter(m => m.status === 'sent').length, color: '#fff' },
    { label: 'Hôm nay', value: messages.filter(m => m.created_at?.startsWith(today)).length, color: '#4ade80' },
    { label: 'Chờ duyệt', value: messages.filter(m => m.status === 'draft' || m.status === 'approved').length, color: '#fbbf24' },
    { label: 'Tổng', value: messages.length, color: '#818cf8' },
  ];

  const filtered = filter === 'all' ? messages : messages.filter(m => m.status === filter);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', gap: 11 }}>
        {stats.map(s => (
          <div key={s.label} style={cardStyle()}>
            <p style={{ color: theme.textFaint, fontSize: 11, marginBottom: 4 }}>{s.label}</p>
            <p style={{ fontSize: 22, fontWeight: 700, color: s.color }}>{s.value}</p>
          </div>
        ))}
      </div>

      <Row style={{ gap: 8, flexWrap: 'wrap' }}>
        {FILTERS.map(f => (
          <button
            key={f.value}
            onClick={() => setFilter(f.value)}
            style={{
              padding: '5px 12px',
              borderRadius: 7,
              border: 'none',
              cursor: 'pointer',
              fontSize: 12,
              background: filter === f.value ? theme.primary : theme.surface,
              color: filter === f.value ? '#fff' : theme.textMuted,
            }}
          >
            {f.label}
          </button>
        ))}
        <button onClick={load} style={{ ...secondaryBtn({ padding: '6px 12px', fontSize: 12 }), marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 5 }}>
          <RefreshCw size={13} /> Làm mới
        </button>
      </Row>

      {msg && <p style={{ color: '#fca5a5', fontSize: 12 }}>{msg}</p>}

      {loading && (
        <div style={{ width: 24, height: 24, border: `3px solid ${theme.border}`, borderTopColor: theme.primary, borderRadius: '50%', animation: 'spin 0.7s linear infinite', margin: '40px auto' }} />
      )}

      {!loading && (
        <div style={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 12, overflow: 'hidden' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
            <thead>
              <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                {['Tài khoản', 'Đối tượng', 'Nội dung', 'Thời gian', 'Trạng thái', ''].map(h => (
                  <th key={h} style={{ padding: '9px 14px', textAlign: 'left', color: theme.textFaint, fontWeight: 500, fontSize: 11 }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {filtered.length === 0 ? (
                <tr><td colSpan={6} style={{ padding: 20, textAlign: 'center', color: theme.textFaint }}>Không có dữ liệu thật trong hàng đợi comment.</td></tr>
              ) : filtered.map(m => (
                <tr key={m.id} style={{ borderBottom: `1px solid ${theme.border}` }}>
                  <td style={{ padding: '9px 14px' }}>
                    <Row style={{ gap: 6 }}>
                      <Avatar text={String(m.account_id).slice(-1)} size={24} />
                      <span style={{ color: theme.text, fontSize: 12 }}>#{m.account_id}</span>
                    </Row>
                  </td>
                  <td style={{ padding: '9px 14px', color: '#d1d5db' }}>{m.target_name || '-'}</td>
                  <td style={{ padding: '9px 14px', color: theme.textMuted, maxWidth: 360 }}>
                    <p style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', margin: 0 }}>{m.content || '(Trống)'}</p>
                  </td>
                  <td style={{ padding: '9px 14px', color: theme.textFaint }}>{m.created_at?.slice(0, 10)}</td>
                  <td style={{ padding: '9px 14px' }}><Badge label={m.status} /></td>
                  <td style={{ padding: '9px 14px' }}>
                    <Row style={{ gap: 6, justifyContent: 'flex-end' }}>
                      {m.status === 'draft' && (
                        <>
                          <button onClick={() => transition(m.id, 'approve')} style={secondaryBtn({ padding: '5px 8px', fontSize: 11, color: '#4ade80' })}><Check size={12} /></button>
                          <button onClick={() => transition(m.id, 'reject')} style={secondaryBtn({ padding: '5px 8px', fontSize: 11, color: theme.red })}><X size={12} /></button>
                        </>
                      )}
                      {m.target_url && (
                        <a href={m.target_url} target="_blank" rel="noopener noreferrer" style={secondaryBtn({ padding: '5px 8px', fontSize: 11 })}>
                          <ExternalLink size={12} />
                        </a>
                      )}
                      <button onClick={() => transition(m.id, 'delete')} style={secondaryBtn({ padding: '5px 8px', fontSize: 11, color: theme.textFaint })}><Trash2 size={12} /></button>
                    </Row>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
