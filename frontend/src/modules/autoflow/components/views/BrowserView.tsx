import { useEffect, useState } from 'react';
import { AlertTriangle, CheckCircle, LogIn, Mail, Monitor, Plus, RefreshCw, ShieldCheck, StopCircle } from 'lucide-react';
import { theme } from '../../constants/styles';
import { useWorkspaces } from '../../hooks/useWorkspaces';
import { useConnectors } from '../../hooks/useConnectors';
import { useAuthStore } from '../../stores/authStore';
import { getSystemInfo, type SystemInfo } from '../../services/systemService';
import { disconnectLocalConnector, getLocalConnectorScreen } from '../../services/connectorsService';
import type { LocalConnector, LocalConnectorScreen, WorkspaceSessionSnapshot } from '../../types';
import VncCanvas from '../VncCanvas';
import { AutomationCommandCenter } from '../browser/AutomationCommandCenter';
import { CyberEmptyState } from '../browser/CyberEmptyState';
import { LocalChromeViewer } from '../browser/LocalChromeViewer';
import { LocalConnectorPanel } from '../browser/LocalConnectorPanel';
import { facebookIdentityLabel, isDashboardStreamConnector, isUsableConnectorForAccount, stateLabel, stateTone } from '../browser/browserHelpers';
import '../../autoflow.css';

interface BrowserViewProps { orgId: string; }

