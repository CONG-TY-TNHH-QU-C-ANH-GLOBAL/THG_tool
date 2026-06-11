import React, { useState } from 'react';
import { theme, primaryBtn, secondaryBtn, alpha } from '../../constants/styles';
import { strings, eventLabel, groupLabel, type Lang } from './telegramCopy';
import { EVENT_GROUPS, sanitizeEventTypes } from './logic';
import type { TelegramDestination } from '../../services/telegramIntegrationApi';

interface Props {
  lang: Lang; destination: TelegramDestination; availableEventTypes: string[]; availableFilters: string[];
  onSave: (id: number, body: { event_types: string[]; channel_filter: string }) => Promise<void>;
  onClose: () => void;
}

// Per-destination event subscriptions + channel filter + (future) delivery mode. Admin edits which
// events this channel receives. Channel-neutral (filters come from the backend).
export function DestinationPreferencesPanel({ lang, destination, availableFilters, onSave, onClose }: Props) {
  const { t } = strings(lang);
  const [types, setTypes] = useState<string[]>(destination.event_types || []);
  const [filter, setFilter] = useState(destination.channel_filter || 'all');
  const [busy, setBusy] = useState(false);
  const filters = availableFilters?.length ? availableFilters : ['all', 'facebook', 'taobao', '1688'];

  const toggle = (k: string) => setTypes((p) => (p.includes(k) ? p.filter((x) => x !== k) : [...p, k]));
  const save = async () => {
    setBusy(true);
    try { await onSave(destination.id, { event_types: sanitizeEventTypes(types), channel_filter: filter }); onClose(); }
    finally { setBusy(false); }
  };

  return (
    <div style={{ border: `1px solid ${theme.border}`, borderRadius: 'var(--radius-md)', padding: 14, display: 'grid', gap: 12, background: 'var(--bg-elev-2)' }}>
      <p style={{ color: theme.text, fontSize: 13.5, fontWeight: 650, margin: 0 }}>{t('prefs_title')} — {destination.title}</p>
      <p style={{ color: theme.textFaint, fontSize: 12, margin: 0 }}>{t('prefs_hint')}</p>

      {EVENT_GROUPS.map((g) => (
        <div key={g.key}>
          <p style={{ color: theme.textFaint, fontSize: 11, margin: '0 0 5px' }}>{groupLabel(lang, g.key)}</p>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(170px, 1fr))', gap: 5 }}>
            {g.types.map((k) => (
              <label key={k} style={{ display: 'flex', gap: 7, alignItems: 'center', cursor: 'pointer' }}>
                <input type="checkbox" checked={types.includes(k)} onChange={() => toggle(k)} />
                <span style={{ color: theme.textMuted, fontSize: 12 }}>{eventLabel(lang, k)}</span>
              </label>
            ))}
          </div>
        </div>
      ))}

      <div>
        <p style={{ color: theme.textFaint, fontSize: 11, margin: '0 0 5px' }}>{t('channel_filter')}</p>
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
          {filters.map((f) => {
            const on = filter === f;
            return (
              <button key={f} onClick={() => setFilter(f)} style={{
                fontSize: 12, padding: '4px 11px', borderRadius: 99, cursor: 'pointer',
                border: `1px solid ${on ? theme.primary : theme.border}`, background: on ? alpha(theme.primary, 16) : 'transparent',
                color: on ? theme.primaryPale : theme.textMuted,
              }}>{f}</button>
            );
          })}
        </div>
      </div>

      <div>
        <p style={{ color: theme.textFaint, fontSize: 11, margin: '0 0 5px' }}>{t('delivery_mode')}</p>
        <span style={{ fontSize: 12, color: theme.textMuted }}>{t('mode_immediate')} · <span style={{ color: theme.textFaint }}>{t('mode_digest')}</span></span>
      </div>

      <div style={{ display: 'flex', gap: 8 }}>
        <button style={primaryBtn({ padding: '8px 16px', opacity: busy ? 0.6 : 1 })} disabled={busy} onClick={save}>{t('save')}</button>
        <button style={secondaryBtn({ padding: '8px 16px' })} onClick={onClose}>{t('cancel')}</button>
      </div>
    </div>
  );
}
