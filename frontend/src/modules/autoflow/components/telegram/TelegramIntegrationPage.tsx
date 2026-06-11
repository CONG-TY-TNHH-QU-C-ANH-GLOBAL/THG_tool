'use client';
import React from 'react';
import { Send } from 'lucide-react';
import { theme } from '../../constants/styles';
import { useLang } from '../../i18n/useLang';
import { useAuthStore } from '../../stores/authStore';
import { useTelegramIntegration } from '../../hooks/useTelegramIntegration';
import { needsAttentionReasons, canManageAllBindings } from './logic';
import { copy } from './copy';
import { TelegramSafetyNotice } from './TelegramSafetyNotice';
import { TelegramStatusCard } from './TelegramStatusCard';
import { TelegramNeedsAttention } from './TelegramNeedsAttention';
import { TelegramSetupGuide } from './TelegramSetupGuide';
import { TelegramBindingsTable } from './TelegramBindingsTable';
import { TelegramAlertPreferences } from './TelegramAlertPreferences';
import { TelegramAuditPanel } from './TelegramAuditPanel';
import { TelegramEmptyState } from './TelegramEmptyState';

// Settings → Integrations → Telegram. Orchestrates the feature: loads data via the hook and
// composes the render-only cards. Channel-neutral; role-gated (admin sees all bindings + audit +
// edit controls; members see their own binding read-mostly).
export default function TelegramIntegrationPage({ isAdmin }: { orgId: string; isAdmin: boolean }) {
  const { lang } = useLang();
  const { t } = copy(lang);
  const currentUserId = useAuthStore((s) => s.user?.id ?? 0);
  const d = useTelegramIntegration(isAdmin);

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
    return (
      <div style={{ display: 'grid', gap: 16 }}>
        {header}
        <div className="skeleton" style={{ height: 160, borderRadius: 12 }} />
      </div>
    );
  }
  if (d.error || !d.status) {
    return (
      <div style={{ display: 'grid', gap: 16 }}>
        {header}
        <p style={{ color: theme.red, fontSize: 13 }}>{d.error ?? t('err_generic')}</p>
      </div>
    );
  }

  const s = d.status;
  const showEmpty = s.status === 'not_connected' && d.bindings.length === 0;

  return (
    <div style={{ display: 'grid', gap: 16 }}>
      {header}
      <TelegramSafetyNotice lang={lang} />
      {showEmpty ? (
        <TelegramEmptyState lang={lang} isAdmin={isAdmin} busy={false} onEnable={() => void d.setEnabled(true)} />
      ) : (
        <>
          <TelegramStatusCard lang={lang} status={s} />
          <TelegramNeedsAttention lang={lang} reasons={needsAttentionReasons(s)} />
          <TelegramSetupGuide
            lang={lang} isAdmin={isAdmin} enabled={s.enabled}
            notifyEnabled={s.flags.TELEGRAM_NOTIFY_ENABLED} activeBindings={s.bound_users}
            onSetEnabled={d.setEnabled}
          />
          <TelegramBindingsTable
            lang={lang} bindings={d.bindings} isAdmin={canManageAllBindings(d.canManageAll)}
            currentUserId={currentUserId} onRevoke={d.revoke}
          />
          {d.alerts && <TelegramAlertPreferences lang={lang} prefs={d.alerts} isAdmin={isAdmin} onSave={d.saveAlerts} />}
          <TelegramAuditPanel lang={lang} events={d.audit} isAdmin={isAdmin} />
        </>
      )}
    </div>
  );
}
