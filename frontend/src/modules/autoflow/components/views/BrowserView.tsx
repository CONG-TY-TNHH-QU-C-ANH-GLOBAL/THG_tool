import { useEffect, useState } from 'react';
import { theme } from '../../constants/styles';
import { useWorkspaces } from '../../hooks/useWorkspaces';
import { useConnectors } from '../../hooks/useConnectors';
import { getSystemInfo, type SystemInfo } from '../../services/systemService';
import type { LocalConnector, WorkspaceSessionSnapshot } from '../../types';
import { AlertTriangle, ArrowRight, Cpu, Monitor, StopCircle, LogIn, RefreshCw, CheckCircle, Plus, ShieldCheck, Laptop, Radio, Copy, Download, KeyRound, Shield } from 'lucide-react';
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
    case 'error': return 'error';
    default: return state || '';
  }
}

function stateTone(state?: string) {
  if (state === 'error') return { color: '#fca5a5', bg: '#7f1d1d55', border: '#ef444466' };
  if (state === 'checkpoint' || state === 'human_required') return { color: '#fcd34d', bg: '#78350f55', border: '#f59e0b66' };
  if (state === 'initializing') return { color: '#fde68a', bg: '#78350f44', border: '#f59e0b55' };
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

function LocalConnectorPanel({
  connectors,
  creating,
  pairingCode,
  pairingExpiresAt,
  systemInfo,
  onCreate,
}: {
  connectors: LocalConnector[];
  creating: boolean;
  pairingCode: string;
  pairingExpiresAt: string;
  systemInfo: SystemInfo | null;
  onCreate: () => void;
}) {
  const online = connectors.filter(c => c.online).length;
  const [setupOpen, setSetupOpen] = useState(connectors.length === 0);
  const preferred = preferredDownloadKey();
  const primaryDownload = DOWNLOADS.find(d => d.key === preferred) ?? DOWNLOADS[0];
  const primaryAvailable = Boolean(systemInfo?.agent_builds?.[primaryDownload.key]);
  const hasAnyBuild = DOWNLOADS.some(d => systemInfo?.agent_builds?.[d.key]);
  return (
    <div style={{ background: theme.surface, border: `1px solid ${online ? '#10b98166' : '#334155'}`, borderRadius: 10, overflow: 'hidden' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '11px 14px', borderBottom: `1px solid ${theme.border}` }}>
        <div style={{ width: 34, height: 34, borderRadius: 9, background: '#0f766e22', border: '1px solid #2dd4bf55', display: 'grid', placeItems: 'center' }}>
          <Laptop size={17} color="#5eead4" />
        </div>
        <div style={{ minWidth: 0 }}>
          <p style={{ color: theme.text, fontSize: 13, fontWeight: 800 }}>Chrome thật của nhân viên</p>
          <p style={{ color: theme.textMuted, fontSize: 11 }}>Primary production path: dùng device/IP/profile thật, dashboard chỉ điều phối và quan sát automation.</p>
        </div>
        <span style={{ marginLeft: 'auto', color: online ? '#4ade80' : theme.textMuted, fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 5 }}>
          <Radio size={12} /> {online}/{connectors.length} online
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
              <p style={{ color: '#93c5fd', fontSize: 11, fontWeight: 800, marginBottom: 7 }}>1. Tải app desktop</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 50 }}>Cài THG Local Connector trên máy đang dùng Chrome/Facebook thật của nhân viên.</p>
              <a
                href={primaryAvailable ? primaryDownload.href : undefined}
                style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '7px 10px', borderRadius: 7, border: '1px solid #3b82f666', background: primaryAvailable ? '#1d4ed833' : '#33415555', color: primaryAvailable ? '#dbeafe' : theme.textFaint, pointerEvents: primaryAvailable ? 'auto' : 'none', textDecoration: 'none', fontSize: 12, fontWeight: 700 }}
              >
                <Download size={13} /> {primaryAvailable ? `Tải cho ${primaryDownload.label}` : 'Chưa có bản cài'}
              </a>
              {!primaryAvailable && hasAnyBuild && (
                <p style={{ color: theme.textFaint, fontSize: 10, marginTop: 7 }}>Bản cho hệ điều hành này chưa có, chọn bản khác trong mục Settings khi cần.</p>
              )}
            </div>

            <div style={{ border: `1px solid ${pairingCode ? '#22c55e55' : theme.border}`, borderRadius: 8, padding: 11, background: theme.surface }}>
              <p style={{ color: '#bbf7d0', fontSize: 11, fontWeight: 800, marginBottom: 7 }}>2. Tạo mã kết nối</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 50 }}>Mã ngắn hạn chỉ dùng một lần khi cài app. Sau khi pair, app tự lưu device token riêng.</p>
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
              <p style={{ color: '#fcd34d', fontSize: 11, fontWeight: 800, marginBottom: 7 }}>3. Mở Chrome Facebook</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 50 }}>Mở app, dán mã kết nối trên máy đang dùng Chrome/Facebook thật. Dashboard sẽ nhận heartbeat tự động.</p>
              <span style={{ color: '#fef3c7', fontSize: 11, display: 'inline-flex', gap: 5, alignItems: 'center' }}><Shield size={12} /> Không cần nhập mật khẩu Facebook vào THG</span>
            </div>

            <div style={{ border: `1px solid ${online ? '#22c55e66' : theme.border}`, borderRadius: 8, padding: 11, background: theme.surface }}>
              <p style={{ color: online ? '#86efac' : theme.textMuted, fontSize: 11, fontWeight: 800, marginBottom: 7 }}>4. Xác nhận online</p>
              <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.45, minHeight: 50 }}>Khi thấy trạng thái online, agent có thể nhận lệnh crawler/comment/inbox qua Chrome thật của nhân viên.</p>
              <span style={{ color: online ? '#4ade80' : theme.textFaint, fontSize: 12, display: 'inline-flex', gap: 5, alignItems: 'center' }}>
                {online ? <CheckCircle size={13} /> : <Radio size={13} />} {online ? 'Đã có connector online' : 'Đang chờ connector'}
              </span>
            </div>
          </div>
        </div>
      )}

      {connectors.length === 0 ? (
        <p style={{ color: theme.textMuted, fontSize: 12, padding: '12px 14px' }}>Chưa có máy nhân viên nào kết nối. Tạo mã kết nối rồi cài THG Local Connector trên máy dùng Chrome thật.</p>
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
              <p style={{ color: theme.textFaint, fontSize: 11, marginTop: 5 }}>Seen {formatLastSeen(c.lastSeen)} · stream {c.streamStatus || 'idle'}</p>
              {c.currentUrl && <p style={{ color: '#93c5fd', fontSize: 11, marginTop: 5, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.currentUrl}</p>}
              {c.fbUserId && <p style={{ color: '#c4b5fd', fontSize: 11, marginTop: 5 }}>FB {c.fbUserId}</p>}
            </div>
          ))}
        </div>
      )}
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

  const selectedWs = workspaces.find(w => w.accountId === selectedId);
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

  useEffect(() => {
    if (selectedId === null || !selectedWs?.running) {
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
  }, [selectedId, selectedWs?.running, hasSavedSession, humanRequired, autoSyncPaused]); // eslint-disable-line react-hooks/exhaustive-deps

  const running = workspaces.filter(w => w.running).length;

  const handleNewSession = async () => {
    setNewLoading(true);
    try {
      const id = await startNew();
      setSelectedId(id);
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
    const name = `Local Chrome ${new Date().toLocaleDateString('vi-VN')}`;
    const created = await createPairingCode(name, selectedId ?? undefined);
    setPairingCode(created.code);
    setPairingExpiresAt(created.expires_at);
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
        onCreate={() => void handleCreateConnector()}
      />

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
                  onClick={e => { e.stopPropagation(); void start(w.accountId).then(() => setSelectedId(w.accountId)); }}
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

      {selectedId !== null && selectedWs?.running && (
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
