import React, { useState } from 'react';
import { theme, cardStyle, primaryBtn, secondaryBtn, alpha } from '../../constants/styles';
import { copy, type Lang } from './copy';
import { canTestNotification } from './logic';
import { TelegramBindCodeCard } from './TelegramBindCodeCard';
import * as tg from '../../services/telegramIntegrationApi';

function Step({ n, text }: { n: number; text: string }) {
  return (
    <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
      <span style={{ width: 20, height: 20, borderRadius: 99, background: alpha(theme.primary, 16), color: theme.primary, fontSize: 11, fontWeight: 700, display: 'grid', placeItems: 'center', flexShrink: 0 }}>{n}</span>
      <span style={{ color: theme.textMuted, fontSize: 13 }}>{text}</span>
    </div>
  );
}

interface Props {
  lang: Lang; isAdmin: boolean; enabled: boolean; notifyEnabled: boolean; activeBindings: number;
  onSetEnabled: (v: boolean) => Promise<void>;
}

// Guided setup workflow. Holds the generate-code + test-notification actions; pure gating comes
// from logic.canTestNotification.
export function TelegramSetupGuide({ lang, isAdmin, enabled, notifyEnabled, activeBindings, onSetEnabled }: Props) {
  const { t } = copy(lang);
  const [code, setCode] = useState<tg.BindCodeResponse | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const run = async (fn: () => Promise<void>) => {
    setBusy(true); setMsg(null);
    try { await fn(); } catch (e) { setMsg(e instanceof Error ? e.message : t('err_generic')); }
    finally { setBusy(false); }
  };
  const generate = () => run(async () => setCode(await tg.createBindCode()));
  const test = () => run(async () => { await tg.sendTestNotification(); setMsg(t('saved')); });

  return (
    <div style={cardStyle({ display: 'grid', gap: 14 })}>
      <p style={{ color: theme.text, fontSize: 14, fontWeight: 650, margin: 0 }}>{t('setup_title')}</p>
      <div style={{ display: 'grid', gap: 8 }}>
        <Step n={1} text={t('step_enable')} />
        <Step n={2} text={t('step_code')} />
        <Step n={3} text={t('step_open_bot')} />
        <Step n={4} text={t('step_bind')} />
        <Step n={5} text={t('step_test')} />
      </div>

      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        {isAdmin && !enabled && (
          <button style={primaryBtn({ opacity: busy ? 0.6 : 1 })} disabled={busy} onClick={() => run(() => onSetEnabled(true))}>{t('enable')}</button>
        )}
        {isAdmin && enabled && (
          <button style={secondaryBtn({ opacity: busy ? 0.6 : 1 })} disabled={busy} onClick={() => run(() => onSetEnabled(false))}>{t('disable')}</button>
        )}
        <button style={primaryBtn({ opacity: busy ? 0.6 : 1 })} disabled={busy} onClick={generate}>{t('generate_code')}</button>
        <button
          style={secondaryBtn({ opacity: canTestNotification(notifyEnabled, activeBindings) && !busy ? 1 : 0.5 })}
          disabled={busy || !canTestNotification(notifyEnabled, activeBindings)}
          onClick={test}
        >{t('test_notification')}</button>
      </div>

      {code && <TelegramBindCodeCard lang={lang} code={code} onRegenerate={generate} />}
      {msg && <p style={{ color: theme.textMuted, fontSize: 12.5, margin: 0 }}>{msg}</p>}
    </div>
  );
}
