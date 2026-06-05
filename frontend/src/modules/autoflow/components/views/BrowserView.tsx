import { useEffect, useState } from 'react';
import { AlertTriangle, CheckCircle2, LogIn, Mail, Monitor, Plus, RefreshCw, ShieldCheck, StopCircle } from 'lucide-react';
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
import { AccountPresenceBoard } from '../browser/AccountPresenceBoard';
import { facebookIdentityLabel, isDashboardStreamConnector, isUsableConnectorForAccount, stateLabel, stateTone } from '../browser/browserHelpers';
import '../../autoflow.css';

interface BrowserViewProps { orgId: string; }

function isErrorNotice(message: string): boolean {
  return /không|fail|error|lỗi/i.test(message);
}

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
      setBrowserNotice('Chrome của bạn chưa kết nối workspace. Cài Chrome Extension, tạo mã ghép nối, dán mã vào popup extension, rồi mở tab Facebook đã đăng nhập.');
      return;
    }
    setNewLoading(true);
    setBrowserNotice(null);
    try {
      const id = await startNew();
      setSelectedId(id);
      setBrowserNotice('Đã tạo phiên Facebook. Chrome Extension sẽ stream tab Facebook thật về dashboard — hãy giữ tab Facebook đã đăng nhập trong Chrome.');
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
    const name = `Chrome ${new Date().toLocaleDateString('vi-VN')}`;
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
      setConnectorNotice(`Đã ngắt kết nối ${connector.hostname || connector.name}. Extension trên Chrome đó sẽ bị từ chối ở lần đồng bộ kế tiếp.`);
    } catch (e) {
      setConnectorNotice(e instanceof Error ? e.message : 'Không ngắt được kết nối Chrome');
    } finally {
      setDisconnectingId(null);
    }
  };

  return (
    <div className="af-browser-shell" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--s-4)', padding: 'var(--s-4)' }}>
      <header style={{ display: 'flex', alignItems: 'flex-start', gap: 16 }}>
        <div>
          <div className="eyebrow"><span className="dot" />PHIÊN FACEBOOK</div>
          <h2 style={{ fontSize: 28, marginTop: 8 }}>Browser</h2>
          <p style={{ color: 'var(--text-mute)', fontSize: 13.5, marginTop: 6 }}>
            Phiên trình duyệt thật, có cookie. Mọi hành động được ghi log.
          </p>
        </div>
      </header>
      <AutomationCommandCenter
        workspaces={workspaces}
        connectors={connectors}
        actions={recentActions}
        running={running}
        loading={newLoading}
        onRefresh={() => { void refresh(); void refreshConnectors(); }}
        onNewSession={() => void handleNewSession()}
      />

      <div
        className="card"
        style={{
          display: 'flex',
          gap: 'var(--s-5)',
          padding: 'var(--s-3) var(--s-5)',
          alignItems: 'center',
          flexWrap: 'wrap',
        }}
      >
        <span style={{ color: 'var(--text-mute)', fontSize: 12 }}>
          Tài khoản: <strong className="tabular" style={{ color: 'var(--text)' }}>{workspaces.length}</strong>
        </span>
        <span style={{ color: 'var(--text-mute)', fontSize: 12 }}>
          Đang chạy: <strong className="tabular" style={{ color: running > 0 ? 'var(--ok)' : 'var(--text-faint)' }}>{running}</strong>
        </span>
        <span style={{ color: 'var(--text-mute)', fontSize: 12 }}>
          Extension online: <strong className="tabular" style={{ color: connectors.some(c => c.online) ? 'var(--ok)' : 'var(--text-faint)' }}>
            {connectors.filter(c => c.online).length}
          </strong>
        </span>
        <button type="button" className="btn btn-ghost btn-sm" onClick={() => { void refresh(); void refreshConnectors(); }} style={{ marginLeft: 'auto' }}>
          <RefreshCw size={13} /> Làm mới
        </button>
        <button type="button" className="btn btn-primary btn-sm" onClick={() => void handleNewSession()} disabled={newLoading}>
          {newLoading ? <RefreshCw size={13} className="spin" /> : <Plus size={13} />}
          {newLoading ? 'Đang khởi động...' : 'Phiên mới'}
        </button>
      </div>

      <AccountPresenceBoard />

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
        <Notice tone={isErrorNotice(connectorNotice) ? 'hot' : 'ok'}>{connectorNotice}</Notice>
      )}
      {browserNotice && (
        <Notice tone="warn">{browserNotice}</Notice>
      )}

      <div className="af-browser-account-list" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--s-2)' }}>
        {workspaces.length === 0 && (
          <CyberEmptyState onCreate={() => void handleNewSession()} loading={newLoading} />
        )}

        {workspaces.map(w => {
          const tone = stateTone(w.browserState);
          const isSelected = selectedId === w.accountId;
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
              className="card"
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 'var(--s-3)',
                padding: 'var(--s-3) var(--s-4)',
                background: isSelected ? 'var(--accent-soft)' : 'var(--bg-elev-2)',
                borderColor: isSelected ? 'var(--accent-glow)' : 'var(--line)',
                cursor: w.running ? 'pointer' : 'default',
              }}
            >
              <span
                style={{
                  width: 8,
                  height: 8,
                  borderRadius: '50%',
                  flexShrink: 0,
                  background: w.running ? 'var(--ok)' : 'var(--text-faint)',
                }}
              />
              <Monitor size={14} color="var(--text-mute)" />
              <span style={{ flex: 1, color: 'var(--text)', fontWeight: 500, fontSize: 13 }}>
                {identityLabel || w.accountName}
              </span>
              {w.loggedIn && (
                <span className="tag tag-cold" style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                  <CheckCircle2 size={11} />Đã đăng nhập
                </span>
              )}
              {w.fbUserId && !w.fbDisplayName && !w.fbUsername && !w.email && (
                <span className="tag tag-mute mono">FB {w.fbUserId}</span>
              )}
              {w.email && (
                <span
                  title={w.email}
                  className="tag tag-cold"
                  style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: 4,
                    maxWidth: 220,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                  }}
                >
                  <Mail size={11} /> {w.email}
                </span>
              )}
              {w.browserState && (
                <span
                  className="tag"
                  style={{
                    background: tone.bg,
                    color: tone.color,
                    border: `1px solid ${tone.border}`,
                  }}
                >
                  {stateLabel(w.browserState)}
                </span>
              )}
              {!w.running ? (
                <button
                  type="button"
                  onClick={e => {
                    e.stopPropagation();
                    setBrowserNotice(null);
                    if (!rowHasOnlineConnector) {
                      setBrowserNotice('Account này cần một Chrome Extension online. Tạo mã ghép nối riêng cho account này, dán vào popup extension, rồi mở tab Facebook đã đăng nhập.');
                      return;
                    }
                    void start(w.accountId)
                      .then(() => {
                        setSelectedId(w.accountId);
                        setBrowserNotice('Đang yêu cầu Chrome Extension stream tab Facebook thật về dashboard.');
                        void refreshLocalScreen(w.accountId);
                      })
                      .catch(err => setBrowserNotice(err instanceof Error ? err.message : 'Không kết nối được tab Facebook'));
                  }}
                  disabled={actionLoading.has(w.accountId)}
                  className={`btn btn-sm ${rowHasOnlineConnector ? 'btn-primary' : 'btn-ghost'}`}
                >
                  {actionLoading.has(w.accountId) ? <RefreshCw size={12} className="spin" /> : <LogIn size={12} />}
                  {actionLoading.has(w.accountId)
                    ? rowHasOnlineConnector ? 'Đang kết nối...' : 'Đang kiểm tra...'
                    : rowHasOnlineConnector ? 'Bắt đầu stream' : 'Chưa sẵn sàng'}
                </button>
              ) : (
                <button
                  type="button"
                  className="btn btn-ghost btn-sm"
                  style={{ color: 'var(--hot)' }}
                  onClick={e => {
                    e.stopPropagation();
                    void stop(w.accountId).then(() => {
                      if (selectedId === w.accountId) setSelectedId(null);
                    });
                  }}
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
        <div className="card" style={{ padding: 0, overflow: 'hidden', background: 'var(--screen-bg)' }}>
          <header
            style={{
              display: 'grid',
              gridTemplateColumns: 'repeat(4, minmax(0, 1fr)) auto',
              gap: 'var(--s-3)',
              alignItems: 'center',
              padding: 'var(--s-3) var(--s-4)',
              background: 'var(--bg-elev-2)',
              borderBottom: '1px solid var(--line)',
            }}
          >
            <SessionStat label="Session" tone={humanRequired ? 'warn' : (sessionInfo?.loggedIn || selectedWs.loggedIn ? 'ok' : 'mute')}>
              {humanRequired ? <AlertTriangle size={12} /> : <ShieldCheck size={12} />}
              {humanRequired ? 'Cần xác minh' : (sessionInfo?.loggedIn || selectedWs.loggedIn ? 'Đã lưu' : 'Chưa xác thực')}
            </SessionStat>
            <SessionStat label="Facebook ID">
              <span className="mono">{sessionInfo?.fbUserId || selectedWs.fbUserId || '—'}</span>
            </SessionStat>
            <SessionStat label="URL" muted>
              {sessionInfo?.currentUrl || '—'}
            </SessionStat>
            <SessionStat label="CDP" tone={syncError || sessionInfo?.cookieError ? 'hot' : 'mute'}>
              {humanRequired
                ? sessionInfo?.humanReason || 'human_required'
                : syncError || sessionInfo?.cookieError || (syncLoading ? 'đang đồng bộ' : 'sẵn sàng')}
            </SessionStat>
            <button
              type="button"
              className={`btn btn-sm ${manualCaptureMode ? 'btn-primary' : 'btn-ghost'}`}
              onClick={() => void handleManualSync()}
              disabled={syncLoading}
              style={{ whiteSpace: 'nowrap' }}
            >
              <RefreshCw size={12} className={syncLoading ? 'spin' : ''} />
              {manualCaptureMode ? 'Đã đăng nhập — đồng bộ' : 'Đồng bộ'}
            </button>
          </header>

          <VncCanvas
            accountId={selectedId}
            accountName={selectedWs.accountName}
            cdpPort={selectedWs.cdpPort}
            vncPort={selectedWs.vncPort}
            errorMsg={selectedWs.errorMsg}
          />

          <footer
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 'var(--s-3)',
              padding: 'var(--s-2) var(--s-4)',
              background: 'var(--bg-elev)',
              borderTop: '1px solid var(--line)',
              fontSize: 12,
            }}
          >
            {manualCaptureMode ? (
              <span style={{ color: 'var(--warn)', display: 'flex', alignItems: 'center', gap: 6 }}>
                <AlertTriangle size={13} /> Auto-sync tạm dừng — đăng nhập, vào News Feed rồi bấm đồng bộ.
              </span>
            ) : selectedWs.loggedIn ? (
              <span style={{ color: 'var(--ok)', display: 'flex', alignItems: 'center', gap: 6 }}>
                <CheckCircle2 size={13} /> Session đã được lưu.
              </span>
            ) : (
              <span style={{ color: 'var(--text-mute)', display: 'flex', alignItems: 'center', gap: 6 }}>
                <RefreshCw size={13} className={syncLoading ? 'spin' : ''} /> Đang tự xác thực session.
              </span>
            )}
          </footer>
        </div>
      )}
    </div>
  );
}

