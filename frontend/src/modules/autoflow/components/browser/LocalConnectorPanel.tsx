import { useEffect, useState } from 'react';
import { CheckCircle, Copy, ExternalLink, Eye, EyeOff, KeyRound, Monitor, Puzzle, Radio, RefreshCw, Shield, Unplug } from 'lucide-react';
import { theme } from '../../constants/styles';
import type { SystemInfo } from '../../services/systemService';
import type { LocalConnector } from '../../types';
import { connectorStatusLabel, facebookIdentityLabel, formatCountdown, formatLastSeen, isDashboardStreamConnector } from './browserHelpers';

export function LocalConnectorPanel({
  connectors,
  creating,
  pairingCode,
  pairingExpiresAt,
  systemInfo,
  currentUserId,
  currentUserRole,
  disconnectingId,
  onCreate,
  onDisconnect,
}: {
  connectors: LocalConnector[];
  creating: boolean;
  pairingCode: string;
  pairingExpiresAt: string;
  systemInfo: SystemInfo | null;
  currentUserId: number;
  currentUserRole: string;
  disconnectingId: number | null;
  onCreate: () => void;
  onDisconnect: (connector: LocalConnector) => void;
}) {
  const online = connectors.filter(c => c.online).length;
  const extensionOnline = connectors.filter(c => c.online && isDashboardStreamConnector(c)).length;
  const [setupOpen, setSetupOpen] = useState(connectors.length === 0);
  const [pairingCodeVisible, setPairingCodeVisible] = useState(false);
  const [pairingRemainingMs, setPairingRemainingMs] = useState<number | null>(null);
  const [dashboardServer, setDashboardServer] = useState('');
  const pairingExpired = pairingCode !== '' && pairingRemainingMs !== null && pairingRemainingMs <= 0;
  const extensionStoreUrl = (systemInfo?.chrome_extension_store_url || '').trim();
  const extensionInstallReady = extensionStoreUrl.length > 0;

  useEffect(() => {
    setPairingCodeVisible(false);
  }, [pairingCode]);

  useEffect(() => {
    if (!pairingExpiresAt) {
      setPairingRemainingMs(null);
      return;
    }
    const expiresAt = new Date(pairingExpiresAt).getTime();
    if (Number.isNaN(expiresAt)) {
      setPairingRemainingMs(null);
      return;
    }
    const tick = () => setPairingRemainingMs(Math.max(0, expiresAt - Date.now()));
    tick();
    const timer = window.setInterval(tick, 1000);
    return () => window.clearInterval(timer);
  }, [pairingExpiresAt]);

  useEffect(() => {
    if (typeof window !== 'undefined') {
      setDashboardServer(window.location.origin);
    }
  }, []);

  return (
    <div className="af-connector-panel" style={{ background: theme.surface, border: `1px solid ${online ? '#10b98166' : '#334155'}`, borderRadius: 10, overflow: 'hidden' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '11px 14px', borderBottom: `1px solid ${theme.border}`, flexWrap: 'wrap' }}>
        <div style={{ width: 34, height: 34, borderRadius: 9, background: '#0f766e22', border: '1px solid #2dd4bf55', display: 'grid', placeItems: 'center' }}>
          <Puzzle size={17} color="#5eead4" />
        </div>
        <div style={{ minWidth: 240, flex: '1 1 360px' }}>
          <p style={{ color: theme.text, fontSize: 13, fontWeight: 800 }}>THG Chrome Extension: Facebook thật, dashboard quan sát tập trung</p>
          <p style={{ color: theme.textMuted, fontSize: 11 }}>Extension chạy trong Chrome đã đăng nhập Facebook của nhân viên, stream hình ảnh và action log về Browser dashboard.</p>
        </div>
        <span style={{ color: extensionOnline ? '#4ade80' : theme.textMuted, fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 5 }}>
          <Radio size={12} /> {extensionOnline}/{connectors.length} extension ready
        </span>
        <button onClick={() => setSetupOpen(v => !v)} style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '7px 12px', borderRadius: 8, border: '1px solid #2dd4bf66', background: '#0f766e33', color: '#ccfbf1', cursor: 'pointer', fontSize: 12, fontWeight: 700 }}>
          <Monitor size={13} />
          Hướng dẫn kết nối
        </button>
      </div>

      {setupOpen && (
        <div style={{ padding: 12, borderBottom: `1px solid ${theme.border}`, background: '#07131f' }}>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(230px, 1fr))', gap: 10 }}>
            <div style={{ border: `1px solid ${theme.border}`, borderRadius: 8, padding: 11, background: theme.surface }}>
              <p style={{ color: '#93c5fd', fontSize: 11, fontWeight: 800, marginBottom: 7 }}>1. Cài THG Chrome Extension</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 54 }}>
                Cài extension vào Chrome cá nhân đang dùng Facebook. THG không nhận mật khẩu Facebook của bạn.
              </p>
              {extensionInstallReady ? (
                <a
                  href={extensionStoreUrl}
                  target="_blank"
                  rel="noreferrer"
                  style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '7px 10px', borderRadius: 7, border: '1px solid #14b8a666', background: '#0f766e33', color: '#ccfbf1', textDecoration: 'none', fontSize: 12, fontWeight: 700 }}
                >
                  <ExternalLink size={13} /> Cài từ Chrome Web Store
                </a>
              ) : (
                <button
                  type="button"
                  disabled
                  style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '7px 10px', borderRadius: 7, border: `1px solid ${theme.border}`, background: theme.surfaceAlt, color: theme.textFaint, fontSize: 12, fontWeight: 700, cursor: 'not-allowed' }}
                  title="Cấu hình CHROME_EXTENSION_STORE_URL trên production để bật nút cài đặt."
                >
                  <ExternalLink size={13} /> Chưa cấu hình Web Store
                </button>
              )}
              <p style={{ color: theme.textFaint, fontSize: 10, marginTop: 7, lineHeight: 1.45 }}>
                Chrome Web Store sẽ cài và tự cập nhật extension cho Chrome của bạn.
              </p>
            </div>

            <div style={{ border: `1px solid ${pairingCode ? '#22c55e55' : theme.border}`, borderRadius: 8, padding: 11, background: theme.surface }}>
              <p style={{ color: '#bbf7d0', fontSize: 11, fontWeight: 800, marginBottom: 7 }}>2. Ghép Chrome với workspace</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 54 }}>
                Mỗi nhân viên tự tạo mã riêng. Mã chỉ dùng một lần, hết hạn sau 10 phút và không dùng chung cho cả workspace.
              </p>
              {dashboardServer && (
                <div style={{ display: 'flex', gap: 7, alignItems: 'center', marginBottom: 9 }}>
                  <code style={{ color: '#bae6fd', fontSize: 11, flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{dashboardServer}</code>
                  <button
                    type="button"
                    onClick={() => navigator.clipboard?.writeText(dashboardServer)}
                    style={{ display: 'inline-flex', alignItems: 'center', gap: 5, padding: '5px 8px', borderRadius: 7, border: '1px solid #38bdf866', background: '#07598533', color: '#e0f2fe', cursor: 'pointer', fontSize: 10, fontWeight: 700 }}
                    title="Copy THG server"
                  >
                    <Copy size={11} /> Copy server
                  </button>
                </div>
              )}
              {pairingCode ? (
                <div style={{ display: 'flex', gap: 7, alignItems: 'center', flexWrap: 'wrap' }}>
                  <code style={{ color: '#dcfce7', fontSize: 18, fontWeight: 900, flex: '1 1 130px', letterSpacing: pairingCodeVisible ? 0 : 2 }}>
                    {pairingCodeVisible ? pairingCode : '••••-••••'}
                  </code>
                  <button
                    type="button"
                    onClick={() => setPairingCodeVisible(v => !v)}
                    disabled={pairingExpired}
                    style={{ display: 'inline-flex', alignItems: 'center', gap: 5, padding: '6px 9px', borderRadius: 7, border: `1px solid ${pairingCodeVisible ? '#f59e0b66' : theme.border}`, background: pairingCodeVisible ? '#78350f33' : theme.surfaceAlt, color: pairingExpired ? theme.textFaint : (pairingCodeVisible ? '#fcd34d' : theme.textMuted), cursor: pairingExpired ? 'not-allowed' : 'pointer', opacity: pairingExpired ? 0.6 : 1, fontSize: 11, fontWeight: 700 }}
                  >
                    {pairingCodeVisible ? <EyeOff size={12} /> : <Eye size={12} />}
                    {pairingCodeVisible ? 'Ẩn mã' : 'Hiện mã'}
                  </button>
                  <button
                    type="button"
                    disabled={!pairingCodeVisible || pairingExpired}
                    onClick={() => {
                      if (pairingCodeVisible && !pairingExpired) navigator.clipboard?.writeText(pairingCode);
                    }}
                    style={{ display: 'inline-flex', alignItems: 'center', gap: 5, padding: '6px 9px', borderRadius: 7, border: `1px solid ${theme.border}`, background: theme.surfaceAlt, color: pairingCodeVisible && !pairingExpired ? theme.textMuted : theme.textFaint, cursor: pairingCodeVisible && !pairingExpired ? 'pointer' : 'not-allowed', opacity: pairingCodeVisible && !pairingExpired ? 1 : 0.55, fontSize: 11 }}
                    title={pairingExpired ? 'Mã đã hết hạn, hãy tạo mã mới' : (pairingCodeVisible ? 'Copy mã kết nối' : 'Hiện mã trước khi copy')}
                  >
                    <Copy size={12} /> Copy
                  </button>
                  <button
                    type="button"
                    onClick={onCreate}
                    disabled={creating}
                    style={{ display: 'inline-flex', alignItems: 'center', gap: 5, padding: '6px 9px', borderRadius: 7, border: '1px solid #22c55e66', background: '#16653433', color: '#dcfce7', cursor: creating ? 'wait' : 'pointer', opacity: creating ? 0.65 : 1, fontSize: 11, fontWeight: 700 }}
                  >
                    {creating ? <RefreshCw size={12} className="spin" /> : <KeyRound size={12} />}
                    Tạo mã mới
                  </button>
                  <span style={{ display: 'inline-flex', alignItems: 'center', gap: 5, padding: '5px 8px', borderRadius: 999, border: `1px solid ${pairingExpired ? '#ef444466' : '#22c55e66'}`, background: pairingExpired ? '#7f1d1d33' : '#064e3b33', color: pairingExpired ? '#fecaca' : '#bbf7d0', fontSize: 11, fontWeight: 700 }}>
                    {pairingExpired ? 'Đã hết hạn' : `Còn ${formatCountdown(pairingRemainingMs ?? 0)}`}
                  </span>
                </div>
              ) : (
                <button onClick={onCreate} disabled={creating} style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '7px 10px', borderRadius: 7, border: '1px solid #22c55e66', background: '#16653433', color: '#dcfce7', cursor: creating ? 'wait' : 'pointer', opacity: creating ? 0.65 : 1, fontSize: 12, fontWeight: 700 }}>
                  {creating ? <RefreshCw size={13} className="spin" /> : <KeyRound size={13} />}
                  Tạo mã kết nối
                </button>
              )}
              {pairingExpiresAt && <p style={{ color: theme.textFaint, fontSize: 10, marginTop: 7 }}>Hết hạn: {formatLastSeen(pairingExpiresAt)}</p>}
            </div>

            <div style={{ border: `1px solid ${theme.border}`, borderRadius: 8, padding: 11, background: theme.surface }}>
              <p style={{ color: '#fcd34d', fontSize: 11, fontWeight: 800, marginBottom: 7 }}>3. Mở tab Facebook đã đăng nhập</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 54 }}>
                Mở facebook.com trong Chrome đã cài extension. Khi bấm Bắt đầu trên account, extension sẽ stream tab Facebook về Browser dashboard.
              </p>
              <span style={{ color: '#fef3c7', fontSize: 11, display: 'inline-flex', gap: 5, alignItems: 'center' }}><Shield size={12} /> Chrome vẫn là thiết bị/IP thật của nhân viên</span>
            </div>

            <div style={{ border: `1px solid ${extensionOnline ? '#22c55e66' : theme.border}`, borderRadius: 8, padding: 11, background: theme.surface }}>
              <p style={{ color: extensionOnline ? '#86efac' : theme.textMuted, fontSize: 11, fontWeight: 800, marginBottom: 7 }}>4. Dashboard nhận stream thật</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 54 }}>
                Browser tab của THG hiển thị hình ảnh/action log từ tab Facebook thật để user quan sát crawl, comment, inbox và posting tập trung.
              </p>
              <span style={{ color: extensionOnline ? '#4ade80' : theme.textFaint, fontSize: 12, display: 'inline-flex', gap: 5, alignItems: 'center' }}>
                {extensionOnline ? <CheckCircle size={13} /> : <Radio size={13} />} {extensionOnline ? 'Extension đã sẵn sàng stream' : online ? 'Đã có thiết bị online, đang chờ Facebook tab' : 'Đang chờ Chrome kết nối'}
              </span>
            </div>
          </div>
        </div>
      )}

      {connectors.length === 0 ? (
        <p style={{ color: theme.textMuted, fontSize: 12, padding: '12px 14px' }}>
          Chưa có Chrome nào kết nối. Cài extension từ Chrome Web Store, tạo mã kết nối, dán mã vào popup extension, rồi mở tab Facebook đã đăng nhập.
        </p>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))', gap: 10, padding: 12 }}>
          {connectors.map(c => {
            const identityLabel = facebookIdentityLabel({
              displayName: c.fbDisplayName,
              username: c.fbUsername,
              fbUserId: c.fbUserId,
            });
            const canDisconnect = c.createdBy === currentUserId || currentUserRole === 'admin' || currentUserRole === 'founder' || currentUserRole === 'superadmin';
            return (
              <div key={c.id} style={{ border: `1px solid ${c.online ? '#22c55e55' : theme.border}`, borderRadius: 8, background: theme.surfaceAlt, padding: 11 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 7 }}>
                  <span style={{ width: 8, height: 8, borderRadius: '50%', background: c.online ? '#4ade80' : theme.textFaint }} />
                  <p style={{ color: theme.text, fontSize: 13, fontWeight: 700, flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.name}</p>
                  <span style={{ color: c.online ? '#4ade80' : theme.textFaint, fontSize: 10 }}>{c.online ? 'online' : 'offline'}</span>
                </div>
                <p style={{ color: theme.textMuted, fontSize: 11 }}>{c.hostname || 'Chrome Extension'} · {c.os || 'Chrome'} · {c.version || 'no version'}</p>
                <p style={{ color: theme.textFaint, fontSize: 11, marginTop: 5 }}>Lần cuối {formatLastSeen(c.lastSeen)} · {connectorStatusLabel(c.streamStatus)}</p>
                <p style={{ color: c.createdBy === currentUserId ? '#86efac' : theme.textFaint, fontSize: 11, marginTop: 5 }}>
                  {c.createdBy === currentUserId ? 'Chrome của bạn' : `Chrome của thành viên #${c.createdBy}`}
                  {c.assignedAccountId ? ` · gắn account #${c.assignedAccountId}` : ''}
                </p>
                {c.currentUrl && <p style={{ color: '#93c5fd', fontSize: 11, marginTop: 5, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.currentUrl}</p>}
                {c.streamStatus === 'facebook_logged_in' && identityLabel && <p style={{ color: '#c4b5fd', fontSize: 11, marginTop: 5 }}>{identityLabel}</p>}
                {c.chromeError && <p style={{ color: '#fca5a5', fontSize: 11, marginTop: 5, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.chromeError}</p>}
                {canDisconnect && (
                  <button
                    type="button"
                    onClick={() => onDisconnect(c)}
                    disabled={disconnectingId === c.id}
                    style={{ display: 'inline-flex', alignItems: 'center', gap: 6, marginTop: 10, padding: '6px 9px', borderRadius: 7, border: '1px solid #ef444455', background: '#7f1d1d33', color: '#fecaca', cursor: disconnectingId === c.id ? 'wait' : 'pointer', opacity: disconnectingId === c.id ? 0.65 : 1, fontSize: 11, fontWeight: 700 }}
                  >
                    {disconnectingId === c.id ? <RefreshCw size={12} className="spin" /> : <Unplug size={12} />}
                    {disconnectingId === c.id ? 'Đang ngắt' : 'Disconnect Chrome này'}
                  </button>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
