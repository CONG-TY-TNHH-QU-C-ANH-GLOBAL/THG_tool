'use client';

import { useCallback, useEffect, useState } from 'react';
import { AlertTriangle, Plus, RefreshCw } from 'lucide-react';
import { clearActorBlock, getAccountReadiness } from '../../services/accountHealthService';
import { getSystemInfo, type SystemInfo } from '../../services/systemService';
import { getDefaultAccountId, setDefaultAccountId } from '../../services/executionContextService';
import type { AccountReadiness } from './types';
import { overallStatus, severityLabel, type Severity } from './reasonMessages';
import { AccountHealthCard } from './AccountHealthCard';
import { FacebookConnectionWizard } from './FacebookConnectionWizard';
import { NextStepsPanel } from './NextStepsPanel';
import { SafetyCard } from './SafetyCard';

interface Props { orgId: string; isAdmin: boolean; onNavigate?: (tab: string) => void; }

const SUMMARY_ORDER: Severity[] = ['ready', 'warning', 'waiting', 'blocked'];
const SUMMARY_COLOR: Record<Severity, string> = {
  ready: 'var(--ok)', warning: 'var(--warn)', waiting: '#3b82f6', blocked: 'var(--hot)',
};

export default function AccountHealthBoard({ orgId, isAdmin, onNavigate }: Readonly<Props>) {
  const [accounts, setAccounts] = useState<AccountReadiness[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [clearingId, setClearingId] = useState<number | null>(null);
  const [wizardOpen, setWizardOpen] = useState(false);
  const [systemInfo, setSystemInfo] = useState<SystemInfo | null>(null);
  const [defaultAccountId, setDefaultId] = useState(0);
  const [settingDefault, setSettingDefault] = useState(false);
  const [actionMsg, setActionMsg] = useState<{ tone: 'ok' | 'hot'; text: string } | null>(null);

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
  // Poll while no account has materialised yet. A connector can pair and detect a
  // Facebook session seconds later (or via the popup "Sync Now"); the account is
  // bound on the next heartbeat. Without this the board would sit on the empty state
  // until a manual "Làm mới". Silent refetch (no spinner); stops once an account
  // appears, so it never polls when there is already data.
  useEffect(() => {
    if (loading || error || accounts.length > 0) return;
    const id = setInterval(() => { getAccountReadiness().then(setAccounts).catch(() => {}); }, 12_000);
    return () => clearInterval(id);
  }, [loading, error, accounts.length]);
  useEffect(() => { getSystemInfo().then(setSystemInfo).catch(() => setSystemInfo(null)); }, []);
  useEffect(() => { getDefaultAccountId().then(setDefaultId).catch(() => setDefaultId(0)); }, []);

  const handleClear = async (accountId: number) => {
    setClearingId(accountId);
    setActionMsg(null);
    try { await clearActorBlock(accountId); await load(); setActionMsg({ tone: 'ok', text: 'Đã gỡ chặn tài khoản.' }); }
    catch (e) { setActionMsg({ tone: 'hot', text: e instanceof Error ? e.message : 'Không gỡ chặn được — thử lại hoặc liên hệ admin.' }); }
    finally { setClearingId(null); }
  };

  const handleSetDefault = async (accountId: number) => {
    setSettingDefault(true);
    setActionMsg(null);
    try { setDefaultId(await setDefaultAccountId(accountId)); setActionMsg({ tone: 'ok', text: 'Đã đặt làm tài khoản mặc định.' }); }
    catch (e) { setActionMsg({ tone: 'hot', text: e instanceof Error ? e.message : 'Không đặt được tài khoản mặc định — thử lại.' }); }
    finally { setSettingDefault(false); }
  };

  // P1.3E: "Sẵn sàng" counts only EXECUTABLE accounts (requester-owned live connector), never
  // "no blocking reasons". Non-executable accounts fall into their health/connector bucket; an
  // account with no per-capability health reason but executable=false (e.g. not_controllable /
  // connector/control issue) is counted as blocked, never as ready.
  const counts: Record<Severity, number> = { ready: 0, warning: 0, blocked: 0, waiting: 0 };
  for (const a of accounts) {
    if (a.executable) { counts.ready += 1; continue; }
    let sev = overallStatus(Array.from(new Set(a.capabilities.flatMap(c => c.reasons ?? [])))).severity;
    if (sev === 'ready') sev = 'blocked';
    counts[sev] += 1;
  }

  return (
    <div style={{ padding: 'var(--s-4)', display: 'flex', flexDirection: 'column', gap: 'var(--s-4)' }}>
      <header style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16, flexWrap: 'wrap' }}>
        <div>
          <div className="eyebrow"><span className="dot" />FACEBOOK BÁN HÀNG</div>
          <h2 style={{ fontSize: 28, marginTop: 8 }}>Kết nối Facebook bán hàng</h2>
          <p style={{ color: 'var(--text-mute)', fontSize: 13.5, marginTop: 6, maxWidth: 640 }}>
            Kết nối Facebook của nhân viên để agent có thể tìm lead, bình luận, inbox và đăng bài theo quyền được cấp. THG không lưu mật khẩu Facebook.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => void load()} disabled={loading}>
            <RefreshCw size={13} className={loading ? 'spin' : ''} /> Làm mới
          </button>
          <button type="button" className="btn btn-primary btn-sm" onClick={() => setWizardOpen(true)}>
            <Plus size={14} /> Kết nối Facebook mới
          </button>
        </div>
      </header>

      {wizardOpen && (
        <FacebookConnectionWizard systemInfo={systemInfo} onClose={() => setWizardOpen(false)} onConnected={() => void load()} />
      )}

      {error && (
        <div className="banner banner-hot" style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <AlertTriangle size={16} color="var(--hot)" />
          <span style={{ fontSize: 13 }}>{error}</span>
        </div>
      )}
      {actionMsg && (
        <div className={`banner banner-${actionMsg.tone === 'ok' ? 'ok' : 'hot'}`} style={{ display: 'flex', gap: 8, alignItems: 'center', justifyContent: 'space-between' }}>
          <span style={{ fontSize: 13 }}>{actionMsg.text}</span>
          <button type="button" style={{ background: 'transparent', border: 0, cursor: 'pointer', color: 'var(--text-mute)', fontSize: 12 }} onClick={() => setActionMsg(null)}>Đóng</button>
        </div>
      )}

      <div style={{ display: 'flex', gap: 'var(--s-4)', alignItems: 'flex-start', flexWrap: 'wrap' }}>
        <div style={{ flex: '2 1 440px', minWidth: 0, display: 'flex', flexDirection: 'column', gap: 'var(--s-3)' }}>
          <div className="card" style={{ display: 'flex', gap: 'var(--s-5)', padding: 'var(--s-3) var(--s-5)', flexWrap: 'wrap' }}>
            {SUMMARY_ORDER.map(sev => (
              <span key={sev} style={{ display: 'inline-flex', alignItems: 'center', gap: 7, fontSize: 12.5, color: 'var(--text-mute)' }}>
                <span style={{ width: 9, height: 9, borderRadius: '50%', background: SUMMARY_COLOR[sev] }} />
                {severityLabel(sev)}: <strong className="tabular" style={{ color: 'var(--text)' }}>{counts[sev]}</strong>
              </span>
            ))}
          </div>
          {loading && accounts.length === 0 ? (
            <div style={{ color: 'var(--text-mute)', fontSize: 13, padding: 'var(--s-4)' }}>Đang tải trạng thái tài khoản…</div>
          ) : accounts.length === 0 ? (
            <div className="card" style={{ padding: 'var(--s-6, var(--s-5))', textAlign: 'center', display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 12 }}>
              <Plus size={28} color="var(--text-faint)" />
              <div>
                <div style={{ fontSize: 16, fontWeight: 600 }}>Chưa có Facebook nào được kết nối</div>
                <p style={{ fontSize: 13, color: 'var(--text-mute)', marginTop: 6, maxWidth: 420 }}>
                  Kết nối Facebook đầu tiên để agent có thể tìm lead, bình luận, inbox và đăng bài theo quyền được cấp.
                </p>
              </div>
              <button type="button" className="btn btn-primary btn-sm" onClick={() => setWizardOpen(true)}>
                <Plus size={14} /> Kết nối Facebook mới
              </button>
            </div>
          ) : (
            accounts.map(a => (
              <AccountHealthCard key={a.account_id} account={a} isAdmin={isAdmin} multiAccount={accounts.length > 1}
                onClearBlock={accId => void handleClear(accId)} clearing={clearingId === a.account_id} />
            ))
          )}
        </div>

        <div style={{ flex: '1 1 280px', minWidth: 260, display: 'flex', flexDirection: 'column', gap: 'var(--s-3)' }}>
          <NextStepsPanel accounts={accounts} onNavigate={onNavigate} defaultAccountId={defaultAccountId}
            onSetDefault={accId => void handleSetDefault(accId)} settingDefault={settingDefault} />
          <SafetyCard />
        </div>
      </div>
    </div>
  );
}
