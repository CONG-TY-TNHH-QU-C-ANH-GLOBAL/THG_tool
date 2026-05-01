import { useCallback, useEffect, useRef, useState, type MouseEvent, type WheelEvent } from 'react';
import { theme } from '../../constants/styles';
import { useWorkspaces } from '../../hooks/useWorkspaces';
import { useConnectors } from '../../hooks/useConnectors';
import { useAuthStore } from '../../stores/authStore';
import { getSystemInfo, type SystemInfo } from '../../services/systemService';
import { disconnectLocalConnector, getLocalConnectorScreen, sendConnectorInput } from '../../services/connectorsService';
import type { LocalConnector, LocalConnectorScreen, WorkspaceSessionSnapshot } from '../../types';
import { AlertTriangle, ArrowRight, Cpu, Monitor, StopCircle, LogIn, RefreshCw, CheckCircle, Plus, ShieldCheck, Laptop, Radio, Copy, KeyRound, Shield, Unplug, Eye, EyeOff } from 'lucide-react';
import VncCanvas from '../VncCanvas';
import '../../autoflow.css';

interface BrowserViewProps { orgId: string; }

function stateLabel(state?: string): string {
  switch (state) {
    case 'initializing': return 'đang khởi động';
    case 'display_ready': return 'desktop ready';
    case 'ready': return 'ready';
    case 'idle': return 'idle';
    case 'active': return 'active';
    case 'checkpoint': return 'human required';
    case 'human_required': return 'human required';
    case 'local_starting': return 'đang chờ Runtime';
    case 'local_active': return 'Chrome thật đang chạy';
    case 'local_login_required': return 'cần đăng nhập Facebook';
    case 'local_human_required': return 'Facebook cần xác minh';
    case 'local_ready': return 'Facebook local ready';
    case 'local_error': return 'local error';
    case 'error': return 'error';
    default: return state || '';
  }
}

function stateTone(state?: string) {
  if (state === 'error') return { color: '#fca5a5', bg: '#7f1d1d55', border: '#ef444466' };
  if (state === 'local_error') return { color: '#fca5a5', bg: '#7f1d1d55', border: '#ef444466' };
  if (state === 'checkpoint' || state === 'human_required') return { color: '#fcd34d', bg: '#78350f55', border: '#f59e0b66' };
  if (state === 'local_human_required' || state === 'local_login_required') return { color: '#fcd34d', bg: '#78350f55', border: '#f59e0b66' };
  if (state === 'initializing') return { color: '#fde68a', bg: '#78350f44', border: '#f59e0b55' };
  if (state === 'local_starting' || state === 'local_active' || state === 'local_ready') return { color: '#a7f3d0', bg: '#064e3b44', border: '#10b98155' };
  return { color: '#a7f3d0', bg: '#064e3b44', border: '#10b98155' };
}

function formatLastSeen(value?: string) {
  if (!value) return 'not connected';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString('vi-VN', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
}

function formatCountdown(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000));
  const minutes = Math.floor(total / 60);
  const seconds = total % 60;
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
}

type DownloadKey = keyof SystemInfo['agent_builds'];

const RUNTIME_DOWNLOADS: Array<{ key: DownloadKey; label: string; href: string }> = [
  { key: 'local_kit_windows', label: 'Windows', href: '/downloads/thg-local-kit-windows.zip' },
  { key: 'local_kit_mac_m1', label: 'macOS', href: '/downloads/thg-local-kit-mac-m1.zip' },
  { key: 'local_kit_linux', label: 'Linux', href: '/downloads/thg-local-kit-linux.zip' },
];

