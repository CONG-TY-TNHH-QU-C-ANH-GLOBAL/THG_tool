import { useState, useEffect } from 'react';
import type { Comment } from '../../types';
import { Avatar, Badge, Row } from '../ui';
import { theme, cardStyle, primaryBtn, secondaryBtn } from '../../constants/styles';
import { MOCK_COMMENTS } from '../../services/mockData';
import { MessageCircle, Zap, Check, X } from 'lucide-react';

interface CommentingViewProps { orgId: string; }

export default function CommentingView({ orgId }: CommentingViewProps) {
  const [comments, setComments] = useState<Comment[]>([]);
  const [filter, setFilter] = useState<'all' | 'pending' | 'sent'>('all');
  void orgId;

  useEffect(() => { setComments([...MOCK_COMMENTS]); }, []);

  const stats = [
    { l: 'Đã comment', v: comments.length, c: '#fff' },
    { l: 'Hôm nay', v: 3, c: '#4ade80' },
    { l: 'Đang chờ', v: 1, c: '#fbbf24' },
    { l: 'Tỉ lệ phản hồi', v: '67%', c: '#818cf8' },
  ];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', gap: 11 }}>
        {stats.map(s => (
          <div key={s.l} style={cardStyle()}>
            <p style={{ color: theme.textFaint, fontSize: 11, marginBottom: 4 }}>{s.l}</p>
            <p style={{ fontSize: 22, fontWeight: 700, color: s.c }}>{s.v}</p>
          </div>
        ))}
      </div>

      <Row style={{ gap: 8 }}>
        {(['all', 'pending', 'sent'] as const).map(f => (
          <button key={f} onClick={() => setFilter(f)} style={{
            padding: '5px 12px', borderRadius: 7, border: 'none', cursor: 'pointer', fontSize: 12,
            background: filter === f ? theme.primary : theme.surface,
            color: filter === f ? '#fff' : theme.textMuted,
          }}>
            {f === 'all' ? 'Tất cả' : f === 'pending' ? 'Đang chờ' : 'Đã gửi'}
          </button>
        ))}
        <button style={{ ...primaryBtn({ padding: '6px 13px', fontSize: 12 }), marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 5 }}>
          <Zap size={13} />Auto-comment tất cả
        </button>
      </Row>

      <div style={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 12, overflow: 'hidden' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
          <thead>
            <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
              {['Agent', 'Khách hàng', 'Bài viết', 'Nội dung comment', 'Thời gian', ''].map(h => (
                <th key={h} style={{ padding: '9px 14px', textAlign: 'left', color: theme.textFaint, fontWeight: 500, fontSize: 11 }}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {comments.map(c => (
              <tr key={c.id} style={{ borderBottom: `1px solid ${theme.borderAlt}` }}>
                <td style={{ padding: '9px 14px' }}>
                  <Row style={{ gap: 6 }}>
                    <Avatar text={c.agent[c.agent.length - 1]} size={24} />
                    <span style={{ color: theme.text, fontSize: 12 }}>{c.agent}</span>
                  </Row>
                </td>
                <td style={{ padding: '9px 14px', color: '#d1d5db' }}>{c.lead}</td>
                <td style={{ padding: '9px 14px' }}>
                  <span style={{ background: theme.border, color: '#d1d5db', padding: '2px 8px', borderRadius: 5, fontSize: 10 }}>{c.post}</span>
                </td>
                <td style={{ padding: '9px 14px', color: theme.textMuted, maxWidth: 260 }}>
                  <p style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.comment}</p>
                </td>
                <td style={{ padding: '9px 14px', color: theme.textFaint }}>{c.time}</td>
                <td style={{ padding: '9px 14px' }}>
                  <Badge label="Sent" />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

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
              <button style={secondaryBtn({ padding: '7px 12px', fontSize: 11 })}>
                <Check size={12} />
              </button>
              <button style={{ ...secondaryBtn({ padding: '7px 12px', fontSize: 11 }), color: theme.red }}>
                <X size={12} />
              </button>
            </Row>
          </div>
        ))}
      </div>
    </div>
  );
}
