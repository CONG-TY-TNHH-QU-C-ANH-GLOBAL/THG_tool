import { useState, useEffect } from 'react';
import type { Post } from '../../types';
import { Badge, Row } from '../ui';
import { theme, cardStyle, primaryBtn, secondaryBtn } from '../../constants/styles';
import { MOCK_POSTS } from '../../services/mockData';
import { ThumbsUp, MessageCircle, Share2, Eye, Plus } from 'lucide-react';

interface PostingViewProps { orgId: string; }

export default function PostingView({ orgId }: PostingViewProps) {
  const [posts, setPosts] = useState<Post[]>([]);
  void orgId;

  useEffect(() => { setPosts([...MOCK_POSTS]); }, []);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <Row style={{ gap: 8 }}>
        {['Tất cả', 'Live', 'Đã kết thúc'].map(f => (
          <button key={f} style={secondaryBtn({ padding: '6px 13px', fontSize: 12 })}>{f}</button>
        ))}
        <button style={{ ...primaryBtn({ padding: '6px 13px', fontSize: 12 }), marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 5 }}>
          <Plus size={13} />Tạo bài viết
        </button>
      </Row>
      {posts.map(p => (
        <div key={p.id} style={cardStyle()}>
          <Row style={{ gap: 8, marginBottom: 8 }}>
            <span style={{ background: theme.border, color: '#d1d5db', padding: '2px 8px', borderRadius: 5, fontSize: 10 }}>{p.group}</span>
            <span style={{ color: theme.textFaint, fontSize: 11 }}>{p.time}</span>
            <Badge label={p.status} />
          </Row>
          <p style={{ color: '#d1d5db', fontSize: 13, lineHeight: 1.6 }}>{p.content}</p>
          <Row style={{ gap: 20, marginTop: 12, paddingTop: 12, borderTop: `1px solid ${theme.border}` }}>
            {[
              { Icon: ThumbsUp, v: p.likes, l: 'Likes' },
              { Icon: MessageCircle, v: p.comments, l: 'Comments' },
              { Icon: Share2, v: p.shares, l: 'Shares' },
            ].map(s => (
              <Row key={s.l} style={{ gap: 5, color: theme.textMuted }}>
                <s.Icon size={12} />
                <span style={{ color: theme.text, fontWeight: 500, fontSize: 12 }}>{s.v}</span>
                <span style={{ fontSize: 11 }}>{s.l}</span>
              </Row>
            ))}
            <button style={{ marginLeft: 'auto', background: 'none', border: 'none', color: theme.primaryLight, fontSize: 11, cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 4 }}>
              <Eye size={11} />Xem bài
            </button>
          </Row>
        </div>
      ))}
    </div>
  );
}