function connectorCapabilities(connector: LocalConnector): Record<string, unknown> {
  try {
    const parsed = JSON.parse(connector.capabilitiesJson || '{}');
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch {
    return {};
  }
}

function isDashboardStreamConnector(connector: LocalConnector): boolean {
  const caps = connectorCapabilities(connector);
  return connector.kind === 'desktop_connector' ||
    connector.transport === 'local_chrome' ||
    caps.native_companion === true ||
    caps.multi_profile === true;
}

function connectorStatusLabel(status?: string): string {
  switch ((status || '').toLowerCase()) {
    case 'pairing':
      return 'Đã ghép thiết bị';
    case 'online':
    case 'connector_online':
      return 'Sẵn sàng';
    case 'chrome_not_connected':
      return 'Chưa kết nối Chrome';
    case 'chrome_connected':
      return 'Đã thấy Chrome local';
    case 'facebook_login_required':
      return 'Chưa đăng nhập Facebook';
    case 'facebook_human_required':
      return 'Facebook cần xác minh';
    case 'facebook_logged_in':
      return 'Đã kết nối Facebook';
    case 'idle':
      return 'Đang chờ lệnh';
    case 'running':
      return 'Đang chạy';
    case 'error':
      return 'Cần kiểm tra';
    default:
      return status || 'Đang chờ lệnh';
  }
}

function isRemoteControlKey(key: string): boolean {
  return [
    'Enter', 'Backspace', 'Tab', 'Escape', 'Delete',
    'ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown',
    'Home', 'End', 'PageUp', 'PageDown',
  ].includes(key);
}

function LocalConnectorPanel({
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
    <div style={{ background: theme.surface, border: `1px solid ${online ? '#10b98166' : '#334155'}`, borderRadius: 10, overflow: 'hidden' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '11px 14px', borderBottom: `1px solid ${theme.border}` }}>
        <div style={{ width: 34, height: 34, borderRadius: 9, background: '#0f766e22', border: '1px solid #2dd4bf55', display: 'grid', placeItems: 'center' }}>
          <Laptop size={17} color="#5eead4" />
        </div>
        <div style={{ minWidth: 0 }}>
          <p style={{ color: theme.text, fontSize: 13, fontWeight: 800 }}>Browser stream tập trung trên máy nhân viên</p>
          <p style={{ color: theme.textMuted, fontSize: 11 }}>Flow production chính: THG Local Runtime chạy Chrome profile riêng trên device/IP thật và stream toàn bộ Facebook về dashboard.</p>
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
                Một gói theo hệ điều hành, bên trong có THG Local Runtime để stream Chrome profile riêng về dashboard.
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
                Sau khi giải nén, chạy file Start trong kit. Windows dùng Start-THG-Local-Runtime.cmd để cửa sổ luôn mở cho bạn nhập mã kết nối.
              </p>
            </div>

            <div style={{ border: `1px solid ${pairingCode ? '#22c55e55' : theme.border}`, borderRadius: 8, padding: 11, background: theme.surface }}>
              <p style={{ color: '#bbf7d0', fontSize: 11, fontWeight: 800, marginBottom: 7 }}>2. Ghép thiết bị với workspace</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 50 }}>
                Tạo mã rồi dán vào cửa sổ THG Local Runtime. Sau khi ghép thành công, Runtime tự online lại bằng token riêng.
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
                THG server trong Runtime phải trùng domain dashboard đang tạo mã. Mã chỉ dùng một lần và hết hạn sau 10 phút.
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
              <p style={{ color: '#fcd34d', fontSize: 11, fontWeight: 800, marginBottom: 7 }}>3. Chạy Facebook trong Browser dashboard</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 50 }}>
                Bấm Chạy Facebook trên account. Runtime sẽ mở Chrome profile riêng trên máy nhân viên và stream về website, không giật tab Chrome cá nhân.
              </p>
              <span style={{ color: '#fef3c7', fontSize: 11, display: 'inline-flex', gap: 5, alignItems: 'center' }}><Shield size={12} /> Không nhập mật khẩu Facebook vào THG</span>
            </div>

            <div style={{ border: `1px solid ${runtimeOnline ? '#22c55e66' : theme.border}`, borderRadius: 8, padding: 11, background: theme.surface }}>
              <p style={{ color: runtimeOnline ? '#86efac' : theme.textMuted, fontSize: 11, fontWeight: 800, marginBottom: 7 }}>4. Dashboard nhận stream thật</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 50 }}>
                Khi Runtime online, Browser tab sẽ nhận ảnh từ Chrome profile local và agent mới được phép chạy crawler/comment/inbox qua kênh đó.
              </p>
              <span style={{ color: runtimeOnline ? '#4ade80' : theme.textFaint, fontSize: 12, display: 'inline-flex', gap: 5, alignItems: 'center' }}>
                {runtimeOnline ? <CheckCircle size={13} /> : <Radio size={13} />} {runtimeOnline ? 'Runtime đã sẵn sàng stream' : online ? 'Đã có thiết bị online, đang chờ Runtime stream' : 'Đang chờ máy kết nối'}
              </span>
            </div>
          </div>
        </div>
      )}

      {connectors.length === 0 ? (
        <p style={{ color: theme.textMuted, fontSize: 12, padding: '12px 14px' }}>
          Chưa có thiết bị nào kết nối. Cài THG Local Runtime để stream Browser dashboard, tạo mã kết nối, rồi nhập mã trong app.
        </p>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))', gap: 10, padding: 12 }}>
          {connectors.map(c => (
            <div key={c.id} style={{ border: `1px solid ${c.online ? '#22c55e55' : theme.border}`, borderRadius: 8, background: theme.surfaceAlt, padding: 11 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 7 }}>
                <span style={{ width: 8, height: 8, borderRadius: '50%', background: c.online ? '#4ade80' : theme.textFaint }} />
                <p style={{ color: theme.text, fontSize: 13, fontWeight: 700, flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.name}</p>
                <span style={{ color: c.online ? '#4ade80' : theme.textFaint, fontSize: 10 }}>{c.online ? 'online' : 'offline'}</span>
              </div>
              <p style={{ color: theme.textMuted, fontSize: 11 }}>{c.hostname || 'unknown host'} · {c.os || 'unknown os'} · {c.version || 'no version'}</p>
              <p style={{ color: theme.textFaint, fontSize: 11, marginTop: 5 }}>Lần cuối {formatLastSeen(c.lastSeen)} · {connectorStatusLabel(c.streamStatus)}</p>
              {c.currentUrl && <p style={{ color: '#93c5fd', fontSize: 11, marginTop: 5, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.currentUrl}</p>}
              {c.streamStatus === 'facebook_logged_in' && c.fbUserId && <p style={{ color: '#c4b5fd', fontSize: 11, marginTop: 5 }}>FB {c.fbUserId}</p>}
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
          ))}
        </div>
      )}
    </div>
  );
}

