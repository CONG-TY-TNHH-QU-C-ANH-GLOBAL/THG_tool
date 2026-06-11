import React, { useState } from 'react';
import { Trash2 } from 'lucide-react';
import { theme, cardStyle, alpha, tableHeaderCell, tableCell } from '../../constants/styles';
import { copy, type Lang } from './copy';
import { canRevokeBinding } from './logic';
import type { TelegramBinding } from '../../services/telegramIntegrationApi';

interface Props {
  lang: Lang; bindings: TelegramBinding[]; isAdmin: boolean; currentUserId: number;
  onRevoke: (id: number) => Promise<void>;
}

// SECONDARY section: optional personal DM bindings (personal notifications / commands). Visually
// quieter than the channels card. Shows last_command_at. Role/ownership-aware revoke.
export function PersonalBindingsTable({ lang, bindings, isAdmin, currentUserId, onRevoke }: Props) {
  const { t } = copy(lang);
  const [busyId, setBusyId] = useState<number | null>(null);
  const revoke = async (id: number) => {
    setBusyId(id);
    try { await onRevoke(id); } finally { setBusyId(null); }
  };

  return (
    <div style={cardStyle({ padding: 0, overflow: 'hidden', opacity: 0.96 })}>
      <div style={{ padding: '14px 16px 8px' }}>
        <p style={{ color: theme.textMuted, fontSize: 13, fontWeight: 600, margin: 0 }}>{t('personal_title')}</p>
        <p style={{ color: theme.textFaint, fontSize: 12, margin: '3px 0 0' }}>{t('personal_desc')}</p>
      </div>
      {bindings.length === 0 ? (
        <p style={{ color: theme.textMuted, fontSize: 12.5, padding: '0 16px 16px' }}>{t('personal_empty')}</p>
      ) : (
        <div style={{ overflowX: 'auto' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ borderTop: `1px solid ${theme.border}` }}>
                <th style={tableHeaderCell}>{t('col_user')}</th>
                <th style={tableHeaderCell}>{t('col_role')}</th>
                <th style={tableHeaderCell}>{t('col_last_command')}</th>
                <th style={tableHeaderCell}>{t('col_status')}</th>
                <th style={tableHeaderCell} />
              </tr>
            </thead>
            <tbody>
              {bindings.map((b) => {
                const name = b.display_name || (b.telegram_username ? '@' + b.telegram_username : '#' + b.telegram_user_id);
                const isActive = b.status === 'active';
                const sc = isActive ? theme.green : theme.textFaint;
                const lastCmd = b.last_command_at ? new Date(b.last_command_at).toLocaleString(lang === 'en' ? 'en-US' : 'vi-VN') : '—';
                return (
                  <tr key={b.id} style={{ borderTop: `1px solid ${theme.border}` }}>
                    <td style={{ ...tableCell, color: theme.text, fontSize: 13 }}>{name}</td>
                    <td style={{ ...tableCell, color: theme.textMuted, fontSize: 12 }}>{b.role || '—'}</td>
                    <td style={{ ...tableCell, color: theme.textMuted, fontSize: 12 }}>{lastCmd}</td>
                    <td style={tableCell}>
                      <span style={{ fontSize: 11, color: sc, border: `1px solid ${alpha(sc, 30)}`, background: alpha(sc, 8), borderRadius: 99, padding: '2px 8px' }}>{t(b.status)}</span>
                    </td>
                    <td style={{ ...tableCell, textAlign: 'right' }}>
                      {isActive && canRevokeBinding(isAdmin, b.user_id, currentUserId) && (
                        <button aria-label={t('revoke')} title={t('revoke')} disabled={busyId === b.id} onClick={() => revoke(b.id)}
                          style={{ background: 'transparent', border: 'none', cursor: 'pointer', color: theme.red, opacity: busyId === b.id ? 0.5 : 1, padding: 4 }}>
                          <Trash2 size={15} />
                        </button>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
