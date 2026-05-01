import { useEffect, useState } from 'react';
import { theme } from '../../constants/styles';
import { useWorkspaces } from '../../hooks/useWorkspaces';
import { useConnectors } from '../../hooks/useConnectors';
import { useAuthStore } from '../../stores/authStore';
import { getSystemInfo, type SystemInfo } from '../../services/systemService';
import { disconnectLocalConnector, getLocalConnectorScreen } from '../../services/connectorsService';
import type { LocalConnector, LocalConnectorScreen, WorkspaceSessionSnapshot } from '../../types';
import { AlertTriangle, ArrowRight, Cpu, Monitor, StopCircle, LogIn, RefreshCw, CheckCircle, Plus, ShieldCheck, Laptop, Radio, Copy, Download, KeyRound, Shield, Unplug } from 'lucide-react';
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
    case 'local_starting': return 'đang mở Chrome thật';
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

type DownloadKey = keyof SystemInfo['agent_builds'];

const DOWNLOADS: Array<{ key: DownloadKey; label: string; href: string }> = [
  { key: 'windows', label: 'Windows', href: '/downloads/thg-login-windows.exe' },
  { key: 'mac_m1', label: 'macOS Apple Silicon', href: '/downloads/thg-login-mac-m1' },
  { key: 'mac_intel', label: 'macOS Intel', href: '/downloads/thg-login-mac-intel' },
  { key: 'linux', label: 'Linux', href: '/downloads/thg-login-linux' },
];