function LocalChromeViewer({
  screen,
  accountId,
  accountName,
  loading,
  onRefresh,
}: {
  screen: LocalConnectorScreen | null;
  accountId: number;
  accountName?: string;
  loading: boolean;
  onRefresh: () => void;
}) {
  const imgRef = useRef<HTMLImageElement | null>(null);
  const surfaceRef = useRef<HTMLDivElement | null>(null);
  const inputQueueRef = useRef<Promise<void>>(Promise.resolve());
  const lastWheelAtRef = useRef(0);
  const [inputStatus, setInputStatus] = useState<string | null>(null);
  const [inputActive, setInputActive] = useState(false);
  const age = screen?.updatedAt ? Math.max(0, Math.round((Date.now() - new Date(screen.updatedAt).getTime()) / 1000)) : null;

  const queueInput = useCallback((type: 'click' | 'key' | 'text' | 'scroll', payload: Record<string, unknown>) => {
    if (!screen?.imageData) return;
    inputQueueRef.current = inputQueueRef.current
      .catch(() => undefined)
      .then(async () => {
        try {
          const res = await sendConnectorInput(accountId, type, payload);
          setInputStatus(`Input queued #${res.id}`);
        } catch (err) {
          setInputStatus(err instanceof Error ? err.message : 'Khong gui duoc thao tac den THG Local Runtime');
        }
      });
  }, [accountId, screen?.imageData]);

  const imagePoint = (clientX: number, clientY: number) => {
    const img = imgRef.current;
    if (!img || img.naturalWidth <= 0 || img.naturalHeight <= 0) return null;
    const rect = img.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) return null;
    return {
      x: Math.max(0, Math.min(img.naturalWidth, (clientX - rect.left) * (img.naturalWidth / rect.width))),
      y: Math.max(0, Math.min(img.naturalHeight, (clientY - rect.top) * (img.naturalHeight / rect.height))),
    };
  };

  const handlePointerDown = (e: MouseEvent<HTMLImageElement>) => {
    if (!screen?.imageData) return;
    setInputActive(true);
    surfaceRef.current?.focus();
    const point = imagePoint(e.clientX, e.clientY);
    if (!point) return;
    void queueInput('click', {
      x: point.x,
      y: point.y,
      button: e.button === 2 ? 'right' : e.button === 1 ? 'middle' : 'left',
      clicks: Math.max(1, e.detail || 1),
    });
  };

  const handleWheel = (e: WheelEvent<HTMLImageElement>) => {
    if (!screen?.imageData) return;
    const now = Date.now();
    if (now - lastWheelAtRef.current < 120) return;
    lastWheelAtRef.current = now;
    setInputActive(true);
    const point = imagePoint(e.clientX, e.clientY) ?? { x: 0, y: 0 };
    void queueInput('scroll', {
      x: point.x,
      y: point.y,
      delta_x: e.deltaX,
      delta_y: e.deltaY,
    });
  };

  useEffect(() => {
    if (!inputActive || !screen?.imageData) return;
    const handleWindowKeyDown = (e: globalThis.KeyboardEvent) => {
      if (e.key.length === 1 && !e.ctrlKey && !e.altKey && !e.metaKey) {
        e.preventDefault();
        queueInput('text', { text: e.key });
        return;
      }
      if (isRemoteControlKey(e.key) || e.ctrlKey || e.metaKey) {
        e.preventDefault();
        queueInput('key', {
          key: e.key,
          code: e.code,
          ctrl_key: e.ctrlKey,
          alt_key: e.altKey,
          shift_key: e.shiftKey,
          meta_key: e.metaKey,
        });
      }
    };
    const handleWindowPaste = (e: globalThis.ClipboardEvent) => {
      const text = e.clipboardData?.getData('text') ?? '';
      if (!text) return;
      e.preventDefault();
      queueInput('text', { text: text.slice(0, 256) });
    };
    window.addEventListener('keydown', handleWindowKeyDown, true);
    window.addEventListener('paste', handleWindowPaste, true);
    return () => {
      window.removeEventListener('keydown', handleWindowKeyDown, true);
      window.removeEventListener('paste', handleWindowPaste, true);
    };
  }, [inputActive, queueInput, screen?.imageData]);

  return (
    <div style={{ background: '#020617', borderRadius: 12, overflow: 'hidden', border: `1px solid ${theme.border}` }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '10px 12px', background: theme.surface, borderBottom: `1px solid ${theme.border}` }}>
        <span style={{ width: 8, height: 8, borderRadius: '50%', background: screen?.imageData ? '#4ade80' : theme.textFaint }} />
        <Monitor size={14} color="#5eead4" />
        <div style={{ minWidth: 0, flex: 1 }}>
          <p style={{ color: theme.text, fontSize: 13, fontWeight: 800 }}>Chrome thật {accountName ? `- ${accountName}` : ''}</p>
          <p style={{ color: theme.textMuted, fontSize: 11, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {screen?.currentUrl || 'Đang chờ ảnh từ THG Local Runtime'}
          </p>
        </div>
        {inputActive && <span style={{ color: '#5eead4', border: '1px solid #14b8a644', background: '#134e4a33', borderRadius: 6, padding: '3px 8px', fontSize: 11 }}>control active</span>}
        {screen?.fbUserId && <span style={{ color: '#c4b5fd', border: '1px solid #6366f144', background: '#312e8133', borderRadius: 6, padding: '3px 8px', fontSize: 11 }}>FB {screen.fbUserId}</span>}
        {inputStatus && <span style={{ color: '#fca5a5', fontSize: 11, maxWidth: 280, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{inputStatus}</span>}
        {age !== null && <span style={{ color: age < 30 ? '#86efac' : '#fcd34d', fontSize: 11 }}>{age}s trước</span>}
        <button onClick={onRefresh} style={{ display: 'inline-flex', alignItems: 'center', gap: 5, padding: '6px 10px', background: 'transparent', border: `1px solid ${theme.border}`, borderRadius: 8, color: theme.textMuted, fontSize: 12, cursor: 'pointer' }}>
          <RefreshCw size={12} className={loading ? 'spin' : ''} /> Làm mới
        </button>
      </div>
      <div
        ref={surfaceRef}
        tabIndex={0}
        style={{ minHeight: 420, display: 'grid', placeItems: 'center', background: '#000', outline: 'none' }}
      >
        {screen?.imageData ? (
          <img
            ref={imgRef}
            src={screen.imageData}
            alt="Local Chrome Facebook"
            onMouseDown={handlePointerDown}
            onWheel={handleWheel}
            onContextMenu={e => e.preventDefault()}
            style={{ width: '100%', height: 'auto', display: 'block', background: '#000', cursor: 'crosshair', userSelect: 'none' }}
          />
        ) : (
          <div style={{ textAlign: 'center', padding: 28, maxWidth: 520 }}>
            <Laptop size={34} color="#5eead4" style={{ marginBottom: 12 }} />
            <p style={{ color: theme.text, fontSize: 14, fontWeight: 800, marginBottom: 6 }}>Đang chờ Chrome thật trên máy nhân viên</p>
            <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.6 }}>
              THG Local Runtime sẽ chạy Chrome profile riêng trên máy nhân viên và gửi ảnh về dashboard. Chrome cá nhân của nhân viên không bị tự chuyển tab.
            </p>
          </div>
        )}
      </div>
    </div>
  );
}

