import React, { useState } from 'react';
import { theme, inputStyle, primaryBtn, alpha } from '../../constants/styles';
import { strings, type Lang } from './telegramCopy';
import * as tg from '../../services/telegramIntegrationApi';

function Step({ n, text }: Readonly<{ n: number; text: string }>) {
  return (
    <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
      <span style={{ width: 18, height: 18, borderRadius: 99, background: alpha(theme.primary, 16), color: theme.primary, fontSize: 10, fontWeight: 700, display: 'grid', placeItems: 'center', flexShrink: 0 }}>{n}</span>
      <span style={{ color: theme.textMuted, fontSize: 12.5 }}>{text}</span>
    </div>
  );
}

// Public-channel connect: instructions + @username field → backend sends a test message and stores
// the chat id/title Telegram returns (never shown). Surfaces verifying / connected / error states.
export function PublicChannelConnectCard({ lang, onConnected }: Readonly<{ lang: Lang; onConnected: () => void }>) {
  const { t } = strings(lang);
  const [username, setUsername] = useState('');
  const [state, setState] = useState<'idle' | 'verifying' | 'connected'>('idle');
  const [err, setErr] = useState<string | null>(null);

  const connect = async () => {
    if (!username.trim()) { setErr(t('reason_username_required')); return; }
    setState('verifying'); setErr(null);
    try {
      await tg.connectPublicChannel(username.trim());
      setState('connected');
      onConnected();
    } catch (e) {
      setState('idle');
      // Map the backend reason code (bot_token_missing, bot_not_channel_admin, …) to a clear
      // message; fall back to the generic error for anything unmapped.
      const code = e instanceof Error ? e.message : '';
      const mapped = t('reason_' + code);
      setErr(code && mapped !== 'reason_' + code ? mapped : t('err_generic'));
    }
  };

  return (
    <div style={{ display: 'grid', gap: 12 }}>
      <div style={{ display: 'grid', gap: 6 }}>
        <Step n={1} text={t('pub_s1')} />
        <Step n={2} text={t('pub_s2')} />
        <Step n={3} text={t('pub_s3')} />
      </div>
      <div>
        <p style={{ color: theme.textFaint, fontSize: 11, margin: '0 0 5px' }}>{t('pub_username_label')}</p>
        <input style={inputStyle} placeholder="@my_channel" value={username}
          onChange={(e) => setUsername(e.target.value)} disabled={state === 'verifying'} />
      </div>
      {state === 'connected' ? (
        <span style={{ color: theme.green, fontSize: 13 }}>✅ {t('connected_ok')}</span>
      ) : (
        <button style={primaryBtn({ opacity: state === 'verifying' ? 0.6 : 1 })} disabled={state === 'verifying'} onClick={connect}>
          {state === 'verifying' ? t('verifying') : t('verify_connect')}
        </button>
      )}
      {err && <span style={{ color: theme.red, fontSize: 12.5 }}>{err}</span>}
    </div>
  );
}
