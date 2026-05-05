import { useEffect, useState } from 'react';
import { Badge, Row } from '../ui';
import { theme, cardStyle, primaryBtn, secondaryBtn } from '../../constants/styles';
import { Check, ExternalLink, RefreshCw, Trash2, X } from 'lucide-react';
import { approveOutbox, deleteOutbox, getOutbox, OutboundMessage, rejectOutbox } from '../../services/outboxService';

interface PostingViewProps { orgId: string; }

type PostFilter = 'all' | 'sent' | 'draft' | 'approved' | 'failed' | 'rejected';

const FILTERS: { label: string; value: PostFilter }[] = [
  { label: 'Tất cả', value: 'all' },
  { label: 'Draft', value: 'draft' },
  { label: 'Đã duyệt', value: 'approved' },
  { label: 'Đã gửi', value: 'sent' },
  { label: 'Lỗi', value: 'failed' },
  { label: 'Từ chối', value: 'rejected' },
];

export default function PostingView({ orgId }: PostingViewProps) {
  const [messages, setMessages] = useState<OutboundMessage[]>([]);
  const [filter, setFilter] = useState<PostFilter>('all');
  const [loading, setLoading] = useState(true);
  const [msg, setMsg] = useState('');
  void orgId;

  const load = async () => {
    setLoading(true);
    try {
      const r = await getOutbox({ limit: 150 });
      setMessages((r.messages ?? []).filter(m => m.type === 'group_post' || m.type === 'profile_post'));
    } catch (err) {
      setMsg(err instanceof Error ? err.message : 'Không tải được outbox posting.');
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
      setMsg(err instanceof Error ? err.message : 'Không cập nhật được bài viết.');
    }
  };

  const filtered = filter === 'all' ? messages : messages.filter(m => m.status === filter);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <Row style={{ gap: 8, flexWrap: 'wrap' }}>
        {FILTERS.map(f => (
          <button
            key={f.value}
            onClick={() => setFilter(f.value)}
            style={{
              ...secondaryBtn({ padding: '6px 13px', fontSize: 12 }),
              background: filter === f.value ? theme.primary : undefined,
              color: filter === f.value ? '#fff' : undefined,
            }}
          >
            {f.label}
          </button>
        ))}
        <button onClick={load} style={{ ...primaryBtn({ padding: '6px 13px', fontSize: 12 }), marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 5 }}>
          <RefreshCw size={13} /> Làm mới
        </button>
      </Row>

      {msg && <p style={{ color: '#fca5a5', fontSize: 12 }}>{msg}</p>}

      {loading && (
        <div style={{ width: 24, height: 24, border: `3px solid ${theme.border}`, borderTopColor: theme.primary, borderRadius: '50%', animation: 'spin 0.7s linear infinite', margin: '40px auto' }} />
      )}

      {!loading && filtered.length === 0 && (
        <div style={{ textAlign: 'center', color: theme.textMuted, fontSize: 13, padding: 40 }}>Chưa có bài post trong hàng đợi thật.</div>
      )}

      {!loading && filtered.map(m => (
        <div key={m.id} style={cardStyle()}>
          <Row style={{ gap: 8, marginBottom: 8, flexWrap: 'wrap' }}>
            <span style={{ background: theme.border, color: '#d1d5db', padding: '2px 8px', borderRadius: 5, fontSize: 10 }}>
              {m.target_name || 'Chưa có target'}
            </span>
            <span style={{ color: theme.textFaint, fontSize: 11 }}>{m.created_at?.slice(0, 10)}</span>
            <Badge label={m.status} />
          </Row>
          <p style={{ color: '#d1d5db', fontSize: 13, lineHeight: 1.6, whiteSpace: 'pre-wrap' }}>
            {m.content || '(Trống)'}
          </p>
          {m.context && <p style={{ color: theme.textFaint, fontSize: 12, lineHeight: 1.5, marginTop: 8 }}>{m.context.slice(0, 240)}</p>}
          <Row style={{ gap: 8, marginTop: 12, paddingTop: 12, borderTop: `1px solid ${theme.border}` }}>
            <span style={{ fontSize: 11, color: theme.textFaint }}>Account #{m.account_id}</span>
            {m.status === 'draft' && (
              <>
                <button onClick={() => transition(m.id, 'approve')} style={secondaryBtn({ padding: '6px 10px', fontSize: 11, color: '#4ade80' })}><Check size={12} /> Duyệt</button>
                <button onClick={() => transition(m.id, 'reject')} style={secondaryBtn({ padding: '6px 10px', fontSize: 11, color: theme.red })}><X size={12} /> Từ chối</button>
              </>
            )}
            <button onClick={() => transition(m.id, 'delete')} style={secondaryBtn({ padding: '6px 10px', fontSize: 11, color: theme.textFaint })}><Trash2 size={12} /> Xóa</button>
            {m.target_url && (
              <a
                href={m.target_url}
                target="_blank"
                rel="noopener noreferrer"
                style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 4, color: theme.primaryLight, fontSize: 11, textDecoration: 'none' }}
              >
                <ExternalLink size={11} /> Mở Facebook
              </a>
            )}
          </Row>
        </div>
      ))}
    </div>
  );
}