function preferredDownloadKey(): DownloadKey {
  if (typeof navigator === 'undefined') return 'windows';
  const ua = navigator.userAgent.toLowerCase();
  const platform = navigator.platform.toLowerCase();
  if (platform.includes('win') || ua.includes('windows')) return 'windows';
  if (platform.includes('mac') || ua.includes('mac os')) return 'mac_m1';
  if (platform.includes('linux') || ua.includes('linux')) return 'linux';
  return 'windows';
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
  const facebookConnected = connectors.filter(c => c.online && c.streamStatus === 'facebook_logged_in').length;
  const [setupOpen, setSetupOpen] = useState(connectors.length === 0);
  const [selectedDownloadKey, setSelectedDownloadKey] = useState<DownloadKey>(() => preferredDownloadKey());
  const selectedDownload = DOWNLOADS.find(d => d.key === selectedDownloadKey) ?? DOWNLOADS[0];
  const selectedAvailable = Boolean(systemInfo?.agent_builds?.[selectedDownload.key]);
  const hasAnyBuild = DOWNLOADS.some(d => systemInfo?.agent_builds?.[d.key]);

  useEffect(() => {
    if (!systemInfo) return;
    if (systemInfo.agent_builds?.[selectedDownloadKey]) return;
    const firstAvailable = DOWNLOADS.find(d => systemInfo.agent_builds?.[d.key]);
    if (firstAvailable) setSelectedDownloadKey(firstAvailable.key);
  }, [selectedDownloadKey, systemInfo]);

  return (
    <div style={{ background: theme.surface, border: `1px solid ${online ? '#10b98166' : '#334155'}`, borderRadius: 10, overflow: 'hidden' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '11px 14px', borderBottom: `1px solid ${theme.border}` }}>
        <div style={{ width: 34, height: 34, borderRadius: 9, background: '#0f766e22', border: '1px solid #2dd4bf55', display: 'grid', placeItems: 'center' }}>
          <Laptop size={17} color="#5eead4" />
        </div>
        <div style={{ minWidth: 0 }}>
          <p style={{ color: theme.text, fontSize: 13, fontWeight: 800 }}>Chrome thật của nhân viên</p>
          <p style={{ color: theme.textMuted, fontSize: 11 }}>Dùng chính máy và Chrome cá nhân của nhân viên để giữ đúng IP, thiết bị và phiên Facebook thật.</p>
        </div>
        <span style={{ marginLeft: 'auto', color: online ? '#4ade80' : theme.textMuted, fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 5 }}>
          <Radio size={12} /> {facebookConnected}/{connectors.length} Facebook ready
        </span>
        <button onClick={() => setSetupOpen(v => !v)} style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '7px 12px', borderRadius: 8, border: '1px solid #2dd4bf66', background: '#0f766e33', color: '#ccfbf1', cursor: 'pointer', fontSize: 12, fontWeight: 700 }}>
          <Laptop size={13} />
          Bắt đầu kết nối
        </button>
      </div>

      {setupOpen && (
        <div style={{ padding: 12, borderBottom: `1px solid ${theme.border}`, background: '#07131f' }}>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(210px, 1fr))', gap: 10 }}>
            <div style={{ border: `1px solid ${theme.border}`, borderRadius: 8, padding: 11, background: theme.surface }}>
              <p style={{ color: '#93c5fd', fontSize: 11, fontWeight: 800, marginBottom: 7 }}>1. Tải ứng dụng THG</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 42 }}>Cài app trên đúng máy sẽ dùng Facebook. Máy Windows tải Windows, máy Mac tải macOS.</p>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginBottom: 8 }}>
                {DOWNLOADS.map(d => {
                  const available = Boolean(systemInfo?.agent_builds?.[d.key]);
                  const selected = selectedDownloadKey === d.key;
                  return (
                    <button
                      key={d.key}
                      type="button"
                      onClick={() => setSelectedDownloadKey(d.key)}
                      disabled={!available}
                      style={{
                        display: 'inline-flex',
                        alignItems: 'center',
                        gap: 5,
                        padding: '6px 8px',
                        borderRadius: 7,
                        border: `1px solid ${selected ? '#60a5fa99' : theme.border}`,
                        background: selected ? '#1d4ed833' : theme.surfaceAlt,
                        color: available ? '#dbeafe' : theme.textFaint,
                        cursor: available ? 'pointer' : 'not-allowed',
                        opacity: available ? 1 : 0.5,
                        fontSize: 11,
                        fontWeight: selected ? 800 : 600,
                      }}
                    >
                      <Monitor size={11} /> {d.label}
                    </button>
                  );
                })}
              </div>
              <a
                href={selectedAvailable ? selectedDownload.href : undefined}
                style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '7px 10px', borderRadius: 7, border: '1px solid #3b82f666', background: selectedAvailable ? '#1d4ed833' : '#33415555', color: selectedAvailable ? '#dbeafe' : theme.textFaint, pointerEvents: selectedAvailable ? 'auto' : 'none', textDecoration: 'none', fontSize: 12, fontWeight: 700 }}
              >
                <Download size={13} /> {selectedAvailable ? `Tải ${selectedDownload.label}` : 'Chưa có bản cài'}
              </a>
              {!selectedAvailable && hasAnyBuild && (
                <p style={{ color: theme.textFaint, fontSize: 10, marginTop: 7 }}>Bản đang chọn chưa có, chọn hệ điều hành khác trong danh sách.</p>
              )}
            </div>

            <div style={{ border: `1px solid ${pairingCode ? '#22c55e55' : theme.border}`, borderRadius: 8, padding: 11, background: theme.surface }}>
              <p style={{ color: '#bbf7d0', fontSize: 11, fontWeight: 800, marginBottom: 7 }}>2. Nhập mã vào app</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 50 }}>Bấm tạo mã, sau đó mở THG Local Connector và dán mã này vào app. Khi app báo Connected, mã này không cần dùng lại.</p>
              {pairingCode ? (
                <div style={{ display: 'flex', gap: 7, alignItems: 'center' }}>
                  <code style={{ color: '#dcfce7', fontSize: 18, fontWeight: 900, flex: 1 }}>{pairingCode}</code>
                  <button onClick={() => navigator.clipboard?.writeText(pairingCode)} style={{ display: 'inline-flex', alignItems: 'center', gap: 5, padding: '6px 9px', borderRadius: 7, border: `1px solid ${theme.border}`, background: theme.surfaceAlt, color: theme.textMuted, cursor: 'pointer', fontSize: 11 }}>
                    <Copy size={12} /> Copy
                  </button>
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
              <p style={{ color: '#fcd34d', fontSize: 11, fontWeight: 800, marginBottom: 7 }}>3. Mở Facebook trong Chrome</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 50 }}>Trên cùng máy vừa kết nối, mở Chrome cá nhân và vào facebook.com. Nếu Facebook đã đăng nhập sẵn thì chỉ cần giữ tab đó mở.</p>
              <span style={{ color: '#fef3c7', fontSize: 11, display: 'inline-flex', gap: 5, alignItems: 'center' }}><Shield size={12} /> Không nhập mật khẩu Facebook vào THG</span>
            </div>

            <div style={{ border: `1px solid ${facebookConnected ? '#22c55e66' : theme.border}`, borderRadius: 8, padding: 11, background: theme.surface }}>
              <p style={{ color: facebookConnected ? '#86efac' : theme.textMuted, fontSize: 11, fontWeight: 800, marginBottom: 7 }}>4. Facebook đã sẵn sàng</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 50 }}>Khi dòng máy bên dưới hiện Đã kết nối Facebook, dashboard đã thấy đúng Chrome/Facebook trên máy đó.</p>
              <span style={{ color: facebookConnected ? '#4ade80' : theme.textFaint, fontSize: 12, display: 'inline-flex', gap: 5, alignItems: 'center' }}>
                {facebookConnected ? <CheckCircle size={13} /> : <Radio size={13} />} {facebookConnected ? 'Đã kết nối Facebook' : online ? 'Máy online, đang chờ Chrome/Facebook' : 'Đang chờ máy kết nối'}
              </span>
            </div>
          </div>
        </div>
      )}

      {connectors.length === 0 ? (
        <p style={{ color: theme.textMuted, fontSize: 12, padding: '12px 14px' }}>Chưa có máy nhân viên nào kết nối. Tải app, nhập mã kết nối, rồi mở Facebook trong Chrome trên máy đó.</p>
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
  accountName,
  loading,
  onRefresh,
}: {
  screen: LocalConnectorScreen | null;
  accountName?: string;
  loading: boolean;
  onRefresh: () => void;
}) {
  const age = screen?.updatedAt ? Math.max(0, Math.round((Date.now() - new Date(screen.updatedAt).getTime()) / 1000)) : null;
  return (
    <div style={{ background: '#020617', borderRadius: 12, overflow: 'hidden', border: `1px solid ${theme.border}` }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '10px 12px', background: theme.surface, borderBottom: `1px solid ${theme.border}` }}>
        <span style={{ width: 8, height: 8, borderRadius: '50%', background: screen?.imageData ? '#4ade80' : theme.textFaint }} />
        <Monitor size={14} color="#5eead4" />
        <div style={{ minWidth: 0, flex: 1 }}>
          <p style={{ color: theme.text, fontSize: 13, fontWeight: 800 }}>Chrome thật {accountName ? `- ${accountName}` : ''}</p>
          <p style={{ color: theme.textMuted, fontSize: 11, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {screen?.currentUrl || 'Đang chờ ảnh từ THG Local Connector'}
          </p>
        </div>
        {screen?.fbUserId && <span style={{ color: '#c4b5fd', border: '1px solid #6366f144', background: '#312e8133', borderRadius: 6, padding: '3px 8px', fontSize: 11 }}>FB {screen.fbUserId}</span>}
        {age !== null && <span style={{ color: age < 30 ? '#86efac' : '#fcd34d', fontSize: 11 }}>{age}s trước</span>}
        <button onClick={onRefresh} style={{ display: 'inline-flex', alignItems: 'center', gap: 5, padding: '6px 10px', background: 'transparent', border: `1px solid ${theme.border}`, borderRadius: 8, color: theme.textMuted, fontSize: 12, cursor: 'pointer' }}>
          <RefreshCw size={12} className={loading ? 'spin' : ''} /> Làm mới
        </button>
      </div>
      <div style={{ minHeight: 420, display: 'grid', placeItems: 'center', background: '#000' }}>
        {screen?.imageData ? (
          <img src={screen.imageData} alt="Local Chrome Facebook" style={{ width: '100%', height: 'auto', display: 'block', background: '#000' }} />
        ) : (
          <div style={{ textAlign: 'center', padding: 28, maxWidth: 520 }}>
            <Laptop size={34} color="#5eead4" style={{ marginBottom: 12 }} />
            <p style={{ color: theme.text, fontSize: 14, fontWeight: 800, marginBottom: 6 }}>Đang chờ Chrome thật trên máy nhân viên</p>
            <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.6 }}>
              Bấm mở Chrome thật cho account này, giữ THG Local Connector đang chạy. App sẽ tự mở một Chrome profile riêng và gửi màn hình về dashboard.
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
    const timer = setInterval(run, 5000);
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
  const hasLocalConnector = connectors.length > 0;

  const handleNewSession = async () => {
    setNewLoading(true);
    setBrowserNotice(null);
    try {
      const id = await startNew();
      setSelectedId(id);
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
      setConnectorNotice(`Đã disconnect ${connector.hostname || connector.name}. Nếu app còn mở, nó sẽ dừng ở heartbeat kế tiếp.`);
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
                    void start(w.accountId)
                      .then(() => { setSelectedId(w.accountId); void refreshLocalScreen(w.accountId); })
                      .catch(err => setBrowserNotice(err instanceof Error ? err.message : 'Không mở được Chrome thật'));
                  }}
                  disabled={actionLoading.has(w.accountId)}
                  style={{ display: 'flex', alignItems: 'center', gap: 5, padding: '5px 12px', background: '#16a34a', border: 'none', borderRadius: 7, color: '#fff', fontSize: 12, cursor: actionLoading.has(w.accountId) ? 'wait' : 'pointer', opacity: actionLoading.has(w.accountId) ? 0.6 : 1 }}
                >
                  {actionLoading.has(w.accountId) ? <RefreshCw size={12} className="spin" /> : <LogIn size={12} />}
                  {actionLoading.has(w.accountId) ? 'Đang khởi động...' : 'Bắt đầu'}
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
