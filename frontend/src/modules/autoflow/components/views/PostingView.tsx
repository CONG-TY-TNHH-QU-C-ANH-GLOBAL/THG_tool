import { useState, useEffect } from 'react';
import { Badge, Row } from '../ui';
import { theme, cardStyle, primaryBtn, secondaryBtn } from '../../constants/styles';
import { ExternalLink, Plus } from 'lucide-react';
import { getOutbox, OutboundMessage } from '../../services/outboxService';

interface PostingViewProps { orgId: string; }

type PostFilter = 'all' | 'sent' | 'draft' | 'failed';

const FILTERS: { label: string; value: PostFilter }[] = [
  { label: 'Tất cả', value: 'all' },
  { label: 'Đã gửi', value: 'sent' },
  { label: 'Chờ duyệt', value: 'draft' },
  { label: 'Lỗi', value: 'failed' },
];

export default function PostingView({ orgId }: PostingViewProps) {
  const [messages, setMessages] = useState<OutboundMessage[]>([]);
  const [filter, setFilter] = useState<PostFilter>('all');
  const [loading, setLoading] = useState(true);
  void orgId;

  useEffect(() => {
    getOutbox({ limit: 100 })
      .then(r => setMessages(r.messages ?? []))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const filtered = filter === 'all' ? messages : messages.filter(m => m.status === filter);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <Row style={{ gap: 8 }}>
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
        <button style={{ ...primaryBtn({ padding: '6px 13px', fontSize: 12 }), marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 5 }}>
          <Plus size={13} /> Tạo bài viết
        </button>
      </Row>

      {loading && (
        <div style={{ width: 24, height: 24, border: `3px solid ${theme.border}`, borderTopColor: theme.primary, borderRadius: '50%', animation: 'spin 0.7s linear infinite', margin: '40px auto' }} />
      )}

      {!loading && filtered.length === 0 && (
        <div style={{ textAlign: 'center', color: theme.textMuted, fontSize: 13, padding: 40 }}>Chưa có bài viết nào</div>
      )}

      {!loading && filtered.map(m => (
        <div key={m.id} style={cardStyle()}>
          <Row style={{ gap: 8, marginBottom: 8 }}>
            <span style={{ background: theme.border, color: '#d1d5db', padding: '2px 8px', borderRadius: 5, fontSize: 10 }}>
              {m.target_name || 'Không rõ'}
            </span>
            <span style={{ color: theme.textFaint, fontSize: 11 }}>{m.created_at.slice(0, 10)}</span>
            <Badge label={m.status} />
          </Row>
          <p style={{ color: '#d1d5db', fontSize: 13, lineHeight: 1.6 }}>
            {m.content.length > 200 ? m.content.slice(0, 200) + '...' : m.content}
          </p>
          <Row style={{ gap: 20, marginTop: 12, paddingTop: 12, borderTop: `1px solid ${theme.border}` }}>
            <span style={{ fontSize: 11, color: theme.textFaint }}>{m.type}</span>
            {m.target_url && (
              <a
                href={m.target_url}
                target="_blank"
                rel="noopener noreferrer"
                style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 4, color: theme.primaryLight, fontSize: 11, textDecoration: 'none' }}
              >
                <ExternalLink size={11} /> Xem bài
              </a>
            )}
          </Row>
        </div>
      ))}
    </div>
  );
}
