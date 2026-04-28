import { useState, useEffect } from 'react';
import { Avatar, Badge, Row } from '../ui';
import { theme, cardStyle, primaryBtn, secondaryBtn } from '../../constants/styles';
import { MessageCircle, Zap, Check, X } from 'lucide-react';
import { getOutbox, OutboundMessage } from '../../services/outboxService';

interface CommentingViewProps { orgId: string; }

type CFilter = 'all' | 'draft' | 'sent';

export default function CommentingView({ orgId }: CommentingViewProps) {
  const [messages, setMessages] = useState<OutboundMessage[]>([]);
  const [filter, setFilter] = useState<CFilter>('all');
  const [loading, setLoading] = useState(true);
  void orgId;

  useEffect(() => {
    getOutbox({ type: 'comment', limit: 200 })
      .then(r => setMessages(r.messages ?? []))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const today = new Date().toISOString().slice(0, 10);
  const sentCount = messages.filter(m => m.status === 'sent').length;
  const todayCount = messages.filter(m => m.created_at.startsWith(today)).length;
  const pending = messages.filter(m => m.status === 'draft' || m.status === 'approved').length;

  const filtered =
    filter === 'all' ? messages :
    filter === 'draft' ? messages.filter(m => m.status === 'draft' || m.status === 'approved') :
    messages.filter(m => m.status === 'sent');

  const stats = [
    { l: 'Đã comment', v: sentCount, c: '#fff' },
    { l: 'Hôm nay', v: todayCount, c: '#4ade80' },
    { l: 'Đang chờ', v: pending, c: '#fbbf24' },
    { l: 'Tổng', v: messages.length, c: '#818cf8' },
  ];

  const filterBtns: { label: string; value: CFilter }[] = [
    { label: 'Tất cả', value: 'all' },
    { label: 'Đang chờ', value: 'draft' },
    { label: 'Đã gửi', value: 'sent' },
  ];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      {/* Stats */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', gap: 11 }}>
        {stats.map(s => (
          <div key={s.l} style={cardStyle()}>
            <p style={{ color: theme.textFaint, fontSize: 11, marginBottom: 4 }}>{s.l}</p>
            <p style={{ fontSize: 22, fontWeight: 700, color: s.c }}>{s.v}</p>
          </div>
        ))}
      </div>

      {/* Filter bar */}
      <Row style={{ gap: 8 }}>
        {filterBtns.map(f => (
          <button
            key={f.value}
            onClick={() => setFilter(f.value)}
            style={{
              padding: '5px 12px', borderRadius: 7, border: 'none', cursor: 'pointer', fontSize: 12,
              background: filter === f.value ? theme.primary : theme.surface,
              color: filter === f.value ? '#fff' : theme.textMuted,
            }}
          >
            {f.label}
          </button>
        ))}
        <button style={{ ...primaryBtn({ padding: '6px 13px', fontSize: 12 }), marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 5 }}>
          <Zap size={13} /> Auto-comment tất cả
        </button>
      </Row>

      {/* Loading */}
      {loading && (
        <div style={{ width: 24, height: 24, border: `3px solid ${theme.border}`, borderTopColor: theme.primary, borderRadius: '50%', animation: 'spin 0.7s linear infinite', margin: '40px auto' }} />
      )}

      {/* Table */}
      {!loading && (
        <div style={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 12, overflow: 'hidden' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
            <thead>
              <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                {['Tài khoản', 'Đối tượng', 'Nội dung', 'Thời gian', 'Trạng thái'].map(h => (
                  <th key={h} style={{ padding: '9px 14px', textAlign: 'left', color: theme.textFaint, fontWeight: 500, fontSize: 11 }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {filtered.length === 0 ? (
                <tr>
                  <td colSpan={5} style={{ padding: 20, textAlign: 'center', color: theme.textFaint }}>Không có dữ liệu</td>
                </tr>
              ) : filtered.map((m, i) => (
                <tr key={i} style={{ borderBottom: `1px solid ${theme.border}` }}>
                  <td style={{ padding: '9px 14px' }}>
                    <Row style={{ gap: 6 }}>
                      <Avatar text={String(m.account_id).slice(-1)} size={24} />
                      <span style={{ color: theme.text, fontSize: 12 }}>{m.account_id}</span>
                    </Row>
                  </td>
                  <td style={{ padding: '9px 14px', color: '#d1d5db' }}>{m.target_name || '-'}</td>
                  <td style={{ padding: '9px 14px', color: theme.textMuted, maxWidth: 260 }}>
                    <p style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', margin: 0 }}>{m.content}</p>
                  </td>
                  <td style={{ padding: '9px 14px', color: theme.textFaint }}>{m.created_at.slice(0, 10)}</td>
                  <td style={{ padding: '9px 14px' }}><Badge label={m.status} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Template card */}
      <div style={cardStyle()}>
        <Row style={{ gap: 10, marginBottom: 16 }}>
          <MessageCircle size={16} color={theme.primaryLight} />
          <p style={{ color: theme.text, fontWeight: 600, fontSize: 13 }}>Template comment AI</p>
        </Row>
        {[
          { label: 'Hot lead', text: 'Dạ {tên} ơi, còn hàng ạ! Em inbox tư vấn chi tiết nhé 😊' },
          { label: 'Warm lead', text: 'Bạn {tên} ơi mình có chương trình ưu đãi đặc biệt, inbox mình nhé!' },
          { label: 'Cold lead', text: 'Chào {tên}, nếu cần tư vấn thêm mình sẵn sàng hỗ trợ ạ 🙏' },
        ].map(t => (
          <div key={t.label} style={{ marginBottom: 12 }}>
            <Row style={{ gap: 7, marginBottom: 6 }}>
              <Badge label={t.label === 'Hot lead' ? 'Hot' : t.label === 'Warm lead' ? 'Warm' : 'Cold'} />
            </Row>
            <Row style={{ gap: 8 }}>
              <input
                defaultValue={t.text}
                style={{ flex: 1, background: theme.border, border: `1px solid #374151`, borderRadius: 8, padding: '8px 12px', color: '#fff', fontSize: 12, outline: 'none' }}
              />
              <button style={secondaryBtn({ padding: '7px 12px', fontSize: 11 })}><Check size={12} /></button>
              <button style={{ ...secondaryBtn({ padding: '7px 12px', fontSize: 11 }), color: theme.red }}><X size={12} /></button>
            </Row>
          </div>
        ))}
      </div>
    </div>
  );
}
