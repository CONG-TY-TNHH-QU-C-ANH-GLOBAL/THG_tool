import React, { useState } from 'react';
import { Send } from 'lucide-react';
import { theme, secondaryBtn } from '../../constants/styles';
import { strings, type Lang } from './telegramCopy';
import * as tg from '../../services/telegramIntegrationApi';

// Sends a test notification to one destination and shows the delivered/failed result inline.
// On failure the parent destination flips to needs_attention (server-side) and the table reloads.
export function TestNotificationButton({ lang, destinationId, onResult }: {
  lang: Lang; destinationId: number; onResult?: () => void;
}) {
  const { t } = strings(lang);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);

  const run = async () => {
    setBusy(true); setMsg(null);
    try {
      await tg.testDestination(destinationId);
      setMsg({ ok: true, text: t('test_delivered') });
    } catch (e) {
      setMsg({ ok: false, text: e instanceof Error ? e.message : t('test_failed') });
    } finally {
      setBusy(false);
      onResult?.();
    }
  };

  return (
    <span style={{ display: 'inline-flex', gap: 8, alignItems: 'center' }}>
      <button style={secondaryBtn({ padding: '6px 12px', fontSize: 12, opacity: busy ? 0.6 : 1 })} disabled={busy} onClick={run}>
        <Send size={13} /> {t('test')}
      </button>
      {msg && <span style={{ fontSize: 12, color: msg.ok ? theme.green : theme.red }}>{msg.text}</span>}
    </span>
  );
}
