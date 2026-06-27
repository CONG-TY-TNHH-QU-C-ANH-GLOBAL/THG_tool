'use client';

import { useMemo, useState } from 'react';
import { Send } from 'lucide-react';
import type { ReelStrings } from '../../i18n/reelStrings';
import { useWorkspaces } from '../../hooks/useWorkspaces';
import { publishReel } from '../../services/reelsService';

interface PublishReelFormProps {
  reelId: number;
  tr: ReelStrings;
  onResult: (msg: string, ok: boolean) => void;
}

// PublishReelForm collects the account + target at the publish step (when the backend
// actually needs them) and calls POST /reels/:id/publish. A duplicate-target rejection is
// the outbound dedup guard working — shown as guidance, not a hard error.
export default function PublishReelForm({ reelId, tr, onResult }: Readonly<PublishReelFormProps>) {
  const { workspaces } = useWorkspaces();
  const activeAccounts = useMemo(() => workspaces.filter((w) => w.running || w.loggedIn), [workspaces]);
  const [accountId, setAccountId] = useState<number | ''>('');
  const [targetUrl, setTargetUrl] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const canSubmit = accountId !== '' && targetUrl.trim() !== '' && !submitting;

  const submit = async () => {
    if (!canSubmit) return;
    setSubmitting(true);
    try {
      const res = await publishReel(reelId, accountId as number, targetUrl.trim());
      if (res.allowed) {
        onResult(tr.pubQueued, true);
      } else if (res.reason === 'duplicate_outbound_target_race') {
        onResult(tr.pubDuplicate, false);
      } else {
        onResult(res.reason || 'blocked', false);
      }
    } catch (err) {
      onResult(err instanceof Error ? err.message : 'error', false);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div style={{ background: 'var(--surface-alt)', borderRadius: 8, padding: 12, marginBottom: 4, display: 'flex', flexDirection: 'column', gap: 10 }}>
      <div>
        <label style={{ display: 'block', fontSize: 11, color: 'var(--text-faint)', marginBottom: 6 }}>
          {tr.pubAccountLabel} <span style={{ color: 'var(--hot)' }}>*</span>
        </label>
        {activeAccounts.length === 0 ? (
          <div style={{ fontSize: 12, color: 'var(--hot)', background: 'rgba(220,40,40,0.08)', padding: '8px 10px', borderRadius: 8 }}>
            {tr.pubAccountEmpty}
          </div>
        ) : (
          <select className="input" value={accountId} onChange={(e) => setAccountId(e.target.value ? Number(e.target.value) : '')} style={{ width: '100%' }}>
            <option value="" disabled>—</option>
            {activeAccounts.map((w) => (
              <option key={w.accountId} value={w.accountId}>
                {w.accountName}{w.fbUsername ? ` · ${w.fbUsername}` : ''}{w.loggedIn ? '' : ' · đang chạy'}
              </option>
            ))}
          </select>
        )}
      </div>
      <div>
        <label style={{ display: 'block', fontSize: 11, color: 'var(--text-faint)', marginBottom: 6 }}>{tr.pubTargetLabel}</label>
        <input className="input" type="url" value={targetUrl} onChange={(e) => setTargetUrl(e.target.value)} placeholder={tr.pubTargetPlaceholder} style={{ width: '100%' }} />
      </div>
      <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
        <button type="button" className="btn btn-primary btn-sm" disabled={!canSubmit} onClick={() => void submit()}>
          <Send size={12} /> {tr.pubSubmit}
        </button>
      </div>
    </div>
  );
}