function Notice({ tone, children }: { tone: 'ok' | 'hot' | 'warn'; children: React.ReactNode }) {
  const iconMap = {
    ok: <CheckCircle2 size={16} color="var(--ok)" />,
    hot: <AlertTriangle size={16} color="var(--hot)" />,
    warn: <AlertTriangle size={16} color="var(--warn)" />,
  };
  return (
    <div className={`banner banner-${tone}`}>
      {iconMap[tone]}
      <div>
        <div style={{ fontSize: 13.5, fontWeight: 500, color: 'var(--text)', marginBottom: 4 }}>
          Thông báo
        </div>
        <div style={{ fontSize: 12.5, color: 'var(--text-mute)' }}>
          {children}
        </div>
      </div>
    </div>
  );
}

function SessionStat({
  label,
  tone,
  muted,
  children,
}: {
  label: string;
  tone?: 'ok' | 'warn' | 'hot' | 'mute';
  muted?: boolean;
  children: React.ReactNode;
}) {
  const colorByTone: Record<NonNullable<typeof tone>, string> = {
    ok: 'var(--ok)',
    warn: 'var(--warn)',
    hot: 'var(--hot)',
    mute: 'var(--text-mute)',
  };
  const valueColor = tone ? colorByTone[tone] : muted ? 'var(--text-mute)' : 'var(--text)';
  return (
    <div style={{ minWidth: 0 }}>
      <p className="field-label" style={{ marginBottom: 4 }}>{label}</p>
      <p
        style={{
          margin: 0,
          color: valueColor,
          fontSize: 12,
          fontWeight: 500,
          display: 'flex',
          alignItems: 'center',
          gap: 5,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
        }}
      >
        {children}
      </p>
    </div>
  );
}
