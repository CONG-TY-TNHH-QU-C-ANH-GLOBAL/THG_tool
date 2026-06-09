'use client';

import { useCallback, useEffect, useState } from 'react';
import { AlertTriangle, RefreshCw } from 'lucide-react';
import { clearActorBlock, getAccountReadiness } from '../../services/accountHealthService';
import type { AccountReadiness } from './types';
import { overallStatus, severityLabel, type Severity } from './reasonMessages';
import { AccountHealthCard } from './AccountHealthCard';

interface Props { orgId: string; isAdmin: boolean; }

const SUMMARY_ORDER: Severity[] = ['ready', 'warning', 'waiting', 'blocked'];
const SUMMARY_COLOR: Record<Severity, string> = {
  ready: 'var(--ok)', warning: 'var(--warn)', waiting: '#3b82f6', blocked: 'var(--hot)',
};

export default function AccountHealthBoard({ orgId, isAdmin }: Props) {
  const [accounts, setAccounts] = useState<AccountReadiness[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [clearingId, setClearingId] = useState<number | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setAccounts(await getAccountReadiness());
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Không tải được trạng thái tài khoản');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void load(); }, [load, orgId]);

  const handleClear = async (accountId: number) => {
    setClearingId(accountId);
    try {
      await clearActorBlock(accountId);
      await load();
    } catch {
      /* surfaced on next load */
    } finally {
      setClearingId(null);
    }
  };

  const counts: Record<Severity, number> = { ready: 0, warning: 0, blocked: 0, waiting: 0 };
  for (const a of accounts) {
    const reasons = Array.from(new Set(a.capabilities.flatMap(c => c.reasons)));
    counts[overallStatus(reasons).severity] += 1;
  }

  return (
    <div style={{ padding: 'var(--s-4)', display: 'flex', flexDirection: 'column', gap: 'var(--s-4)' }}>
      <header style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16, flexWrap: 'wrap' }}>
        <div>
          <div className="eyebrow"><span className="dot" />SỨC KHOẺ TÀI KHOẢN</div>
          <h2 style={{ fontSize: 28, marginTop: 8 }}>Trạng thái tài khoản</h2>
          <p style={{ color: 'var(--text-mute)', fontSize: 13.5, marginTop: 6 }}>
            Mỗi tài khoản bán hàng đang sẵn sàng hay gặp vấn đề gì — và bạn cần làm gì tiếp theo.
          </p>
        </div>
        <button type="button" className="btn btn-ghost btn-sm" onClick={() => void load()} disabled={loading}>
          <RefreshCw size={13} className={loading ? 'spin' : ''} /> Làm mới
        </button>
      </header>

      <div className="card" style={{ display: 'flex', gap: 'var(--s-5)', padding: 'var(--s-3) var(--s-5)', flexWrap: 'wrap' }}>
        {SUMMARY_ORDER.map(sev => (
          <span key={sev} style={{ display: 'inline-flex', alignItems: 'center', gap: 7, fontSize: 12.5, color: 'var(--text-mute)' }}>
            <span style={{ width: 9, height: 9, borderRadius: '50%', background: SUMMARY_COLOR[sev] }} />
            {severityLabel(sev)}: <strong className="tabular" style={{ color: 'var(--text)' }}>{counts[sev]}</strong>
          </span>
        ))}
      </div>

      {error && (
        <div className="banner banner-hot" style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <AlertTriangle size={16} color="var(--hot)" />
          <span style={{ fontSize: 13 }}>{error}</span>
        </div>
      )}

      {loading && accounts.length === 0 ? (
        <div style={{ color: 'var(--text-mute)', fontSize: 13, padding: 'var(--s-4)' }}>Đang tải trạng thái tài khoản…</div>
      ) : accounts.length === 0 ? (
        <div className="card" style={{ padding: 'var(--s-5)', textAlign: 'center', color: 'var(--text-mute)', fontSize: 13.5 }}>
          Chưa có tài khoản nào. Thêm tài khoản Facebook và pair Chrome Extension để bắt đầu.
        </div>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(340px, 1fr))', gap: 'var(--s-3)' }}>
          {accounts.map(a => (
            <AccountHealthCard
              key={a.account_id}
              account={a}
              isAdmin={isAdmin}
              onClearBlock={accId => void handleClear(accId)}
              clearing={clearingId === a.account_id}
            />
          ))}
        </div>
      )}
    </div>
  );
}
