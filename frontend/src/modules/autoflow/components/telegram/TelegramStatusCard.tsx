import React from 'react';
import { theme, cardStyle, alpha } from '../../constants/styles';
import { strings, type Lang } from './telegramCopy';
import { statusTone, botCredState, publicDeliveryAvailable, webhookState, type Tone } from './logic';
import type { TelegramStatus, TelegramDestination, TelegramBotStatus } from '../../services/telegramIntegrationApi';

const TONE_COLOR: Record<Tone, string> = { ok: theme.green, warn: theme.yellow, off: theme.textFaint };

function Pill({ label, value, ok }: { label: string; value: string; ok: boolean }) {
  const c = ok ? theme.green : theme.textFaint;
  return (
    <span style={{ fontSize: 11.5, color: theme.textMuted }}>
      {label}: <span style={{ color: c, fontWeight: 600 }}>{value}</span>
    </span>
  );
}

function Stat({ label, value, tone }: { label: string; value: React.ReactNode; tone?: string }) {
  return (
    <div>
      <p style={{ color: theme.textFaint, fontSize: 11, margin: '0 0 3px' }}>{label}</p>
      <p style={{ color: tone ?? theme.text, fontSize: 14, fontWeight: 600, margin: 0 }}>{value}</p>
    </div>
  );
}

// Channel-first status: headline state + bot/notification/action flags + active channel count +
// latest delivery/error (derived from destinations). Never renders chat_id or the token.
export const TelegramStatusCard = React.memo(({ lang, status, destinations, bot }: {
  lang: Lang; status: TelegramStatus; destinations: TelegramDestination[]; bot: TelegramBotStatus | null;
}) => {
  const { t } = strings(lang);
  const tone = statusTone(status.status);
  const tc = TONE_COLOR[tone];
  const lastDelivery = destinations.map((d) => d.last_delivery_at).filter(Boolean).sort().slice(-1)[0];
  const lastError = destinations.find((d) => d.last_error)?.last_error || '';
  const credState = botCredState(bot);
  const deliveryOK = publicDeliveryAvailable(bot);

  return (
    <div style={cardStyle({ display: 'grid', gap: 16 })}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
        <span style={{ width: 9, height: 9, borderRadius: 99, background: tc, boxShadow: `0 0 10px ${alpha(tc, 50)}` }} />
        <span style={{ color: tc, fontSize: 14, fontWeight: 700 }}>{t('state_' + status.status)}</span>
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(130px, 1fr))', gap: 14 }}>
        <Stat label={t('bot_configured')} value={status.bot_configured ? t('yes') : t('no')} tone={status.bot_configured ? theme.green : theme.red} />
        <Stat label={t('notifications')} value={status.flags.TELEGRAM_NOTIFY_ENABLED ? t('enabled_word') : t('disabled_word')} />
        <Stat label={t('actions_exec')} value={t('disabled_word')} tone={theme.textFaint} />
        <Stat label={t('active_destinations')} value={status.active_destinations} />
        <Stat label={t('last_delivery')} value={lastDelivery ? new Date(lastDelivery).toLocaleString(lang === 'en' ? 'en-US' : 'vi-VN') : t('never')} />
      </div>
      <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap', paddingTop: 4, borderTop: `1px solid ${theme.border}` }}>
        <Pill label={t('bot_credential')} value={t('cred_' + credState)} ok={credState === 'configured'} />
        <Pill label={t('webhook_label')} value={t('webhook_' + webhookState())} ok={false} />
        <Pill label={t('public_delivery')} value={deliveryOK ? t('avail_available') : t('avail_unavailable')} ok={deliveryOK} />
      </div>
      {lastError && <p style={{ color: theme.red, fontSize: 12.5, margin: 0 }}>{t('last_error')}: {lastError}</p>}
    </div>
  );
});
TelegramStatusCard.displayName = 'TelegramStatusCard';
