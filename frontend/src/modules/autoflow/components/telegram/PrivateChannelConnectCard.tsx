import React, { useEffect, useState } from 'react';
import { Copy, Check, RefreshCw } from 'lucide-react';
import { theme, primaryBtn, secondaryBtn, alpha } from '../../constants/styles';
import { strings, type Lang } from './telegramCopy';
import { secondsLeft, formatCountdown } from './logic';
import * as tg from '../../services/telegramIntegrationApi';

// Private-channel connect: generate a one-time code, the admin posts `/connect <code>` in the
// channel; the bot captures it via channel_post. UI shows the code, a live countdown, and a
// "check again" refetch (the page reloads destinations to detect the new channel).
export function PrivateChannelConnectCard({ lang, onCheck }: { lang: Lang; onCheck: () => void }) {
  const { t } = strings(lang);
  const [code, setCode] = useState<tg.ConnectCodeResponse | null>(null);
  const [expiresAt, setExpiresAt] = useState('');
  const [left, setLeft] = useState(0);
  const [copied, setCopied] = useState(false);
  const [busy, setBusy] = useState(false);

  const generate = async () => {
    setBusy(true);
    try {
      const r = await tg.createPrivateChannelConnectCode();
      setCode(r);
      setExpiresAt(new Date(Date.now() + r.ttl_seconds * 1000).toISOString());
    } finally { setBusy(false); }
  };

  useEffect(() => {
    if (!expiresAt) return;
    const tick = () => setLeft(secondsLeft(expiresAt, Date.now()));
    tick();
    const id = setInterval(tick, 1000);
    return () => clearInterval(id);
  }, [expiresAt]);

  const command = code ? `/connect ${code.connect_code}` : '';
  const doCopy = () => {
    try { void navigator.clipboard?.writeText(command); } catch { /* ignore */ }
    setCopied(true); setTimeout(() => setCopied(false), 1500);
  };
  const expired = !!code && left <= 0;

  return (
    <div style={{ display: 'grid', gap: 12 }}>
      <div style={{ display: 'grid', gap: 6 }}>
        <p style={{ color: theme.textMuted, fontSize: 12.5, margin: 0 }}>1. {t('priv_s1')}</p>
        <p style={{ color: theme.textMuted, fontSize: 12.5, margin: 0 }}>2. {t('priv_s2')}</p>
        <p style={{ color: theme.textMuted, fontSize: 12.5, margin: 0 }}>3. {t('priv_s3')}</p>
      </div>
      {!code && (
        <button style={primaryBtn({ opacity: busy ? 0.6 : 1 })} disabled={busy} onClick={generate}>{t('generate_code')}</button>
      )}
      {code && (
        <div style={{ border: `1px solid ${alpha(theme.primary, 30)}`, background: alpha(theme.primary, 6), borderRadius: 'var(--radius-md)', padding: 14, display: 'grid', gap: 10 }}>
          <p style={{ color: theme.textMuted, fontSize: 12.5, margin: 0 }}>{t('priv_post_hint')}</p>
          <code style={{ fontFamily: 'var(--font-mono)', fontSize: 16, color: expired ? theme.textFaint : theme.text }}>{command}</code>
          <span style={{ fontSize: 12.5, color: expired ? theme.red : theme.textMuted }}>
            {expired ? t('code_expired') : `${t('expires_in')} ${formatCountdown(left)}`}
          </span>
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            {expired ? (
              <button style={primaryBtn({ padding: '8px 14px', fontSize: 13 })} onClick={generate}><RefreshCw size={14} /> {t('generate_code')}</button>
            ) : (
              <>
                <button style={secondaryBtn({ padding: '8px 14px', fontSize: 13 })} onClick={doCopy}>
                  {copied ? <Check size={14} /> : <Copy size={14} />} {copied ? t('copied') : t('copy')}
                </button>
                <button style={primaryBtn({ padding: '8px 14px', fontSize: 13 })} onClick={onCheck}>{t('priv_check_again')}</button>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
