import React from 'react';
import { ShieldCheck } from 'lucide-react';
import { theme, alpha } from '../../constants/styles';
import { strings, type Lang } from './telegramCopy';

// Always-visible notice that Telegram action EXECUTION is disabled. Render-only.
export const TelegramSafetyNotice = React.memo(({ lang }: { lang: Lang }) => {
  const { t } = strings(lang);
  return (
    <div
      role="note"
      style={{
        display: 'flex', gap: 10, alignItems: 'flex-start',
        background: alpha(theme.green, 10), border: `1px solid ${alpha(theme.green, 30)}`,
        borderRadius: 'var(--radius-md)', padding: '12px 14px',
      }}
    >
      <ShieldCheck size={16} color={theme.green} style={{ flexShrink: 0, marginTop: 1 }} />
      <p style={{ color: theme.textMuted, fontSize: 12.5, lineHeight: 1.5, margin: 0 }}>{t('safety_notice')}</p>
    </div>
  );
});
TelegramSafetyNotice.displayName = 'TelegramSafetyNotice';
