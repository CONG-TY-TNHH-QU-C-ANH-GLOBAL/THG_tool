import React, { useState } from 'react';
import { theme, cardStyle, primaryBtn, alpha } from '../../constants/styles';
import { copy, type Lang } from './copy';
import { sanitizeAlertTypes, isValidChannelFilter, DEFAULT_CHANNEL_FILTERS } from './logic';
import type { TelegramAlertPrefs } from '../../services/telegramIntegrationApi';

interface Props {
  lang: Lang; prefs: TelegramAlertPrefs; isAdmin: boolean;
  onSave: (b: { alerts_enabled: boolean; channel_filter: string; alert_types: string[] }) => Promise<void>;
}

// Org-level alert preferences editor. Channel-neutral: filters come from the backend's
// available_filters (fallback to the default catalog). Admin-only edit; members see read-only.
export function TelegramAlertPreferences({ lang, prefs, isAdmin, onSave }: Props) {
  const { t, alertLabel } = copy(lang);
  const [enabled, setEnabled] = useState(prefs.alerts_enabled);
  const [filter, setFilter] = useState(prefs.channel_filter || 'all');
  const [types, setTypes] = useState<string[]>(sanitizeAlertTypes(prefs.alert_types));
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const filters = prefs.available_filters?.length ? prefs.available_filters : [...DEFAULT_CHANNEL_FILTERS];
  const allTypes = prefs.available_types?.length ? prefs.available_types : sanitizeAlertTypes(prefs.alert_types);

  const toggle = (k: string) => setTypes((prev) => (prev.includes(k) ? prev.filter((x) => x !== k) : [...prev, k]));

  const save = async () => {
    if (!isValidChannelFilter(filter, filters)) { setMsg(t('err_generic')); return; }
    setBusy(true); setMsg(null);
    try { await onSave({ alerts_enabled: enabled, channel_filter: filter, alert_types: sanitizeAlertTypes(types) }); setMsg(t('saved')); }
    catch (e) { setMsg(e instanceof Error ? e.message : t('err_generic')); }
    finally { setBusy(false); }
  };

  return (
    <div style={cardStyle({ display: 'grid', gap: 14 })}>
      <p style={{ color: theme.text, fontSize: 14, fontWeight: 650, margin: 0 }}>{t('alerts_title')}</p>

      <label style={{ display: 'flex', gap: 10, alignItems: 'center', cursor: isAdmin ? 'pointer' : 'default' }}>
        <input type="checkbox" checked={enabled} disabled={!isAdmin} onChange={(e) => setEnabled(e.target.checked)} />
        <span style={{ color: theme.textMuted, fontSize: 13 }}>{t('alerts_enabled')}</span>
      </label>

      <div>
        <p style={{ color: theme.textFaint, fontSize: 11, margin: '0 0 6px' }}>{t('channel_filter')}</p>
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
          {filters.map((f) => {
            const on = filter === f;
            return (
              <button key={f} disabled={!isAdmin} onClick={() => setFilter(f)}
                style={{ fontSize: 12, padding: '5px 12px', borderRadius: 99, cursor: isAdmin ? 'pointer' : 'default',
                  border: `1px solid ${on ? theme.primary : theme.border}`, background: on ? alpha(theme.primary, 16) : 'transparent',
                  color: on ? theme.primaryPale : theme.textMuted }}>{f}</button>
            );
          })}
        </div>
      </div>

      <div>
        <p style={{ color: theme.textFaint, fontSize: 11, margin: '0 0 6px' }}>{t('alert_types')}</p>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(190px, 1fr))', gap: 6 }}>
          {allTypes.map((k) => (
            <label key={k} style={{ display: 'flex', gap: 8, alignItems: 'center', cursor: isAdmin ? 'pointer' : 'default' }}>
              <input type="checkbox" checked={types.includes(k)} disabled={!isAdmin} onChange={() => toggle(k)} />
              <span style={{ color: theme.textMuted, fontSize: 12.5 }}>{alertLabel(k)}</span>
            </label>
          ))}
        </div>
      </div>

      {isAdmin && (
        <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
          <button style={primaryBtn({ opacity: busy ? 0.6 : 1 })} disabled={busy} onClick={save}>{t('save')}</button>
          {msg && <span style={{ color: theme.textMuted, fontSize: 12.5 }}>{msg}</span>}
        </div>
      )}
      {!isAdmin && <p style={{ color: theme.textFaint, fontSize: 12 }}>{t('admin_only')}</p>}
    </div>
  );
}
