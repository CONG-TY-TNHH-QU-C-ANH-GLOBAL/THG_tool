import { useEffect, useState } from 'react';
import { CheckCircle, Copy, Eye, EyeOff, KeyRound, Laptop, Radio, RefreshCw, Shield, Unplug } from 'lucide-react';
import { theme } from '../../constants/styles';
import type { SystemInfo } from '../../services/systemService';
import type { LocalConnector } from '../../types';
import { connectorStatusLabel, facebookIdentityLabel, formatCountdown, formatLastSeen, isDashboardStreamConnector, RUNTIME_DOWNLOADS } from './browserHelpers';

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
  const runtimeOnline = connectors.filter(c => c.online && isDashboardStreamConnector(c)).length;
  const [setupOpen, setSetupOpen] = useState(connectors.length === 0);
  const [pairingCodeVisible, setPairingCodeVisible] = useState(false);
  const [pairingRemainingMs, setPairingRemainingMs] = useState<number | null>(null);
  const [dashboardServer, setDashboardServer] = useState('');
  const pairingExpired = pairingCode !== '' && pairingRemainingMs !== null && pairingRemainingMs <= 0;

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
    <div className="af-runtime-panel" style={{ background: theme.surface, border: `1px solid ${online ? '#10b98166' : '#334155'}`, borderRadius: 10, overflow: 'hidden' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '11px 14px', borderBottom: `1px solid ${theme.border}` }}>
        <div style={{ width: 34, height: 34, borderRadius: 9, background: '#0f766e22', border: '1px solid #2dd4bf55', display: 'grid', placeItems: 'center' }}>
          <Laptop size={17} color="#5eead4" />
        </div>
        <div style={{ minWidth: 0 }}>
          <p style={{ color: theme.text, fontSize: 13, fontWeight: 800 }}>Chrome local đăng nhập trước, dashboard quan sát sau</p>
          <p style={{ color: theme.textMuted, fontSize: 11 }}>Flow production chính: user đăng nhập Facebook trong Chrome local trên device/IP thật, THG tự đồng bộ trạng thái rồi stream automation về Browser.</p>
        </div>
        <span style={{ marginLeft: 'auto', color: online ? '#4ade80' : theme.textMuted, fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 5 }}>
          <Radio size={12} /> {runtimeOnline}/{connectors.length} runtime ready
        </span>
        <button onClick={() => setSetupOpen(v => !v)} style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '7px 12px', borderRadius: 8, border: '1px solid #2dd4bf66', background: '#0f766e33', color: '#ccfbf1', cursor: 'pointer', fontSize: 12, fontWeight: 700 }}>
          <Laptop size={13} />
          Hướng dẫn kết nối
        </button>
      </div>

      {setupOpen && (
        <div style={{ padding: 12, borderBottom: `1px solid ${theme.border}`, background: '#07131f' }}>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(210px, 1fr))', gap: 10 }}>
            <div style={{ border: `1px solid ${theme.border}`, borderRadius: 8, padding: 11, background: theme.surface }}>
              <p style={{ color: '#93c5fd', fontSize: 11, fontWeight: 800, marginBottom: 7 }}>1. Cài THG Local Kit</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 50 }}>
                Một gói theo hệ điều hành, bên trong có THG Local Runtime để mở Chrome local, giữ session Facebook trên máy người dùng và stream về dashboard.
              </p>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 7 }}>
                {RUNTIME_DOWNLOADS.map(item => (
                  <a
                    key={item.key}
                    href={item.href}
                    style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '7px 10px', borderRadius: 7, border: '1px solid #14b8a666', background: '#0f766e33', color: '#ccfbf1', textDecoration: 'none', fontSize: 12, fontWeight: 700, opacity: systemInfo?.agent_builds?.[item.key] === false ? 0.5 : 1 }}
                  >
                    <Laptop size={13} /> Tải Kit {item.label}
                  </a>
                ))}
              </div>
              <p style={{ color: theme.textFaint, fontSize: 10, marginTop: 7, lineHeight: 1.45 }}>
                Sau khi giải nén, chạy file Start trong kit. Windows dùng Start-THG-Local-Runtime.cmd để cửa sổ luôn mở cho bạn nhập mã kết nối và xem trạng thái.
              </p>
            </div>

            <div style={{ border: `1px solid ${pairingCode ? '#22c55e55' : theme.border}`, borderRadius: 8, padding: 11, background: theme.surface }}>
              <p style={{ color: '#bbf7d0', fontSize: 11, fontWeight: 800, marginBottom: 7 }}>2. Ghép thiết bị với workspace</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 50 }}>
                Mã này chỉ dành cho thiết bị của bạn. Mỗi nhân viên đăng nhập THG và tự tạo mã riêng; không dùng chung mã trong workspace.
              </p>
              {dashboardServer && (
                <div style={{ display: 'flex', gap: 7, alignItems: 'center', marginBottom: 9 }}>
                  <code style={{ color: '#bae6fd', fontSize: 11, flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{dashboardServer}</code>
                  <button
                    type="button"
                    onClick={() => navigator.clipboard?.writeText(dashboardServer)}
                    style={{ display: 'inline-flex', alignItems: 'center', gap: 5, padding: '5px 8px', borderRadius: 7, border: '1px solid #38bdf866', background: '#07598533', color: '#e0f2fe', cursor: 'pointer', fontSize: 10, fontWeight: 700 }}
                    title="Copy THG server để dán vào Runtime"
                  >
                    <Copy size={11} /> Copy server
                  </button>
                </div>
              )}
              <p style={{ color: '#fef3c7', fontSize: 10, lineHeight: 1.45, marginBottom: 8 }}>
                THG server trong Runtime phải trùng domain dashboard đang tạo mã. Mã chỉ dùng một lần, gắn với user tạo mã và hết hạn sau 10 phút.
              </p>
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
              <p style={{ color: '#fcd34d', fontSize: 11, fontWeight: 800, marginBottom: 7 }}>3. Đăng nhập trên Chrome local</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 50 }}>
                Bấm Mở Chrome local trên account. Runtime sẽ mở cửa sổ Chrome trên máy nhân viên; hãy đăng nhập Facebook, 2FA hoặc checkpoint trực tiếp trong cửa sổ đó.
              </p>
              <span style={{ color: '#fef3c7', fontSize: 11, display: 'inline-flex', gap: 5, alignItems: 'center' }}><Shield size={12} /> Không nhập mật khẩu Facebook vào THG</span>
            </div>

            <div style={{ border: `1px solid ${runtimeOnline ? '#22c55e66' : theme.border}`, borderRadius: 8, padding: 11, background: theme.surface }}>
              <p style={{ color: runtimeOnline ? '#86efac' : theme.textMuted, fontSize: 11, fontWeight: 800, marginBottom: 7 }}>4. Dashboard tự nhận session</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 50 }}>
                Khi Chrome đã vào Facebook, Runtime tự đưa cửa sổ local ra nền, dashboard lưu trạng thái/Facebook ID và Browser tab trở thành nơi quan sát automation tập trung.
              </p>
              <span style={{ color: runtimeOnline ? '#4ade80' : theme.textFaint, fontSize: 12, display: 'inline-flex', gap: 5, alignItems: 'center' }}>
                {runtimeOnline ? <CheckCircle size={13} /> : <Radio size={13} />} {runtimeOnline ? 'Runtime đã sẵn sàng đồng bộ' : online ? 'Đã có thiết bị online, đang chờ Chrome local' : 'Đang chờ máy kết nối'}
              </span>
            </div>
          </div>
        </div>
      )}

      {connectors.length === 0 ? (
        <p style={{ color: theme.textMuted, fontSize: 12, padding: '12px 14px' }}>
          Chưa có thiết bị nào kết nối. Cài THG Local Runtime, tạo mã kết nối, rồi nhập mã trong app để mở Chrome local và đồng bộ session Facebook.
        </p>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))', gap: 10, padding: 12 }}>
          {connectors.map(c => {
            const identityLabel = facebookIdentityLabel({
              displayName: c.fbDisplayName,
              username: c.fbUsername,
              fbUserId: c.fbUserId,
            });
            return (
              <div key={c.id} style={{ border: `1px solid ${c.online ? '#22c55e55' : theme.border}`, borderRadius: 8, background: theme.surfaceAlt, padding: 11 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 7 }}>
                  <span style={{ width: 8, height: 8, borderRadius: '50%', background: c.online ? '#4ade80' : theme.textFaint }} />
                  <p style={{ color: theme.text, fontSize: 13, fontWeight: 700, flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.name}</p>
                  <span style={{ color: c.online ? '#4ade80' : theme.textFaint, fontSize: 10 }}>{c.online ? 'online' : 'offline'}</span>
                </div>
                <p style={{ color: theme.textMuted, fontSize: 11 }}>{c.hostname || 'unknown host'} · {c.os || 'unknown os'} · {c.version || 'no version'}</p>
                <p style={{ color: theme.textFaint, fontSize: 11, marginTop: 5 }}>Lần cuối {formatLastSeen(c.lastSeen)} · {connectorStatusLabel(c.streamStatus)}</p>
                <p style={{ color: c.createdBy === currentUserId ? '#86efac' : theme.textFaint, fontSize: 11, marginTop: 5 }}>
                  {c.createdBy === currentUserId ? 'Thiết bị của bạn' : `Thiết bị thành viên #${c.createdBy}`}
                  {c.assignedAccountId ? ` · gắn account #${c.assignedAccountId}` : ''}
                </p>
                {c.currentUrl && <p style={{ color: '#93c5fd', fontSize: 11, marginTop: 5, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.currentUrl}</p>}
                {c.streamStatus === 'facebook_logged_in' && identityLabel && <p style={{ color: '#c4b5fd', fontSize: 11, marginTop: 5 }}>{identityLabel}</p>}
                {c.chromeError && <p style={{ color: '#fca5a5', fontSize: 11, marginTop: 5, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.chromeError}</p>}
                {(c.createdBy === currentUserId || currentUserRole === 'admin' || currentUserRole === 'founder' || currentUserRole === 'superadmin') && (
                  <button
                    type="button"
                    onClick={() => onDisconnect(c)}
                    disabled={disconnectingId === c.id}
                    style={{ display: 'inline-flex', alignItems: 'center', gap: 6, marginTop: 10, padding: '6px 9px', borderRadius: 7, border: '1px solid #ef444455', background: '#7f1d1d33', color: '#fecaca', cursor: disconnectingId === c.id ? 'wait' : 'pointer', opacity: disconnectingId === c.id ? 0.65 : 1, fontSize: 11, fontWeight: 700 }}
                  >
                    {disconnectingId === c.id ? <RefreshCw size={12} className="spin" /> : <Unplug size={12} />}
                    {disconnectingId === c.id ? 'Đang ngắt' : 'Disconnect máy này'}
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
