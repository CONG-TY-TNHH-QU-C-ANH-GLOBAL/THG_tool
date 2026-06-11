import React, { useState } from 'react';
import { Bot, Check, ShieldAlert, Trash2 } from 'lucide-react';
import { theme, cardStyle, inputStyle, primaryBtn, secondaryBtn, alpha } from '../../constants/styles';
import { strings, type Lang } from './telegramCopy';
import { botCredState } from './logic';
import type { TelegramBotStatus } from '../../services/telegramIntegrationApi';

interface Props {
  lang: Lang; bot: TelegramBotStatus | null; isAdmin: boolean;
  onSave: (token: string) => Promise<void>; onRemove: () => Promise<void>;
}

// Step 1: connect the workspace's OWN Telegram bot. The token is entered once and NEVER shown
// again (only @username + last4 + status). Channel connect/delivery uses this org-scoped bot.
export function TelegramBotCredentialCard({ lang, bot, isAdmin, onSave, onRemove }: Props) {
  const { t } = strings(lang);
  const [token, setToken] = useState('');
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [replacing, setReplacing] = useState(false);
  const state = botCredState(bot);
  const configured = state === 'configured';

  const save = async () => {
    if (!token.trim()) return;
    setBusy(true); setErr(null);
    try { await onSave(token.trim()); setToken(''); setReplacing(false); }
    catch (e) {
      const code = e instanceof Error ? e.message : '';
      setErr(code ? t('reason_' + code) : t('err_generic'));
    } finally { setBusy(false); }
  };

  // Connected view (with optional replace form).
  if (configured && !replacing) {
    const verified = bot?.last_verified_at ? new Date(bot.last_verified_at).toLocaleString(lang === 'en' ? 'en-US' : 'vi-VN') : '—';
    return (
      <div style={cardStyle({ display: 'grid', gap: 10 })}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 9 }}>
          <Check size={16} color={theme.green} />
          <span style={{ color: theme.text, fontSize: 14, fontWeight: 650 }}>{t('bot_connected')}: @{bot?.bot_username}</span>
        </div>
        <div style={{ display: 'flex', gap: 18, flexWrap: 'wrap', fontSize: 12.5, color: theme.textMuted }}>
          <span>{bot?.bot_display_name}</span>
          <span>{t('bot_last4')}: ••••{bot?.token_last4}</span>
          <span>{t('bot_last_verified')}: {verified}</span>
        </div>
        {isAdmin && (
          <div style={{ display: 'flex', gap: 10 }}>
            <button style={secondaryBtn({ padding: '7px 14px', fontSize: 12.5 })} onClick={() => setReplacing(true)}>{t('bot_replace')}</button>
            <button style={secondaryBtn({ padding: '7px 14px', fontSize: 12.5, color: theme.red })} onClick={() => void onRemove()}>
              <Trash2 size={13} /> {t('bot_revoke')}
            </button>
          </div>
        )}
      </div>
    );
  }

  // Setup / replace / invalid / revoked → show the connect form.
  return (
    <div style={cardStyle({ display: 'grid', gap: 12, border: `1px solid ${alpha(theme.primary, 30)}` })}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 9 }}>
        {state === 'invalid' ? <ShieldAlert size={16} color={theme.yellow} /> : <Bot size={16} color={theme.primary} />}
        <span style={{ color: theme.text, fontSize: 14, fontWeight: 650 }}>{t('bot_step_title')}</span>
      </div>
      {!isAdmin ? (
        <p style={{ color: theme.textMuted, fontSize: 12.5, margin: 0 }}>{t('admin_only')}</p>
      ) : (
        <>
          <p style={{ color: theme.textMuted, fontSize: 12.5, margin: 0 }}>{t('bot_step_desc')}</p>
          <div>
            <p style={{ color: theme.textFaint, fontSize: 11, margin: '0 0 5px' }}>{t('bot_token_label')}</p>
            <input style={inputStyle} type="password" autoComplete="off" placeholder="123456:ABC-DEF…"
              value={token} onChange={(e) => setToken(e.target.value)} disabled={busy} />
            <p style={{ color: theme.textFaint, fontSize: 11, margin: '5px 0 0' }}>{t('bot_never_shown')}</p>
          </div>
          <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
            <button style={primaryBtn({ opacity: busy || !token.trim() ? 0.6 : 1 })} disabled={busy || !token.trim()} onClick={save}>{t('bot_verify_save')}</button>
            {replacing && <button style={secondaryBtn({ padding: '9px 16px' })} onClick={() => { setReplacing(false); setErr(null); }}>{t('cancel')}</button>}
            {err && <span style={{ color: theme.red, fontSize: 12.5 }}>{err}</span>}
          </div>
        </>
      )}
    </div>
  );
}
