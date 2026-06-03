import { useState } from 'react';
import { Avatar, Badge, Row } from '../ui';
import { alpha, theme, cardStyle } from '../../constants/styles';
import { useContributionLeaderboard } from '../../hooks/useContributionLeaderboard';
import { Crown, GitBranch, Info } from 'lucide-react';

const MEDAL = ['🥇', '🥈', '🥉'];

const WINDOWS: { label: string; days: number }[] = [
  { label: '7 ngày', days: 7 },
  { label: '30 ngày', days: 30 },
  { label: 'Tất cả', days: 0 },
];

// Human labels for the interaction types stored in action_ledger.action_type.
const TYPE_LABEL: Record<string, string> = {
  comment: 'Comment',
  inbox: 'Nhắn tin',
  group_post: 'Đăng nhóm',
  profile_post: 'Đăng tường',
  reply: 'Trả lời',
  follow_up: 'Theo dõi',
};

function typeLabel(t: string): string {
  return TYPE_LABEL[t] ?? t;
}

/**
 * Contribution leaderboard — derived from the append-only action_ledger via the
 * immutable created_by (PR5). Separate lens from the KPI leaderboard: it counts
 * verified executed actions per member, not weighted points. Champion is
 * analytics-only — no routing/ownership/execution privilege.
 */
export default function ContributionLeaderboardView() {
  const [days, setDays] = useState(30);
  const { data, loading, error } = useContributionLeaderboard(days);

  const rows = data?.rows ?? [];
  const top3 = rows.slice(0, 3);
  const rest = rows.slice(3);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      <div style={cardStyle()}>
        <Row style={{ gap: 10, marginBottom: 8 }}>
          <GitBranch size={16} color={theme.primaryLight} />
          <p style={{ color: theme.text, fontWeight: 600, fontSize: 13, flex: 1 }}>Đóng góp thực thi (Organic Network)</p>
          <Row style={{ gap: 6 }}>
            {WINDOWS.map(w => (
              <button
                key={w.days}
                onClick={() => setDays(w.days)}
                style={{
                  padding: '5px 11px',
                  borderRadius: 8,
                  border: 'none',
                  cursor: 'pointer',
                  fontSize: 11,
                  background: days === w.days ? theme.primary : theme.surfaceAlt,
                  color: days === w.days ? 'var(--accent-ink)' : theme.textMuted,
                }}
              >
                {w.label}
              </button>
            ))}
          </Row>
        </Row>
        <Row style={{ gap: 8, alignItems: 'flex-start' }}>
          <Info size={13} color={theme.textFaint} style={{ marginTop: 1, flexShrink: 0 }} />
          <p style={{ color: theme.textFaint, fontSize: 11, lineHeight: 1.5 }}>
            Thống kê từ lịch sử hành động đã xác minh, gắn theo người thực hiện gốc (bất biến — đổi chủ tài khoản
            không làm sai lịch sử). Chỉ để phân tích đóng góp; <strong>không</strong> ảnh hưởng quyền, định tuyến,
            hay ưu tiên thực thi.
          </p>
        </Row>
      </div>

      {loading && (
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: 120 }}>
          <div className="skeleton" style={{ width: 220, height: 14 }} />
        </div>
      )}

      {error && !loading && (
        <div style={{ ...cardStyle(), color: theme.red, fontSize: 12 }}>{error}</div>
      )}

      {!loading && !error && rows.length === 0 && (
        <div style={{ ...cardStyle(), textAlign: 'center', color: theme.textFaint, fontSize: 12, padding: '28px 14px' }}>
          Chưa có đóng góp nào được ghi nhận trong khoảng thời gian này.
        </div>
      )}

      {!loading && top3.length > 0 && (
        <div style={{ display: 'grid', gridTemplateColumns: `repeat(${Math.min(top3.length, 3)},1fr)`, gap: 12 }}>
          {top3.map((s, i) => (
            <div key={s.userId} style={{ ...cardStyle(), textAlign: 'center', position: 'relative', border: i === 0 ? `1px solid ${alpha(theme.primary, 35)}` : `1px solid ${theme.border}` }}>
              {i === 0 && (
                <div style={{ position: 'absolute', top: -10, left: '50%', transform: 'translateX(-50%)', background: theme.primary, color: 'var(--accent-ink)', fontSize: 10, fontWeight: 700, padding: '2px 10px', borderRadius: 99, whiteSpace: 'nowrap', display: 'flex', alignItems: 'center', gap: 4 }}>
                  <Crown size={11} /> Champion
                </div>
              )}
              <div style={{ fontSize: 28, marginBottom: 8 }}>{MEDAL[i]}</div>
              <Avatar text={(s.userName || '?')[0]} size={40} />
              <p style={{ color: theme.text, fontWeight: 600, fontSize: 13, marginTop: 8 }}>{s.userName}</p>
              <p style={{ fontSize: 22, fontWeight: 800, color: i === 0 ? theme.primaryLight : theme.text, marginTop: 4 }}>
                {s.total} <span style={{ fontSize: 12, fontWeight: 400, color: theme.textMuted }}>hành động</span>
              </p>
              <Row style={{ gap: 5, justifyContent: 'center', flexWrap: 'wrap', marginTop: 10 }}>
                {Object.entries(s.byType).map(([t, n]) => (
                  <span key={t} style={{ background: theme.surfaceAlt, borderRadius: 7, padding: '3px 8px', fontSize: 10, color: theme.textMuted }}>
                    {typeLabel(t)} <strong style={{ color: theme.text }}>{n}</strong>
                  </span>
                ))}
              </Row>
            </div>
          ))}
        </div>
      )}

      {!loading && rest.length > 0 && (
        <div style={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 12, overflow: 'hidden' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
            <thead>
              <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                {['#', 'Thành viên', 'Tổng', 'Phân loại'].map(h => (
                  <th key={h} style={{ padding: '9px 14px', textAlign: 'left', color: theme.textFaint, fontWeight: 500, fontSize: 11 }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rest.map((s, i) => (
                <tr key={s.userId} style={{ borderBottom: `1px solid ${theme.borderAlt}` }}>
                  <td style={{ padding: '9px 14px', color: theme.textFaint, fontWeight: 600 }}>{i + 4}</td>
                  <td style={{ padding: '9px 14px' }}>
                    <Row style={{ gap: 8 }}>
                      <Avatar text={(s.userName || '?')[0]} size={26} />
                      <span style={{ color: theme.text, fontWeight: 500 }}>{s.userName}</span>
                    </Row>
                  </td>
                  <td style={{ padding: '9px 14px' }}>
                    <span style={{ color: theme.primaryLight, fontWeight: 700 }}>{s.total}</span>
                  </td>
                  <td style={{ padding: '9px 14px' }}>
                    <Row style={{ gap: 5, flexWrap: 'wrap' }}>
                      {Object.entries(s.byType).map(([t, n]) => (
                        <span key={t} style={{ background: theme.surfaceAlt, borderRadius: 6, padding: '2px 7px', fontSize: 10, color: theme.textMuted }}>
                          {typeLabel(t)} <strong style={{ color: theme.text }}>{n}</strong>
                        </span>
                      ))}
                    </Row>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {!loading && data && data.champion && (
        <Row style={{ gap: 6, justifyContent: 'center' }}>
          <Badge label={`Champion: ${data.champion}`} />
        </Row>
      )}
    </div>
  );
}
