import { useState } from 'react';
import type { KPIConfig } from '../../types';
import { Avatar, Badge, Row } from '../ui';
import { alpha, theme, cardStyle, inputStyle, primaryBtn } from '../../constants/styles';
import { useLeaderboard } from '../../hooks/useLeaderboard';
import ContributionLeaderboardView from './ContributionLeaderboardView';
import { Trophy, Save, Award, GitBranch } from 'lucide-react';

interface LeaderboardViewProps { orgId: string; isAdmin: boolean; }

const MEDAL = ['🥇', '🥈', '🥉'];

type LbMode = 'kpi' | 'contrib';

const MODES: { id: LbMode; label: string; Icon: typeof Award }[] = [
  { id: 'kpi', label: 'KPI điểm', Icon: Award },
  { id: 'contrib', label: 'Đóng góp thực thi', Icon: GitBranch },
];

export default function LeaderboardView({ orgId, isAdmin }: Readonly<LeaderboardViewProps>) {
  const { scored, config, updateConfig, isSaving } = useLeaderboard(orgId);
  const [draft, setDraft] = useState<KPIConfig>(config);
  const [editMode, setEditMode] = useState(false);
  const [mode, setMode] = useState<LbMode>('kpi');

  const handleSave = async () => {
    await updateConfig(draft);
    setEditMode(false);
  };

  const top3 = scored.slice(0, 3);
  const rest = scored.slice(3);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      <Row style={{ gap: 6 }}>
        {MODES.map(({ id, label, Icon }) => (
          <button
            key={id}
            onClick={() => setMode(id)}
            style={{
              display: 'flex', alignItems: 'center', gap: 6,
              padding: '7px 13px', borderRadius: 9, border: 'none', cursor: 'pointer', fontSize: 12,
              background: mode === id ? theme.primary : theme.surface,
              color: mode === id ? 'var(--accent-ink)' : theme.textMuted,
            }}
          >
            <Icon size={12} />{label}
          </button>
        ))}
      </Row>

      {mode === 'contrib' && <ContributionLeaderboardView />}

      {mode === 'kpi' && top3.length > 0 && (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3,1fr)', gap: 12 }}>
          {top3.map((s, i) => (
            <div key={s.id} style={{ ...cardStyle(), textAlign: 'center', position: 'relative', border: i === 0 ? `1px solid ${alpha(theme.primary, 35)}` : `1px solid ${theme.border}` }}>
              {i === 0 && (
                <div style={{ position: 'absolute', top: -10, left: '50%', transform: 'translateX(-50%)', background: theme.primary, color: 'var(--accent-ink)', fontSize: 10, fontWeight: 700, padding: '2px 10px', borderRadius: 99, whiteSpace: 'nowrap' }}>
                  Top Sales
                </div>
              )}
              <div style={{ fontSize: 28, marginBottom: 8 }}>{MEDAL[i]}</div>
              <Avatar text={s.name[0]} size={40} />
              <p style={{ color: theme.text, fontWeight: 600, fontSize: 13, marginTop: 8 }}>{s.name}</p>
              <p style={{ color: theme.textFaint, fontSize: 11, marginBottom: 8 }}>{s.role}</p>
              <p style={{ fontSize: 22, fontWeight: 800, color: i === 0 ? theme.primaryLight : theme.text }}>{s.pts} <span style={{ fontSize: 12, fontWeight: 400, color: theme.textMuted }}>pts</span></p>
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3,1fr)', gap: 6, marginTop: 10 }}>
                {[{ l: 'Hội thoại', v: s.convs }, { l: 'Chốt', v: s.converted }, { l: 'Comments', v: s.cmts }].map(st => (
                  <div key={st.l} style={{ background: theme.surfaceAlt, borderRadius: 7, padding: '6px 4px' }}>
                    <p style={{ color: theme.text, fontWeight: 700, fontSize: 13 }}>{st.v}</p>
                    <p style={{ color: theme.textFaint, fontSize: 9 }}>{st.l}</p>
                  </div>
                ))}
              </div>
              <Badge label={s.status} />
            </div>
          ))}
        </div>
      )}

      {mode === 'kpi' && rest.length > 0 && (
        <div style={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 12, overflow: 'hidden' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
            <thead>
              <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                {['#', 'Nhân viên', 'Hội thoại', 'Chốt deal', 'Comments', 'Điểm', 'Status'].map(h => (
                  <th key={h} style={{ padding: '9px 14px', textAlign: 'left', color: theme.textFaint, fontWeight: 500, fontSize: 11 }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rest.map((s, i) => (
                <tr key={s.id} style={{ borderBottom: `1px solid ${theme.borderAlt}` }}>
                  <td style={{ padding: '9px 14px', color: theme.textFaint, fontWeight: 600 }}>{i + 4}</td>
                  <td style={{ padding: '9px 14px' }}>
                    <Row style={{ gap: 8 }}>
                      <Avatar text={s.name[0]} size={26} />
                      <div>
                        <p style={{ color: theme.text, fontWeight: 500 }}>{s.name}</p>
                        <p style={{ color: theme.textFaint, fontSize: 10 }}>{s.role}</p>
                      </div>
                    </Row>
                  </td>
                  <td style={{ padding: '9px 14px', color: theme.textMuted }}>{s.convs}</td>
                  <td style={{ padding: '9px 14px', color: theme.textMuted }}>{s.converted}</td>
                  <td style={{ padding: '9px 14px', color: theme.textMuted }}>{s.cmts}</td>
                  <td style={{ padding: '9px 14px' }}>
                    <span style={{ color: theme.primaryLight, fontWeight: 700 }}>{s.pts}</span>
                    <span style={{ color: theme.textFaint, fontSize: 10 }}> pts</span>
                  </td>
                  <td style={{ padding: '9px 14px' }}><Badge label={s.status} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {mode === 'kpi' && isAdmin && (
        <div style={cardStyle()}>
          <Row style={{ gap: 10, marginBottom: 16 }}>
            <Trophy size={16} color={theme.primaryLight} />
            <p style={{ color: theme.text, fontWeight: 600, fontSize: 13, flex: 1 }}>Cấu hình KPI</p>
            {!editMode ? (
              <button onClick={() => { setDraft(config); setEditMode(true); }} style={primaryBtn({ padding: '6px 14px', fontSize: 12 })}>Chỉnh sửa</button>
            ) : (
              <Row style={{ gap: 8 }}>
                <button onClick={() => setEditMode(false)} style={{ background: 'none', border: `1px solid ${theme.border}`, borderRadius: 7, color: theme.textMuted, fontSize: 12, padding: '5px 12px', cursor: 'pointer' }}>Hủy</button>
                <button onClick={handleSave} disabled={isSaving} style={{ ...primaryBtn({ padding: '6px 14px', fontSize: 12 }), display: 'flex', alignItems: 'center', gap: 5, opacity: isSaving ? 0.6 : 1 }}>
                  <Save size={12} />{isSaving ? 'Đang lưu...' : 'Lưu'}
                </button>
              </Row>
            )}
          </Row>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3,1fr)', gap: 12 }}>
            {[
              { key: 'conv' as const, l: 'Điểm/hội thoại', hint: 'pts' },
              { key: 'conv2' as const, l: 'Điểm/chốt deal', hint: 'pts' },
              { key: 'cmt' as const, l: 'Điểm/comment', hint: 'pts' },
              { key: 'bonus' as const, l: 'Ngưỡng thưởng', hint: 'pts' },
              { key: 'bonusAmt' as const, l: 'Mức thưởng', hint: '₫' },
              { key: 'penAmt' as const, l: 'Mức phạt', hint: '₫' },
            ].map(({ key, l, hint }) => (
              <div key={key}>
                <p style={{ color: theme.textFaint, fontSize: 11, marginBottom: 5 }}>{l}</p>
                <Row style={{ gap: 6 }}>
                  <input
                    type="number"
                    value={editMode ? draft[key] : config[key]}
                    onChange={e => editMode && setDraft(d => ({ ...d, [key]: Number(e.target.value) }))}
                    disabled={!editMode}
                    style={{ ...inputStyle, flex: 1, padding: '7px 10px', fontSize: 12, opacity: editMode ? 1 : 0.7 }}
                  />
                  <span style={{ color: theme.textFaint, fontSize: 11 }}>{hint}</span>
                </Row>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
