import React, { useState } from 'react';
import { Hash, Lock, ArrowLeft } from 'lucide-react';
import { theme, cardStyle, secondaryBtn, alpha } from '../../constants/styles';
import { strings, type Lang } from './telegramCopy';
import { PublicChannelConnectCard } from './PublicChannelConnectCard';
import { PrivateChannelConnectCard } from './PrivateChannelConnectCard';

type Mode = 'choose' | 'public' | 'private';

function TypeCard({ icon, label, onClick }: { icon: React.ReactNode; label: string; onClick: () => void }) {
  return (
    <button onClick={onClick} style={{
      display: 'flex', gap: 10, alignItems: 'center', textAlign: 'left', cursor: 'pointer',
      border: `1px solid ${theme.border}`, background: 'var(--bg-elev-2)', borderRadius: 'var(--radius-md)',
      padding: '14px 16px', color: theme.text, fontSize: 13.5, fontWeight: 600, flex: 1, minWidth: 200,
    }}>
      <span style={{ width: 34, height: 34, borderRadius: 9, display: 'grid', placeItems: 'center', background: alpha(theme.primary, 14), color: theme.primary }}>{icon}</span>
      {label}
    </button>
  );
}

// Guided "Connect a Telegram channel" wizard: choose type → public (@username) or private (code).
export function TelegramChannelSetupWizard({ lang, onConnected }: { lang: Lang; onConnected: () => void }) {
  const { t } = strings(lang);
  const [mode, setMode] = useState<Mode>('choose');

  return (
    <div style={cardStyle({ display: 'grid', gap: 14 })}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        {mode !== 'choose' && (
          <button aria-label="back" onClick={() => setMode('choose')} style={{ background: 'transparent', border: 'none', cursor: 'pointer', color: theme.textMuted, padding: 0 }}>
            <ArrowLeft size={16} />
          </button>
        )}
        <p style={{ color: theme.text, fontSize: 14, fontWeight: 650, margin: 0 }}>{t('wiz_title')}</p>
      </div>

      {mode === 'choose' && (
        <div>
          <p style={{ color: theme.textFaint, fontSize: 12, margin: '0 0 8px' }}>{t('wiz_choose')}</p>
          <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
            <TypeCard icon={<Hash size={18} />} label={t('type_public')} onClick={() => setMode('public')} />
            <TypeCard icon={<Lock size={18} />} label={t('type_private')} onClick={() => setMode('private')} />
          </div>
        </div>
      )}
      {mode === 'public' && <PublicChannelConnectCard lang={lang} onConnected={onConnected} />}
      {mode === 'private' && <PrivateChannelConnectCard lang={lang} onCheck={onConnected} />}
    </div>
  );
}