export default function BrowserView({ orgId }: BrowserViewProps) {
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

  useEffect(() => {
    setSelectedId(null);
    setSessionInfo(null);
    setLocalScreen(null);
    setSyncError(null);
    setBrowserNotice(null);
  }, [orgId]);

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
  const currentUserId = currentUser?.id ?? 0;
  const recentActions = localScreen?.actions ?? [];

  const ownStreamingConnectors = connectors.filter(c => c.createdBy === currentUserId && c.online && isDashboardStreamConnector(c));

  const handleNewSession = async () => {
    if (ownStreamingConnectors.length === 0) {
      setBrowserNotice('Chrome của bạn chưa kết nối workspace. Cài THG Chrome Extension, tạo mã kết nối, dán mã vào popup extension, rồi mở tab Facebook đã đăng nhập.');
      return;
    }
    setNewLoading(true);
    setBrowserNotice(null);
    try {
      const id = await startNew();
      setSelectedId(id);
      setBrowserNotice('Đã tạo phiên Facebook. THG Chrome Extension sẽ stream tab Facebook thật về Browser dashboard; hãy giữ tab Facebook đã đăng nhập trong Chrome.');
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
    const name = `THG Chrome ${new Date().toLocaleDateString('vi-VN')}`;
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
      setConnectorNotice(`Đã disconnect ${connector.hostname || connector.name}. Extension trên Chrome đó sẽ bị từ chối ở lần đồng bộ kế tiếp.`);
    } catch (e) {
      setConnectorNotice(e instanceof Error ? e.message : 'Không disconnect được Chrome');
    } finally {
      setDisconnectingId(null);
    }
  };

  return (
    <div className="af-browser-shell" style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <AutomationCommandCenter
        workspaces={workspaces}
        connectors={connectors}
        actions={recentActions}
        running={running}
        loading={newLoading}
        onRefresh={() => { void refresh(); void refreshConnectors(); }}
        onNewSession={() => void handleNewSession()}
      />

      <div className="af-browser-legacy-summary" style={{ display: 'flex', gap: 16, padding: '8px 14px', background: theme.surface, borderRadius: 10, border: `1px solid ${theme.border}`, alignItems: 'center' }}>
        <span style={{ color: theme.textMuted, fontSize: 12 }}>Tài khoản: <strong style={{ color: theme.text }}>{workspaces.length}</strong></span>
        <span style={{ color: theme.textMuted, fontSize: 12 }}>Đang chạy: <strong style={{ color: running > 0 ? '#4ade80' : theme.textFaint }}>{running}</strong></span>
        <span style={{ color: theme.textMuted, fontSize: 12 }}>Extension: <strong style={{ color: connectors.some(c => c.online) ? '#4ade80' : theme.textFaint }}>{connectors.filter(c => c.online).length}</strong></span>
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
        currentUserId={currentUserId}
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

      <div className="af-browser-account-list" style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        {workspaces.length === 0 && (
          <CyberEmptyState onCreate={() => void handleNewSession()} loading={newLoading} />
        )}

        {workspaces.map(w => {
          const tone = stateTone(w.browserState);
          const rowHasOnlineConnector = connectors.some(c => isUsableConnectorForAccount(c, currentUserId, w.accountId));
          const identityLabel = facebookIdentityLabel({
            displayName: w.fbDisplayName,
            username: w.fbUsername,
            email: w.email,
            fbUserId: w.fbUserId,
            fallback: w.accountName,
          });
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
              <span style={{ flex: 1, color: theme.text, fontWeight: 500, fontSize: 13 }}>{identityLabel || w.accountName}</span>
              {w.loggedIn && (
                <span style={{ fontSize: 11, color: '#60a5fa', background: '#1e3a5f33', border: '1px solid #3b82f644', padding: '2px 8px', borderRadius: 6, display: 'inline-flex', alignItems: 'center', gap: 3 }}>
                  <CheckCircle size={10} />Đã đăng nhập
                </span>
              )}
              {w.fbUserId && !w.fbDisplayName && !w.fbUsername && !w.email && (
                <span style={{ fontSize: 11, color: '#c4b5fd', background: '#312e8133', border: '1px solid #6366f144', padding: '2px 8px', borderRadius: 6 }}>
                  FB {w.fbUserId}
                </span>
              )}
              {w.email && (
                <span title={w.email} style={{ fontSize: 11, color: '#bfdbfe', background: '#1e3a8a33', border: '1px solid #3b82f644', padding: '2px 8px', borderRadius: 6, display: 'inline-flex', alignItems: 'center', gap: 4, maxWidth: 210, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  <Mail size={10} /> {w.email}
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
                    if (!rowHasOnlineConnector) {
                      setBrowserNotice('Account này cần một THG Chrome Extension online. Tạo mã kết nối riêng cho account này, dán vào popup extension, rồi mở tab Facebook đã đăng nhập.');
                      return;
                    }
                    void start(w.accountId)
                      .then(() => {
                        setSelectedId(w.accountId);
                        setBrowserNotice('Đang yêu cầu THG Chrome Extension stream tab Facebook thật về dashboard.');
                        void refreshLocalScreen(w.accountId);
                      })
                      .catch(err => setBrowserNotice(err instanceof Error ? err.message : 'Không kết nối được tab Facebook'));
                  }}
                  disabled={actionLoading.has(w.accountId)}
                  style={{ display: 'flex', alignItems: 'center', gap: 5, padding: '5px 12px', background: rowHasOnlineConnector ? '#16a34a' : '#78350f', border: 'none', borderRadius: 7, color: rowHasOnlineConnector ? '#fff' : '#fcd34d', fontSize: 12, cursor: actionLoading.has(w.accountId) ? 'wait' : 'pointer', opacity: actionLoading.has(w.accountId) ? 0.6 : 1 }}
                >
                  {actionLoading.has(w.accountId) ? <RefreshCw size={12} className="spin" /> : <LogIn size={12} />}
                  {actionLoading.has(w.accountId) ? (rowHasOnlineConnector ? 'Đang kết nối...' : 'Đang kiểm tra...') : (rowHasOnlineConnector ? 'Bắt đầu stream' : 'Chưa sẵn sàng')}
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
          accountName={facebookIdentityLabel({
            displayName: selectedWs.fbDisplayName || localScreen?.fbDisplayName,
            username: selectedWs.fbUsername || localScreen?.fbUsername,
            email: selectedWs.email,
            fbUserId: selectedWs.fbUserId || localScreen?.fbUserId,
            fallback: selectedWs.accountName,
          })}
          accountEmail={selectedWs.email}
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
