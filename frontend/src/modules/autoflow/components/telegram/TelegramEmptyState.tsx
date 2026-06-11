import React from 'react';
import { Send } from 'lucide-react';
import { theme, cardStyle, primaryBtn, alpha } from '../../constants/styles';
import { copy, type Lang } from './copy';

// "Not connected yet" empty state with the enable CTA. Render-only; the CTA is delegated up.
export const TelegramEmptyState = React.memo(
  ({ lang, isAdmin, onEnable, busy }: { lang: Lang; isAdmin: boolean; onEnable: () => void; busy: boolean }) => {
    const { t } = copy(lang);
    return (
      <div style={cardStyle({ display: 'flex', flexDirection: 'column', alignItems: 'center', textAlign: 'center', gap: 12, padding: 36 })}>
        <div style={{ width: 52, height: 52, borderRadius: 14, display: 'grid', placeItems: 'center', background: alpha(theme.primary, 14) }}>
          <Send size={24} color={theme.primary} />
        </div>
        <p style={{ color: theme.text, fontSize: 16, fontWeight: 650, margin: 0 }}>{t('empty_title')}</p>
        <p style={{ color: theme.textMuted, fontSize: 13, maxWidth: 420, lineHeight: 1.6, margin: 0 }}>{t('empty_body')}</p>
        {isAdmin && (
          <button style={primaryBtn({ marginTop: 4, opacity: busy ? 0.6 : 1 })} disabled={busy} onClick={onEnable}>
            {t('enable')}
          </button>
        )}
        {!isAdmin && <p style={{ color: theme.textFaint, fontSize: 12 }}>{t('admin_only')}</p>}
      </div>
    );
  },
);
TelegramEmptyState.displayName = 'TelegramEmptyState';