function CyberEmptyState({ onCreate, loading }: { onCreate: () => void; loading: boolean }) {
  return (
    <div style={{
      position: 'relative',
      overflow: 'hidden',
      border: '1px solid #22d3ee55',
      background: 'linear-gradient(135deg, #07111f 0%, #111827 46%, #111520 100%)',
      borderRadius: 10,
      padding: 26,
      minHeight: 230,
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      boxShadow: '0 0 0 1px #0e749044 inset, 0 18px 60px #00000040',
    }}>
      <div style={{ position: 'absolute', inset: 0, backgroundImage: 'linear-gradient(#22d3ee12 1px, transparent 1px), linear-gradient(90deg, #22d3ee10 1px, transparent 1px)', backgroundSize: '28px 28px', opacity: 0.55 }} />
      <div style={{ position: 'absolute', top: 0, left: 0, right: 0, height: 1, background: 'linear-gradient(90deg, transparent, #67e8f9, transparent)' }} />
      <div style={{ position: 'relative', textAlign: 'center', maxWidth: 560 }}>
        <div style={{ width: 46, height: 46, margin: '0 auto 14px', borderRadius: 12, background: '#0e749033', border: '1px solid #22d3ee66', display: 'grid', placeItems: 'center', boxShadow: '0 0 24px #06b6d455' }}>
          <Cpu size={22} color="#67e8f9" />
        </div>
        <p style={{ color: '#67e8f9', fontSize: 11, fontWeight: 800, marginBottom: 8 }}>CYBERTECH SIGNAL</p>
        <h3 style={{ color: theme.textWhite, fontSize: 18, fontWeight: 800, marginBottom: 8 }}>Workspace chưa có tài khoản Facebook</h3>
        <p style={{ color: theme.textMuted, fontSize: 13, lineHeight: 1.6, marginBottom: 18 }}>
          Khởi tạo phiên Facebook đầu tiên để agent có browser riêng, session riêng và dữ liệu automation được gắn đúng workspace.
        </p>
        <button
          onClick={onCreate}
          disabled={loading}
          style={{ display: 'inline-flex', alignItems: 'center', gap: 8, padding: '10px 18px', background: '#0891b2', border: '1px solid #67e8f966', borderRadius: 8, color: '#fff', fontSize: 13, fontWeight: 700, cursor: loading ? 'wait' : 'pointer', opacity: loading ? 0.65 : 1, boxShadow: '0 10px 30px #0891b244' }}
        >
          {loading ? <RefreshCw size={15} className="spin" /> : <Plus size={15} />}
          {loading ? 'Đang khởi tạo' : 'Tạo Facebook workspace'}
          {!loading && <ArrowRight size={15} />}
        </button>
      </div>
    </div>
  );
}

