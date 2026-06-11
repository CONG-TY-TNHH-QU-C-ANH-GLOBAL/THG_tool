import React from 'react';
import { theme, cardStyle, alpha } from '../../constants/styles';
import { copy, type Lang } from './copy';
import { statusTone, type Tone } from './logic';
import type { TelegramStatus } from '../../services/telegramIntegrationApi';

const TONE_COLOR: Record<Tone, string> = { ok: theme.green, warn: theme.yellow, off: theme.textFaint };

function Stat({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div>
      <p style={{ color: theme.textFaint, fontSize: 11, margin: '0 0 3px' }}>{label}</p>
      <p style={{ color: theme.text, fontSize: 14, fontWeight: 600, margin: 0 }}>{value}</p>
    </div>
  );
}

function flagRow(name: string, on: boolean) {
  const c = on ? theme.green : theme.textFaint;
  return (
    <span key={name} style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: c, border: `1px solid ${alpha(c, 30)}`, background: alpha(c, 8), borderRadius: 6, padding: '2px 7px' }}>
      {name}={String(on)}
    </span>
  );
}

// Headline status + bot/webhook health + counts + channels + feature flags. Render-only.
export const TelegramStatusCard = React.memo(({ lang, status }: { lang: Lang; status: TelegramStatus }) => {
  const { t } = copy(lang);
  const tone = statusTone(status.status);
  const tc = TONE_COLOR[tone];
  const webhook = status.webhook_last_at ? new Date(status.webhook_last_at).toLocaleString(lang === 'en' ? 'en-US' : 'vi-VN') : t('never');
  return (
    <div style={cardStyle({ display: 'grid', gap: 16 })}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
        <span style={{ width: 9, height: 9, borderRadius: 99, background: tc, boxShadow: `0 0 10px ${alpha(tc, 50)}` }} />
        <span style={{ color: tc, fontSize: 14, fontWeight: 700 }}>{t('state_' + status.status)}</span>
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(120px, 1fr))', gap: 14 }}>
        <Stat label={t('bot_configured')} value={status.bot_configured ? t('yes') : t('no')} />
        <Stat label={t('last_webhook')} value={webhook} />
        <Stat label={t('bound_users')} value={status.bound_users} />
        <Stat label={t('alert_recipients')} value={status.alert_recipients} />
      </div>
      <div>
        <p style={{ color: theme.textFaint, fontSize: 11, margin: '0 0 6px' }}>{t('channels')}</p>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
          {status.channels.map((ch) => {
            const c = ch.active ? theme.green : theme.textFaint;
            return (
              <span key={ch.channel} style={{ fontSize: 11.5, color: c, border: `1px solid ${alpha(c, 30)}`, background: alpha(c, 8), borderRadius: 99, padding: '2px 9px' }}>
                {ch.label}{ch.active ? '' : ' · soon'}
              </span>
            );
          })}
        </div>
      </div>
      <div>
        <p style={{ color: theme.textFaint, fontSize: 11, margin: '0 0 6px' }}>{t('flags')}</p>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
          {flagRow('TELEGRAM_BOT_ENABLED', status.flags.TELEGRAM_BOT_ENABLED)}
          {flagRow('TELEGRAM_NOTIFY_ENABLED', status.flags.TELEGRAM_NOTIFY_ENABLED)}
          {flagRow('TELEGRAM_ACTIONS_ENABLED', status.flags.TELEGRAM_ACTIONS_ENABLED)}
        </div>
      </div>
    </div>
  );
});
TelegramStatusCard.displayName = 'TelegramStatusCard';
