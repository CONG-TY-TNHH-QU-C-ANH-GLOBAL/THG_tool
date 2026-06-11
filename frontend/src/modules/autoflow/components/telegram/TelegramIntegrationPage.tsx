'use client';
import React, { useState } from 'react';
import { Send, AlertTriangle } from 'lucide-react';
import { theme, alpha } from '../../constants/styles';
import { useLang } from '../../i18n/useLang';
import { useAuthStore } from '../../stores/authStore';
import { useTelegramIntegration } from '../../hooks/useTelegramIntegration';
import { destinationReasons, canManageChannels, botReady } from './logic';
import { strings } from './telegramCopy';
import { TelegramSafetyNotice } from './TelegramSafetyNotice';
import { TelegramBotCredentialCard } from './TelegramBotCredentialCard';
import { TelegramStatusCard } from './TelegramStatusCard';
import { TelegramNeedsAttention } from './TelegramNeedsAttention';
import { TelegramChannelDestinationsTable } from './TelegramChannelDestinationsTable';
import { TelegramChannelSetupWizard } from './TelegramChannelSetupWizard';
import { NotificationPreview } from './NotificationPreview';
import { PersonalBindingsTable } from './PersonalBindingsTable';
import { PersonalDmConnect } from './PersonalDmConnect';
import { TelegramAuditPanel } from './TelegramAuditPanel';
import { TelegramEmptyState } from './TelegramEmptyState';

// Settings → Integrations → Telegram (channel-first). Step 1 = connect the workspace's OWN bot;
// channel connect is gated until the org bot is configured. DM bindings are secondary.
export default function TelegramIntegrationPage({ isAdmin }: { orgId: string; isAdmin: boolean }) {
  const { lang } = useLang();
  const { t } = strings(lang);
  const currentUserId = useAuthStore((s) => s.user?.id ?? 0);
  const d = useTelegramIntegration(isAdmin);
  const [showWizard, setShowWizard] = useState(false);
  const admin = canManageChannels(isAdmin);

  const header = (
    <div style={{ display: 'flex', gap: 10, alignItems: 'center', marginBottom: 4 }}>
      <Send size={18} color={theme.primary} />
      <div>
        <p style={{ color: theme.text, fontSize: 16, fontWeight: 700, margin: 0 }}>{t('title')}</p>
        <p style={{ color: theme.textMuted, fontSize: 12.5, margin: '2px 0 0' }}>{t('subtitle')}</p>
      </div>
    </div>
  );

  if (d.loading) {
    return <div style={{ display: 'grid', gap: 16 }}>{header}<div className="skeleton" style={{ height: 160, borderRadius: 12 }} /></div>;
  }
  if (d.error || !d.status) {
    return <div style={{ display: 'grid', gap: 16 }}>{header}<p style={{ color: theme.red, fontSize: 13 }}>{d.error ?? t('err_generic')}</p></div>;
  }

  const s = d.status;
  const botOK = botReady(d.bot);
  const noChannels = d.destinations.length === 0;
  const lastError = d.destinations.find((x) => x.last_error)?.last_error || '';
  const reasons = destinationReasons(lastError, s.flags.TELEGRAM_NOTIFY_ENABLED, s.bot_configured);

  return (
    <div style={{ display: 'grid', gap: 16 }}>
      {header}
      <TelegramSafetyNotice lang={lang} />
      <TelegramStatusCard lang={lang} status={s} destinations={d.destinations} bot={d.bot} />

      {/* Step 1 — connect the workspace bot (always shown; compact once configured) */}
      <TelegramBotCredentialCard lang={lang} bot={d.bot} isAdmin={admin} onSave={d.saveBot} onRemove={d.removeBot} />

      {/* Channel section is GATED on a configured org bot */}
      {!botOK ? (
        <div role="note" style={{ display: 'flex', gap: 9, alignItems: 'center', background: alpha(theme.yellow, 8), border: `1px solid ${alpha(theme.yellow, 30)}`, borderRadius: 'var(--radius-md)', padding: '12px 14px' }}>
          <AlertTriangle size={15} color={theme.yellow} />
          <span style={{ color: theme.textMuted, fontSize: 12.5 }}>{t('bot_required_for_channel')}</span>
        </div>
      ) : noChannels ? (
        <>
          <TelegramEmptyState lang={lang} isAdmin={admin} onConnect={() => setShowWizard(true)} />
          {admin && showWizard && <TelegramChannelSetupWizard lang={lang} onConnected={d.reload} />}
        </>
      ) : (
        <>
          <TelegramNeedsAttention lang={lang} reasons={reasons} />
          <TelegramChannelDestinationsTable
            lang={lang} destinations={d.destinations} isAdmin={admin}
            availableEventTypes={d.availableEventTypes} availableFilters={d.availableFilters}
            onSave={d.savePreferences} onDisconnect={d.disconnect} onReload={d.reload}
          />
          {admin && <TelegramChannelSetupWizard lang={lang} onConnected={d.reload} />}
          <NotificationPreview lang={lang} />
        </>
      )}

      {/* Secondary: optional personal DM bindings (depend on the platform/dev webhook bot) */}
      <PersonalBindingsTable lang={lang} bindings={d.bindings} isAdmin={d.canManageAll} currentUserId={currentUserId} onRevoke={d.revoke} />
      <p style={{ color: theme.textFaint, fontSize: 11.5, margin: 0 }}>{t('personal_dm_note')}</p>
      <PersonalDmConnect lang={lang} />

      <TelegramAuditPanel lang={lang} events={d.audit} isAdmin={isAdmin} />
    </div>
  );
}
