import React, { useState } from 'react';
import { Settings2, Trash2 } from 'lucide-react';
import { theme, cardStyle, alpha } from '../../constants/styles';
import { strings, destTypeLabel, type Lang } from './telegramCopy';
import { destinationTone, type DestStatus } from './logic';
import { DestinationPreferencesPanel } from './DestinationPreferencesPanel';
import { TestNotificationButton } from './TestNotificationButton';
import type { TelegramDestination } from '../../services/telegramIntegrationApi';

const TONE: Record<string, string> = { ok: theme.green, warn: theme.yellow, off: theme.textFaint };

interface Props {
  lang: Lang; destinations: TelegramDestination[]; isAdmin: boolean;
  availableEventTypes: string[]; availableFilters: string[];
  onSave: (id: number, body: { event_types: string[]; channel_filter: string }) => Promise<void>;
  onDisconnect: (id: number) => Promise<void>;
  onReload: () => void;
}

// Primary card: connected channel destinations. Shows title (+ @username for public, title only for
// private), type, status, filter, subscribed-event count, last delivery + error, and admin actions.
// NEVER renders chat_id.
export function TelegramChannelDestinationsTable({ lang, destinations, isAdmin, availableEventTypes, availableFilters, onSave, onDisconnect, onReload }: Readonly<Props>) {
  const { t } = strings(lang);
  const [editId, setEditId] = useState<number | null>(null);

  return (
    <div style={cardStyle({ display: 'grid', gap: 12 })}>
      <p style={{ color: theme.text, fontSize: 14, fontWeight: 650, margin: 0 }}>{t('channels_title')}</p>
      {destinations.length === 0 ? (
        <p style={{ color: theme.textMuted, fontSize: 13, margin: 0 }}>{t('channels_empty')}</p>
      ) : (
        <div style={{ display: 'grid', gap: 10 }}>
          {destinations.map((d) => {
            const tc = TONE[destinationTone(d.status as DestStatus)];
            const name = d.username ? `${d.title} (@${d.username})` : d.title || `#${d.id}`;
            const delivered = d.last_delivery_at ? new Date(d.last_delivery_at).toLocaleString(lang === 'en' ? 'en-US' : 'vi-VN') : t('never');
            return (
              <div key={d.id} style={{ border: `1px solid ${theme.border}`, borderRadius: 'var(--radius-md)', padding: 12, display: 'grid', gap: 8 }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10, flexWrap: 'wrap' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 9, minWidth: 0 }}>
                    <span style={{ width: 8, height: 8, borderRadius: 99, background: tc, flexShrink: 0 }} />
                    <span style={{ color: theme.text, fontSize: 13.5, fontWeight: 600 }}>{name}</span>
                    <span style={{ fontSize: 11, color: theme.textFaint, border: `1px solid ${theme.border}`, borderRadius: 99, padding: '1px 8px' }}>{destTypeLabel(lang, d.destination_type)}</span>
                  </div>
                  <span style={{ fontSize: 11, color: tc, border: `1px solid ${alpha(tc, 34)}`, background: alpha(tc, 10), borderRadius: 99, padding: '2px 8px' }}>{t('state_' + d.status)}</span>
                </div>
                <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap', fontSize: 12, color: theme.textMuted }}>
                  <span>{t('col_filter')}: {d.channel_filter}</span>
                  <span>{t('col_events')}: {d.event_types.length}</span>
                  <span>{t('col_last_delivery')}: {delivered}</span>
                </div>
                {d.last_error && <span style={{ fontSize: 12, color: theme.red }}>{t('col_last_error')}: {d.last_error}</span>}
                {isAdmin && (
                  <div style={{ display: 'flex', gap: 10, alignItems: 'center', flexWrap: 'wrap' }}>
                    <TestNotificationButton lang={lang} destinationId={d.id} onResult={onReload} />
                    <button onClick={() => setEditId(editId === d.id ? null : d.id)} style={linkBtn(theme.textMuted)}><Settings2 size={13} /> {t('edit_prefs')}</button>
                    <button onClick={() => onDisconnect(d.id)} style={linkBtn(theme.red)}><Trash2 size={13} /> {t('disconnect')}</button>
                  </div>
                )}
                {isAdmin && editId === d.id && (
                  <DestinationPreferencesPanel lang={lang} destination={d} availableEventTypes={availableEventTypes}
                    availableFilters={availableFilters} onSave={onSave} onClose={() => setEditId(null)} />
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function linkBtn(color: string): React.CSSProperties {
  return { display: 'inline-flex', gap: 5, alignItems: 'center', background: 'transparent', border: 'none', cursor: 'pointer', color, fontSize: 12, padding: 0 };
}