export default function BrowserView({ orgId }: BrowserViewProps) {
  void orgId;
  const currentUser = useAuthStore(s => s.user);
  const { workspaces, actionLoading, refresh, start, startNew, stop, syncSession } = useWorkspaces();
  const { connectors, creating: connectorCreating, refresh: refreshConnectors, createPairingCode } = useConnectors();
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [newLoading, setNewLoading] = useState(false);
  const [sessionInfo, setSessionInfo] = useState<WorkspaceSessionSnapshot | null>(null);
  const [syncLoading, setSyncLoading] = useState(false);
  const [syncError, setSyncError] = useState<string | null>(null);
  const [autoSyncPaused, setAutoSyncPaused] = useState(false);
  const [pairingCode, setPairingCode] = useState('');
  const [pairingExpiresAt, setPairingExpiresAt] = useState('');
  const [systemInfo, setSystemInfo] = useState<SystemInfo | null>(null);
  const [localScreen, setLocalScreen] = useState<LocalConnectorScreen | null>(null);
  const [localScreenLoading, setLocalScreenLoading] = useState(false);
  const [disconnectingId, setDisconnectingId] = useState<number | null>(null);
  const [connectorNotice, setConnectorNotice] = useState<string | null>(null);
  const [browserNotice, setBrowserNotice] = useState<string | null>(null);

  const selectedWs = workspaces.find(w => w.accountId === selectedId);
  const selectedIsLocal = Boolean(selectedWs?.browserState?.startsWith('local_'));
  const humanRequired = Boolean(
    sessionInfo?.humanRequired ||
    sessionInfo?.checkpoint ||
    selectedWs?.browserState === 'checkpoint' ||
    selectedWs?.browserState === 'human_required'
  );
  const hasSavedSession = Boolean(sessionInfo?.loggedIn || selectedWs?.loggedIn);
  const manualCaptureMode = !hasSavedSession || humanRequired || autoSyncPaused;

  useEffect(() => {
    if (selectedId !== null && selectedWs?.running) return;
    const firstRunning = workspaces.find(w => w.running);
    setSelectedId(firstRunning?.accountId ?? null);
  }, [selectedId, selectedWs?.running, workspaces]);

  useEffect(() => {
    if (newLoading && workspaces.some(w => w.running)) {
      setNewLoading(false);
    }
  }, [newLoading, workspaces]);

  useEffect(() => {
    getSystemInfo().then(setSystemInfo).catch(() => setSystemInfo(null));
  }, []);

  const refreshLocalScreen = async (accountId = selectedId) => {
    if (!accountId) return;
    setLocalScreenLoading(true);
    try {
      setLocalScreen(await getLocalConnectorScreen(accountId));
    } catch {
      setLocalScreen(null);
    } finally {
      setLocalScreenLoading(false);
    }
  };

  useEffect(() => {
    if (!selectedId || !selectedIsLocal) {
      setLocalScreen(null);
      return;
    }
    let cancelled = false;
    const run = async () => {
      try {
        const screen = await getLocalConnectorScreen(selectedId);
        if (!cancelled) setLocalScreen(screen);
      } catch {
        if (!cancelled) setLocalScreen(null);
      }
    };
    void run();
    const timer = setInterval(run, 2000);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [selectedId, selectedIsLocal]);

  useEffect(() => {
    if (selectedId === null || !selectedWs?.running || selectedIsLocal) {
      setSessionInfo(null);
      setSyncError(null);
      setAutoSyncPaused(false);
      return;
    }
    if (!hasSavedSession || humanRequired || autoSyncPaused) {
      setSyncLoading(false);
      return;
    }

    let cancelled = false;
    const run = async () => {
      setSyncLoading(true);
      try {
        const snap = await syncSession(selectedId);
        if (!cancelled) {
          setSessionInfo(snap);
          setSyncError(null);
          if (!snap.loggedIn || snap.humanRequired || snap.checkpoint || snap.cookieError) {
            setAutoSyncPaused(true);
          }
        }
      } catch (e) {
        if (!cancelled) {
          setSyncError(e instanceof Error ? e.message : 'Không đồng bộ được session');
          setAutoSyncPaused(true);
        }
      } finally {
        if (!cancelled) setSyncLoading(false);
      }
    };

    void run();
    const timer = setInterval(run, 10000);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [selectedId, selectedWs?.running, selectedIsLocal, hasSavedSession, humanRequired, autoSyncPaused]); // eslint-disable-line react-hooks/exhaustive-deps

  const running = workspaces.filter(w => w.running).length;
  const hasOnlineConnector = connectors.some(c => c.online && isDashboardStreamConnector(c));

  const handleNewSession = async () => {
    if (!connectors.some(c => c.online)) {
      setBrowserNotice('Chưa có thiết bị nào online. Mở THG Local Runtime, nhập mã kết nối mới, rồi thực hiện lại.');
      return;
    }
    if (!connectors.some(c => c.online && isDashboardStreamConnector(c))) {
      setBrowserNotice('Thiết bị hiện tại chưa có THG Local Runtime sẵn sàng stream Browser dashboard. Tải Local Kit đúng hệ điều hành, chạy Runtime, nhập mã kết nối mới, rồi bấm Chạy Facebook.');
      return;
    }
    setNewLoading(true);
    setBrowserNotice(null);
    try {
      const id = await startNew();
      setSelectedId(id);
      setBrowserNotice('Đã tạo phiên Facebook local. THG Local Runtime sẽ chạy Chrome profile riêng trên máy nhân viên và stream ảnh về Browser dashboard.');
    } catch (e) {
      setBrowserNotice(e instanceof Error ? e.message : 'Không tạo được phiên mới');
    } finally {
      setNewLoading(false);
    }
  };

  const handleManualSync = async () => {
    if (selectedId === null) return;
    setSyncLoading(true);
    try {
      const snap = await syncSession(selectedId);
      setSessionInfo(snap);
      setSyncError(null);
      setAutoSyncPaused(!snap.loggedIn || snap.humanRequired || snap.checkpoint || Boolean(snap.cookieError));
    } catch (e) {
      setSyncError(e instanceof Error ? e.message : 'Không đồng bộ được session');
      setAutoSyncPaused(true);
    } finally {
      setSyncLoading(false);
    }
  };

  const handleCreateConnector = async () => {
    setConnectorNotice(null);
    const name = `Local Chrome ${new Date().toLocaleDateString('vi-VN')}`;
    const created = await createPairingCode(name, selectedId ?? undefined);
    setPairingCode(created.code);
    setPairingExpiresAt(created.expires_at);
  };

  const handleDisconnectConnector = async (connector: LocalConnector) => {
    setConnectorNotice(null);
    setDisconnectingId(connector.id);
    try {
      await disconnectLocalConnector(connector.id);
      setLocalScreen(null);
      await Promise.all([refreshConnectors(), refresh()]);
      setConnectorNotice(`Đã disconnect ${connector.hostname || connector.name}. Nếu Runtime còn mở, token sẽ bị từ chối ở heartbeat kế tiếp.`);
    } catch (e) {
      setConnectorNotice(e instanceof Error ? e.message : 'Không disconnect được thiết bị');
    } finally {
      setDisconnectingId(null);
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <div style={{ display: 'flex', gap: 16, padding: '8px 14px', background: theme.surface, borderRadius: 10, border: `1px solid ${theme.border}`, alignItems: 'center' }}>
        <span style={{ color: theme.textMuted, fontSize: 12 }}>Tài khoản: <strong style={{ color: theme.text }}>{workspaces.length}</strong></span>
        <span style={{ color: theme.textMuted, fontSize: 12 }}>Đang chạy: <strong style={{ color: running > 0 ? '#4ade80' : theme.textFaint }}>{running}</strong></span>
        <span style={{ color: theme.textMuted, fontSize: 12 }}>Local: <strong style={{ color: connectors.some(c => c.online) ? '#4ade80' : theme.textFaint }}>{connectors.filter(c => c.online).length}</strong></span>
        <button onClick={() => { void refresh(); void refreshConnectors(); }} style={{ marginLeft: 'auto', background: 'none', border: 'none', color: theme.textFaint, cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 4, fontSize: 12 }}>
          <RefreshCw size={12} /> Làm mới
        </button>
        <button
          onClick={() => void handleNewSession()}
          disabled={newLoading}
          style={{ display: 'flex', alignItems: 'center', gap: 5, padding: '5px 12px', background: '#16a34a', border: 'none', borderRadius: 7, color: '#fff', fontSize: 12, cursor: newLoading ? 'wait' : 'pointer', opacity: newLoading ? 0.6 : 1 }}
        >
          {newLoading ? <RefreshCw size={12} className="spin" /> : <Plus size={12} />}
          {newLoading ? 'Đang khởi động...' : 'Phiên mới'}
        </button>
      </div>

      <LocalConnectorPanel
        connectors={connectors}
        creating={connectorCreating}
        pairingCode={pairingCode}
        pairingExpiresAt={pairingExpiresAt}
        systemInfo={systemInfo}
        currentUserId={currentUser?.id ?? 0}
        currentUserRole={currentUser?.role ?? ''}
        disconnectingId={disconnectingId}
        onCreate={() => void handleCreateConnector()}
        onDisconnect={connector => void handleDisconnectConnector(connector)}
      />

      {connectorNotice && (
        <div style={{ padding: '9px 12px', borderRadius: 8, border: `1px solid ${connectorNotice.includes('Không') ? '#ef444466' : '#22c55e66'}`, background: connectorNotice.includes('Không') ? '#7f1d1d33' : '#064e3b33', color: connectorNotice.includes('Không') ? '#fecaca' : '#bbf7d0', fontSize: 12 }}>
          {connectorNotice}
        </div>
      )}
      {browserNotice && (
        <div style={{ padding: '9px 12px', borderRadius: 8, border: '1px solid #f59e0b66', background: '#78350f33', color: '#fef3c7', fontSize: 12 }}>
          {browserNotice}
        </div>
      )}

      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        {workspaces.length === 0 && (
          <CyberEmptyState onCreate={() => void handleNewSession()} loading={newLoading} />
        )}

        {workspaces.map(w => {
          const tone = stateTone(w.browserState);
          return (
            <div
              key={w.accountId}
              onClick={() => w.running && setSelectedId(w.accountId)}
              style={{
                display: 'flex', alignItems: 'center', gap: 10, padding: '10px 14px',
                background: selectedId === w.accountId ? '#1a2a1a' : theme.surface,
                border: `1px solid ${selectedId === w.accountId ? '#16a34a' : theme.border}`,
                borderRadius: 10, cursor: w.running ? 'pointer' : 'default',
              }}
            >
              <div style={{ width: 8, height: 8, borderRadius: '50%', flexShrink: 0, background: w.running ? '#4ade80' : theme.textFaint }} />
              <Monitor size={14} color={theme.textMuted} />
              <span style={{ flex: 1, color: theme.text, fontWeight: 500, fontSize: 13 }}>{w.accountName}</span>
              {w.loggedIn && (
                <span style={{ fontSize: 11, color: '#60a5fa', background: '#1e3a5f33', border: '1px solid #3b82f644', padding: '2px 8px', borderRadius: 6, display: 'inline-flex', alignItems: 'center', gap: 3 }}>
                  <CheckCircle size={10} />Đã đăng nhập
                </span>
              )}
              {w.fbUserId && (
                <span style={{ fontSize: 11, color: '#c4b5fd', background: '#312e8133', border: '1px solid #6366f144', padding: '2px 8px', borderRadius: 6 }}>
                  FB {w.fbUserId}
                </span>
              )}
              {w.browserState && (
                <span style={{ fontSize: 11, color: tone.color, background: tone.bg, border: `1px solid ${tone.border}`, padding: '2px 8px', borderRadius: 6 }}>
                  {stateLabel(w.browserState)}
                </span>
              )}
              {!w.running ? (
                <button
                  onClick={e => {
                    e.stopPropagation();
                    setBrowserNotice(null);
                    if (!hasOnlineConnector) {
                      setBrowserNotice('Browser dashboard cần THG Local Runtime để stream chuyên nghiệp. Tải Local Kit đúng hệ điều hành, chạy Runtime, nhập mã kết nối mới, rồi bấm Chạy Facebook.');
                      return;
                    }
                    void start(w.accountId)
                      .then(() => {
                        setSelectedId(w.accountId);
                        setBrowserNotice('Đang chạy Facebook trên THG Local Runtime. Browser dashboard sẽ nhận ảnh từ Chrome profile local thay vì mở tab Chrome cá nhân.');
                        void refreshLocalScreen(w.accountId);
                      })
                      .catch(err => setBrowserNotice(err instanceof Error ? err.message : 'Không kết nối được tab Facebook'));
                  }}
                  disabled={actionLoading.has(w.accountId)}
                  style={{ display: 'flex', alignItems: 'center', gap: 5, padding: '5px 12px', background: hasOnlineConnector ? '#16a34a' : '#78350f', border: 'none', borderRadius: 7, color: hasOnlineConnector ? '#fff' : '#fcd34d', fontSize: 12, cursor: actionLoading.has(w.accountId) ? 'wait' : 'pointer', opacity: actionLoading.has(w.accountId) ? 0.6 : 1 }}
                >
                  {actionLoading.has(w.accountId) ? <RefreshCw size={12} className="spin" /> : <LogIn size={12} />}
                  {actionLoading.has(w.accountId) ? (hasOnlineConnector ? 'Đang mở Facebook...' : 'Đang kiểm tra...') : (hasOnlineConnector ? 'Chạy Facebook' : 'Chưa sẵn sàng')}
                </button>
              ) : (
                <button
                  onClick={e => { e.stopPropagation(); void stop(w.accountId).then(() => { if (selectedId === w.accountId) setSelectedId(null); }); }}
                  style={{ display: 'flex', alignItems: 'center', gap: 5, padding: '5px 12px', background: '#7f1d1d', border: 'none', borderRadius: 7, color: '#fca5a5', fontSize: 12, cursor: 'pointer' }}
                >
                  <StopCircle size={12} /> Dừng
                </button>
              )}
            </div>
          );
        })}
      </div>

      {selectedId !== null && selectedWs?.running && selectedIsLocal && (
        <LocalChromeViewer
          screen={localScreen}
          accountId={selectedId}
          accountName={selectedWs.accountName}
          loading={localScreenLoading}
          onRefresh={() => void refreshLocalScreen(selectedId)}
        />
      )}

      {selectedId !== null && selectedWs?.running && !selectedIsLocal && (
        <div style={{ background: '#000', borderRadius: 12, overflow: 'hidden', border: `1px solid ${theme.border}` }}>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, minmax(0, 1fr)) auto', gap: 10, alignItems: 'center', padding: '10px 12px', background: theme.surface, borderBottom: `1px solid ${theme.border}` }}>
            <div>
              <p style={{ color: theme.textFaint, fontSize: 10, marginBottom: 3 }}>Session</p>
              <p style={{ color: humanRequired ? '#fcd34d' : (sessionInfo?.loggedIn || selectedWs.loggedIn ? '#4ade80' : theme.textMuted), fontSize: 12, fontWeight: 600, display: 'flex', alignItems: 'center', gap: 5 }}>
                {humanRequired ? <AlertTriangle size={12} /> : <ShieldCheck size={12} />}
                {humanRequired ? 'Cần xác minh' : (sessionInfo?.loggedIn || selectedWs.loggedIn ? 'Đã lưu' : 'Chưa xác thực')}
              </p>
            </div>
            <div>
              <p style={{ color: theme.textFaint, fontSize: 10, marginBottom: 3 }}>Facebook ID</p>
              <p style={{ color: theme.text, fontSize: 12, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{sessionInfo?.fbUserId || selectedWs.fbUserId || '-'}</p>
            </div>
            <div>
              <p style={{ color: theme.textFaint, fontSize: 10, marginBottom: 3 }}>URL</p>
              <p style={{ color: theme.textMuted, fontSize: 12, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{sessionInfo?.currentUrl || '-'}</p>
            </div>
            <div>
              <p style={{ color: theme.textFaint, fontSize: 10, marginBottom: 3 }}>CDP</p>
              <p style={{ color: syncError || sessionInfo?.cookieError ? '#fca5a5' : theme.textMuted, fontSize: 12, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {humanRequired ? (sessionInfo?.humanReason || 'human_required') : (syncError || sessionInfo?.cookieError || (syncLoading ? 'đang đồng bộ' : 'sẵn sàng'))}
              </p>
            </div>
            <button
              onClick={() => void handleManualSync()}
              disabled={syncLoading}
              style={{ display: 'flex', alignItems: 'center', gap: 5, padding: '6px 10px', background: manualCaptureMode ? '#78350f44' : 'transparent', border: `1px solid ${manualCaptureMode ? '#f59e0b66' : theme.border}`, borderRadius: 8, color: manualCaptureMode ? '#fcd34d' : theme.textMuted, fontSize: 12, cursor: syncLoading ? 'wait' : 'pointer', whiteSpace: 'nowrap' }}
            >
              <RefreshCw size={12} className={syncLoading ? 'spin' : ''} /> {manualCaptureMode ? 'Đã vào Facebook - đồng bộ' : 'Đồng bộ'}
            </button>
          </div>
          <VncCanvas
            accountId={selectedId}
            accountName={selectedWs.accountName}
            cdpPort={selectedWs.cdpPort}
            vncPort={selectedWs.vncPort}
            errorMsg={selectedWs.errorMsg}
          />

          <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '8px 14px', background: theme.surfaceAlt, borderTop: `1px solid ${theme.border}` }}>
            {manualCaptureMode ? (
              <span style={{ color: '#fcd34d', fontSize: 12, display: 'flex', alignItems: 'center', gap: 5 }}>
                <AlertTriangle size={13} /> Auto-sync tạm dừng trong lúc đăng nhập, vào News Feed rồi bấm đồng bộ
              </span>
            ) : selectedWs.loggedIn ? (
              <span style={{ color: '#4ade80', fontSize: 12, display: 'flex', alignItems: 'center', gap: 5 }}>
                <CheckCircle size={13} /> Session đã được lưu
              </span>
            ) : (
              <span style={{ color: theme.textMuted, fontSize: 12, display: 'flex', alignItems: 'center', gap: 5 }}>
                <RefreshCw size={13} className={syncLoading ? 'spin' : ''} /> Đang tự xác thực session
              </span>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
