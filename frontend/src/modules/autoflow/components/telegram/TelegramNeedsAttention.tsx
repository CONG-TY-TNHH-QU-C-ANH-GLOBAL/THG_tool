import React from 'react';
import { AlertTriangle } from 'lucide-react';
import { theme, alpha } from '../../constants/styles';
import { copy, type Lang } from './copy';

// Renders remediation reasons (keys derived by logic.needsAttentionReasons). Render-only.
export const TelegramNeedsAttention = React.memo(({ lang, reasons }: { lang: Lang; reasons: string[] }) => {
  if (!reasons.length) return null;
  const { t } = copy(lang);
  return (
    <div style={{ background: alpha(theme.yellow, 8), border: `1px solid ${alpha(theme.yellow, 30)}`, borderRadius: 'var(--radius-md)', padding: '12px 14px' }}>
      <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginBottom: 8 }}>
        <AlertTriangle size={15} color={theme.yellow} />
        <span style={{ color: theme.text, fontSize: 13, fontWeight: 600 }}>{t('needs_title')}</span>
      </div>
      <ul style={{ margin: 0, paddingLeft: 18, display: 'grid', gap: 4 }}>
        {reasons.map((r) => (
          <li key={r} style={{ color: theme.textMuted, fontSize: 12.5, lineHeight: 1.5 }}>{t('reason_' + r)}</li>
        ))}
      </ul>
    </div>
  );
});
TelegramNeedsAttention.displayName = 'TelegramNeedsAttention';
