import { useState } from 'react';
import type { LeadStatus } from '../../types';
import { Avatar, Badge, Row } from '../ui';
import { cardStyle, primaryBtn, theme } from '../../constants/styles';
import { useLeads } from '../../hooks/useLeads';
import { Plus } from 'lucide-react';

interface LeadsViewProps { orgId: string; isAdmin: boolean; }

const FILTERS: Array<LeadStatus | 'All'> = ['All', 'Hot', 'Warm', 'Cold'];

export default function LeadsView({ orgId, isAdmin }: LeadsViewProps) {
  const [filter, setFilter] = useState<LeadStatus | 'All'>('All');
  const { leads, isLoading } = useLeads(orgId, filter);

  const stats = [
    { l: 'Total Leads', v: leads.length, c: '#fff' },
    { l: 'Hot Leads', v: leads.filter(l => l.status === 'Hot').length, c: '#f87171' },
    { l: 'Warm Leads', v: leads.filter(l => l.status === 'Warm').length, c: '#fbbf24' },
    { l: 'Avg Score', v: leads.length ? Math.round(leads.reduce((a, l) => a + l.score, 0) / leads.length) : 0, c: '#818cf8' },
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
        {FILTERS.map(x => (
          <button key={x} onClick={() => setFilter(x)} style={{
            padding: '5px 12px', borderRadius: 7, border: 'none', cursor: 'pointer', fontSize: 12,
            background: filter === x ? theme.primary : theme.surface,
            color: filter === x ? '#fff' : theme.textMuted,
          }}>{x}</button>
        ))}
        {isAdmin && (
          <button style={{ ...primaryBtn({ padding: '6px 13px', fontSize: 12 }), marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 5 }}>
            <Plus size={13} />Thêm lead
          </button>
        )}
      </Row>
      {isLoading ? (
        <p style={{ color: theme.textMuted, fontSize: 13, padding: 20, textAlign: 'center' }}>Đang tải...</p>
      ) : (
        <div style={{ background: theme.surface, border: `1px solid ${theme.border}`, borderRadius: 12, overflow: 'hidden' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
            <thead>
              <tr style={{ borderBottom: `1px solid ${theme.border}` }}>
                {['Khách hàng', 'Facebook', 'Status', 'Nhóm', 'Agent', 'Score', 'Liên hệ cuối'].map(h => (
                  <th key={h} style={{ padding: '9px 14px', textAlign: 'left', color: theme.textFaint, fontWeight: 500, fontSize: 11 }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {leads.map(l => (
                <tr key={l.id} style={{ borderBottom: `1px solid ${theme.borderAlt}` }}>
                  <td style={{ padding: '9px 14px' }}>
                    <Row style={{ gap: 8 }}>
                      <Avatar text={l.name.split(' ').pop()![0]} size={26} />
                      <div>
                        <p style={{ color: theme.text, fontWeight: 500 }}>{l.name}</p>
                        <p style={{ color: theme.textFaint, fontSize: 10 }}>{l.phone}</p>
                      </div>
                    </Row>
                  </td>
                  <td style={{ padding: '9px 14px', color: theme.primaryLight, fontSize: 11 }}>fb.com/...</td>
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
