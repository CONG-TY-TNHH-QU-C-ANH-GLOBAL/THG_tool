import React from 'react';
import { Send, Check } from 'lucide-react';
import { theme, cardStyle, primaryBtn, alpha } from '../../constants/styles';
import { copy, type Lang } from './copy';

// Channel-first empty state: benefits + "Connect Telegram channel" CTA. Render-only; CTA delegated.
export const TelegramEmptyState = React.memo(
  ({ lang, isAdmin, onConnect }: { lang: Lang; isAdmin: boolean; onConnect: () => void }) => {
    const { t } = copy(lang);
    return (
      <div style={cardStyle({ display: 'flex', flexDirection: 'column', alignItems: 'center', textAlign: 'center', gap: 12, padding: 36 })}>
        <div style={{ width: 52, height: 52, borderRadius: 14, display: 'grid', placeItems: 'center', background: alpha(theme.primary, 14) }}>
          <Send size={24} color={theme.primary} />
        </div>
        <p style={{ color: theme.text, fontSize: 16, fontWeight: 650, margin: 0 }}>{t('empty_title')}</p>
        <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'grid', gap: 6, textAlign: 'left' }}>
          {['empty_b1', 'empty_b2', 'empty_b3'].map((k) => (
            <li key={k} style={{ display: 'flex', gap: 8, alignItems: 'center', color: theme.textMuted, fontSize: 13 }}>
              <Check size={15} color={theme.green} /> {t(k)}
            </li>
          ))}
        </ul>
        {isAdmin ? (
          <button style={primaryBtn({ marginTop: 4 })} onClick={onConnect}>{t('empty_cta')}</button>
        ) : (
          <p style={{ color: theme.textFaint, fontSize: 12 }}>{t('admin_only')}</p>
        )}
      </div>
    );
  },
);
TelegramEmptyState.displayName = 'TelegramEmptyState';
