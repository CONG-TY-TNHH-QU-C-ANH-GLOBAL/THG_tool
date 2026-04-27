import { useState } from 'react';
import { theme, cardStyle, secondaryBtn } from '../constants/styles';
import { Avatar, Badge, Row } from './ui';
import { MOCK_ORGS_SUMMARY, MOCK_STAFF } from '../services/mockData';
import { Shield } from 'lucide-react';

interface SuperAdminProps { goBack: () => void; }

type AdminTab = 'orgs' | 'users' | 'system';

const SYSTEM_SERVICES = [
  { t: 'Database', v: 'SQLite WAL v3' },
  { t: 'AI Service', v: 'GPT-4o' },
  { t: 'FB Session Server', v: 'Docker · thg-browser:latest' },
  { t: 'Prometheus Metrics', v: ':8080/metrics' },
];

export default function SuperAdmin({ goBack }: SuperAdminProps) {
  const [tab, setTab] = useState<AdminTab>('orgs');

  const totals = [
    { l: 'Tổng tổ chức', v: MOCK_ORGS_SUMMARY.length, c: '#818cf8' },
    { l: 'Người dùng', v: 34, c: '#4ade80' },
    { l: 'Doanh thu/tháng', v: '₫18.5M', c: '#fbbf24' },
    { l: 'Sessions live', v: 12, c: '#38bdf8' },
  ];

  return (
    <div style={{ background: theme.bg, color: theme.text, fontFamily: 'system-ui, sans-serif', minHeight: '100vh' }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', padding: '13px 24px', background: theme.surfaceAlt, borderBottom: `1px solid ${theme.border}` }}>
        <Row style={{ gap: 10 }}>
          <div style={{ width: 30, height: 30, background: '#dc2626', borderRadius: 8, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <Shield size={14} color="#fff" />
          </div>
          <span style={{ fontWeight: 700, fontSize: 14, color: '#fff' }}>AutoFlow Admin Portal</span>
          <span style={{ background: '#dc262622', color: '#f87171', border: '1px solid #dc262644', fontSize: 10, padding: '2px 8px', borderRadius: 99 }}>Super Admin</span>
        </Row>
        <button onClick={goBack} style={{ ...secondaryBtn({ padding: '6px 14px', fontSize: 12 }), marginLeft: 'auto' }}>← Trang chủ</button>
      </div>

      <div style={{ padding: 22 }}>
        {/* Stats */}
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', gap: 12, marginBottom: 22 }}>
          {totals.map(s => (
            <div key={s.l} style={cardStyle()}>
              <p style={{ color: theme.textFaint, fontSize: 11 }}>{s.l}</p>
              <p style={{ fontSize: 24, fontWeight: 800, color: s.c, marginTop: 4 }}>{s.v}</p>
            </div>
          ))}
        </div>

        {/* Tab bar */}
        <Row style={{ gap: 8, marginBottom: 18 }}>
          {(['orgs', 'users', 'system'] as AdminTab[]).map(t => (
            <button key={t} onClick={() => setTab(t)} style={{
              padding: '7px 16px', borderRadius: 8, border: 'none', cursor: 'pointer', fontSize: 13,
              background: tab === t ? theme.primary : theme.surface,
              color: tab === t ? '#fff' : theme.textMuted,
            }}>
              {t === 'orgs' ? 'Tổ chức' : t === 'users' ? 'Người dùng' : 'Hệ thống'}
            </button>
          ))}
        </Row>

        {/* Orgs tab */}
        {tab === 'orgs' && (
          <div style={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 12, overflow: 'hidden' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
              <thead>
                <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                  {['Tổ chức', 'Gói', 'Users', 'Doanh thu', 'Ngày tạo', 'Status', ''].map(h => (
                    <th key={h} style={{ padding: '10px 14px', textAlign: 'left', color: theme.textFaint, fontWeight: 500, fontSize: 12 }}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {MOCK_ORGS_SUMMARY.map(o => (
                  <tr key={o.id} style={{ borderBottom: `1px solid ${theme.borderAlt}` }}>
                    <td style={{ padding: '10px 14px' }}>
                      <Row style={{ gap: 8 }}>
                        <Avatar text={o.name[0]} size={26} />
                        <span style={{ color: theme.text, fontWeight: 500 }}>{o.name}</span>
                      </Row>
                    </td>
                    <td style={{ padding: '10px 14px' }}><Badge label={o.plan} /></td>
                    <td style={{ padding: '10px 14px', color: '#d1d5db' }}>{o.users}</td>
                    <td style={{ padding: '10px 14px', color: '#fbbf24', fontWeight: 500 }}>{o.rev}</td>
                    <td style={{ padding: '10px 14px', color: theme.textFaint }}>{o.joined}</td>
                    <td style={{ padding: '10px 14px' }}><Badge label={o.status} /></td>
                    <td style={{ padding: '10px 14px' }}>
                      <button style={{ background: 'none', border: `1px solid ${theme.border}`, borderRadius: 6, color: theme.textMuted, fontSize: 11, padding: '4px 10px', cursor: 'pointer' }}>Quản lý</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {/* Users tab */}
        {tab === 'users' && (
          <div style={cardStyle()}>
            <p style={{ color: theme.textMuted, fontSize: 13, marginBottom: 14 }}>Toàn bộ user accounts trong hệ thống.</p>
            {MOCK_STAFF.map(s => (
              <div key={s.id} style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '10px 0', borderBottom: `1px solid ${theme.border}` }}>
                <Avatar text={s.name[0]} size={28} />
                <div style={{ flex: 1 }}>
                  <p style={{ color: theme.text, fontWeight: 500, fontSize: 13 }}>{s.name}</p>
                  <p style={{ color: theme.textFaint, fontSize: 12 }}>{s.email}</p>
                </div>
                <Badge label={s.status} />
              </div>
            ))}
          </div>
        )}

        {/* System tab */}
        {tab === 'system' && (
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 14 }}>
            {SYSTEM_SERVICES.map(s => (
              <div key={s.t} style={{ ...cardStyle(), display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <div>
                  <p style={{ color: theme.textMuted, fontSize: 12 }}>{s.t}</p>
                  <p style={{ color: theme.text, fontWeight: 500, fontSize: 14 }}>{s.v}</p>
                </div>
                <Badge label="Active" />
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
