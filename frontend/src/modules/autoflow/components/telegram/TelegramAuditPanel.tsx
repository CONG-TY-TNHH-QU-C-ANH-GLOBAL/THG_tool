import React from 'react';
import { theme, cardStyle, tableHeaderCell, tableCell } from '../../constants/styles';
import { strings, type Lang } from './telegramCopy';
import type { TelegramAuditEvent } from '../../services/telegramIntegrationApi';

// Admin-only audit trail of control-plane events. Render-only. Non-admins never receive the data
// (backend 403s) and the panel is not rendered for them by the page.
export function TelegramAuditPanel({ lang, events, isAdmin }: { lang: Lang; events: TelegramAuditEvent[]; isAdmin: boolean }) {
  if (!isAdmin) return null;
  const { t } = strings(lang);
  return (
    <div style={cardStyle({ padding: 0, overflow: 'hidden' })}>
      <p style={{ color: theme.text, fontSize: 14, fontWeight: 650, margin: 0, padding: '16px 16px 10px' }}>{t('audit_title')}</p>
      {events.length === 0 ? (
        <p style={{ color: theme.textMuted, fontSize: 13, padding: '0 16px 18px' }}>{t('audit_empty')}</p>
      ) : (
        <div style={{ overflowX: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ borderTop: `1px solid ${theme.border}` }}>
                <th style={tableHeaderCell}>{t('col_time')}</th>
                <th style={tableHeaderCell}>{t('col_actor')}</th>
                <th style={tableHeaderCell}>{t('col_action')}</th>
                <th style={tableHeaderCell}>{t('col_result')}</th>
              </tr>
            </thead>
            <tbody>
              {events.map((e) => (
                <tr key={e.id} style={{ borderTop: `1px solid ${theme.border}` }}>
                  <td style={{ ...tableCell, color: theme.textMuted, fontSize: 12 }}>{new Date(e.created_at).toLocaleString(lang === 'en' ? 'en-US' : 'vi-VN')}</td>
                  <td style={{ ...tableCell, color: theme.textMuted, fontSize: 12 }}>{e.user_id ? '#' + e.user_id : '—'}</td>
                  <td style={{ ...tableCell, color: theme.text, fontSize: 12.5, fontFamily: 'var(--font-mono)' }}>{e.action}</td>
                  <td style={{ ...tableCell, color: theme.textMuted, fontSize: 12 }}>{e.result || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
