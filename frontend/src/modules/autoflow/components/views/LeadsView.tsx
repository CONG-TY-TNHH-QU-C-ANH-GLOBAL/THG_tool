import { useState } from 'react';
import type { LeadStatus } from '../../types';
import { Avatar, Badge, Row } from '../ui';
import { cardStyle, theme } from '../../constants/styles';
import { useLeads } from '../../hooks/useLeads';

interface LeadsViewProps { orgId: string; isAdmin: boolean; }

const FILTERS: Array<LeadStatus | 'All'> = ['All', 'Hot', 'Warm', 'Cold'];

export default function LeadsView({ orgId, isAdmin }: LeadsViewProps) {
  const [filter, setFilter] = useState<LeadStatus | 'All'>('All');
  const { leads, isLoading } = useLeads(orgId, filter);
  void isAdmin;

  const stats = [
    { label: 'Total Leads', value: leads.length, color: '#fff' },
    { label: 'Hot Leads', value: leads.filter(l => l.status === 'Hot').length, color: '#f87171' },
    { label: 'Warm Leads', value: leads.filter(l => l.status === 'Warm').length, color: '#fbbf24' },
    { label: 'Avg Score', value: leads.length ? Math.round(leads.reduce((a, l) => a + l.score, 0) / leads.length) : 0, color: '#818cf8' },
  ];

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
      <Row style={{ gap: 8 }}>
        {FILTERS.map(x => (
          <button key={x} onClick={() => setFilter(x)} style={{
            padding: '5px 12px',
            borderRadius: 7,
            border: 'none',
            cursor: 'pointer',
            fontSize: 12,
            background: filter === x ? theme.primary : theme.surface,
            color: filter === x ? '#fff' : theme.textMuted,
          }}>{x}</button>
        ))}
      </Row>
      {isLoading ? (
        <p style={{ color: theme.textMuted, fontSize: 13, padding: 20, textAlign: 'center' }}>Đang tải...</p>
      ) : (
        <div style={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 12, overflow: 'hidden' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
            <thead>
              <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                {['Khách hàng', 'Facebook', 'Status', 'Nhóm', 'Classifier', 'Score', 'Liên hệ cuối'].map(h => (
                  <th key={h} style={{ padding: '9px 14px', textAlign: 'left', color: theme.textFaint, fontWeight: 500, fontSize: 11 }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {leads.length === 0 ? (
                <tr><td colSpan={7} style={{ padding: 22, color: theme.textMuted, textAlign: 'center' }}>Chưa có lead thật từ crawler.</td></tr>
              ) : leads.map(l => (
                <tr key={l.id} style={{ borderBottom: `1px solid ${theme.borderAlt}` }}>
                  <td style={{ padding: '9px 14px' }}>
                    <Row style={{ gap: 8 }}>
                      <Avatar text={(l.name.split(' ').pop()?.[0] || 'L')} size={26} />
                      <div>
                        <p style={{ color: theme.text, fontWeight: 500 }}>{l.name}</p>
                        <p style={{ color: theme.textFaint, fontSize: 10 }}>{l.phone}</p>
                      </div>
                    </Row>
                  </td>
                  <td style={{ padding: '9px 14px', color: theme.primaryLight, fontSize: 11 }}>
                    {l.facebookUrl ? <a href={l.facebookUrl} target="_blank" rel="noopener noreferrer" style={{ color: theme.primaryLight }}>Mở link</a> : '-'}
                  </td>
                  <td style={{ padding: '9px 14px' }}><Badge label={l.status} /></td>
                  <td style={{ padding: '9px 14px', color: '#d1d5db' }}>{l.group}</td>
                  <td style={{ padding: '9px 14px' }}>
                    <span style={{ background: theme.border, color: '#d1d5db', padding: '2px 7px', borderRadius: 5, fontSize: 10 }}>{l.agent}</span>
                  </td>
                  <td style={{ padding: '9px 14px' }}>
                    <Row style={{ gap: 7 }}>
                      <div style={{ width: 40, height: 4, background: '#374151', borderRadius: 99, overflow: 'hidden' }}>
                        <div style={{ width: `${l.score}%`, height: '100%', background: '#6366f1' }} />
                      </div>
                      <span style={{ color: '#fff', fontWeight: 600 }}>{l.score}</span>
                    </Row>
                  </td>
                  <td style={{ padding: '9px 14px', color: theme.textFaint }}>{l.last}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
