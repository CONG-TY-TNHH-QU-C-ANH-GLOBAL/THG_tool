import React, { useState } from 'react';
import { secondaryBtn } from '../../constants/styles';
import { strings, type Lang } from './telegramCopy';
import { TelegramBindCodeCard } from './TelegramBindCodeCard';
import * as tg from '../../services/telegramIntegrationApi';

// Optional, SECONDARY: lets a user link their personal Telegram DM (for personal notifications /
// /status commands). Not required for channel delivery.
export function PersonalDmConnect({ lang }: { lang: Lang }) {
  const { t } = strings(lang);
  const [code, setCode] = useState<tg.BindCodeResponse | null>(null);
  const [busy, setBusy] = useState(false);

  const generate = async () => {
    setBusy(true);
    try { setCode(await tg.createBindCode()); } finally { setBusy(false); }
  };

  return (
    <div style={{ display: 'grid', gap: 10 }}>
      <button style={secondaryBtn({ padding: '8px 14px', fontSize: 13, opacity: busy ? 0.6 : 1, justifySelf: 'start' })} disabled={busy} onClick={generate}>
        {t('connect_dm')}
      </button>
      {code && <TelegramBindCodeCard lang={lang} code={code} onRegenerate={generate} />}
    </div>
  );
}
