import React, { useEffect, useState } from 'react';
import { Copy, Check, ExternalLink } from 'lucide-react';
import { theme, alpha, primaryBtn, secondaryBtn } from '../../constants/styles';
import { copy, type Lang } from './copy';
import { secondsLeft, formatCountdown } from './logic';
import type { BindCodeResponse } from '../../services/telegramIntegrationApi';

// Shows the one-time code with a live expiry countdown, a copy button, the bot deep link, and the
// /bind instruction. Drives an expired state once the countdown hits zero.
export const TelegramBindCodeCard = React.memo(
  ({ lang, code, onRegenerate }: { lang: Lang; code: BindCodeResponse; onRegenerate: () => void }) => {
    const { t } = copy(lang);
    const [left, setLeft] = useState(() => secondsLeft(code.expires_at, Date.now()));
    const [copied, setCopied] = useState(false);

    useEffect(() => {
      setLeft(secondsLeft(code.expires_at, Date.now()));
      const id = setInterval(() => setLeft(secondsLeft(code.expires_at, Date.now())), 1000);
      return () => clearInterval(id);
    }, [code.expires_at]);

    const expired = left <= 0;
    const doCopy = () => {
      try { void navigator.clipboard?.writeText(code.code); } catch { /* clipboard unavailable */ }
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    };

    return (
      <div style={{ border: `1px solid ${alpha(theme.primary, 30)}`, background: alpha(theme.primary, 6), borderRadius: 'var(--radius-md)', padding: 16, display: 'grid', gap: 12 }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 26, fontWeight: 700, letterSpacing: '0.18em', color: expired ? theme.textFaint : theme.text }}>
            {code.code}
          </span>
          <span style={{ fontSize: 12.5, color: expired ? theme.red : theme.textMuted }}>
            {expired ? t('code_expired') : `${t('expires_in')} ${formatCountdown(left)}`}
          </span>
        </div>
        {!expired && (
          <p style={{ fontSize: 12.5, color: theme.textMuted, margin: 0 }}>
            {t('bind_hint')} <code style={{ color: theme.text, fontFamily: 'var(--font-mono)' }}>/bind {code.code}</code>
          </p>
        )}
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
          {expired ? (
            <button style={primaryBtn()} onClick={onRegenerate}>{t('generate_code')}</button>
          ) : (
            <>
              <button style={secondaryBtn({ padding: '8px 14px', fontSize: 13 })} onClick={doCopy}>
                {copied ? <Check size={14} /> : <Copy size={14} />} {copied ? t('copied') : t('copy')}
              </button>
              {code.deep_link && (
                <a href={code.deep_link} target="_blank" rel="noreferrer" style={{ ...primaryBtn({ padding: '8px 14px', fontSize: 13, textDecoration: 'none' }) }}>
                  <ExternalLink size={14} /> {t('open_bot')}
                </a>
              )}
            </>
          )}
        </div>
      </div>
    );
  },
);
TelegramBindCodeCard.displayName = 'TelegramBindCodeCard';
