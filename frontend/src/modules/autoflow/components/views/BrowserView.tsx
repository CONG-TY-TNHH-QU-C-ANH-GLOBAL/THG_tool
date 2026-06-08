import { useEffect, useState } from 'react';
import { AlertTriangle, CheckCircle2, LogIn, Mail, Monitor, Plus, RefreshCw, StopCircle } from 'lucide-react';
import { useWorkspaces } from '../../hooks/useWorkspaces';
import { useConnectors } from '../../hooks/useConnectors';
import { useAuthStore } from '../../stores/authStore';
import { getSystemInfo, type SystemInfo } from '../../services/systemService';
import { disconnectLocalConnector } from '../../services/connectorsService';
import type { LocalConnector } from '../../types';
import { AutomationCommandCenter } from '../browser/AutomationCommandCenter';
import { CyberEmptyState } from '../browser/CyberEmptyState';
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
  const { workspaces, actionLoading, refresh, start, startNew, stop } = useWorkspaces();
  const { connectors, creating: connectorCreating, refresh: refreshConnectors, createPairingCode } = useConnectors();
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [newLoading, setNewLoading] = useState(false);
  const [pairingCode, setPairingCode] = useState('');
  const [pairingExpiresAt, setPairingExpiresAt] = useState('');
  const [systemInfo, setSystemInfo] = useState<SystemInfo | null>(null);
  const [disconnectingId, setDisconnectingId] = useState<number | null>(null);
  const [connectorNotice, setConnectorNotice] = useState<string | null>(null);
  const [browserNotice, setBrowserNotice] = useState<string | null>(null);

  useEffect(() => {
    setSelectedId(null);
    setBrowserNotice(null);
  }, [orgId]);

  useEffect(() => {
    if (selectedId !== null && workspaces.find(w => w.accountId === selectedId)?.running) return;
    const firstRunning = workspaces.find(w => w.running);
    setSelectedId(firstRunning?.accountId ?? null);
  }, [selectedId, workspaces]);

  useEffect(() => {
    if (newLoading && workspaces.some(w => w.running)) {
      setNewLoading(false);
    }
  }, [newLoading, workspaces]);

  useEffect(() => {
    getSystemInfo().then(setSystemInfo).catch(() => setSystemInfo(null));
  }, []);

  const running = workspaces.filter(w => w.running).length;
  const currentUserId = currentUser?.id ?? 0;

  // A connector YOU paired + currently online — required before starting a session.
  const ownOnlineConnectors = connectors.filter(c => c.createdBy === currentUserId && c.online);

  const handleNewSession = async () => {
    if (ownOnlineConnectors.length === 0) {
      setBrowserNotice('Chrome của bạn chưa kết nối workspace. Cài Chrome Extension, tạo mã ghép nối, dán mã vào popup extension, rồi mở tab Facebook đã đăng nhập.');
      return;
    }
    setNewLoading(true);
    setBrowserNotice(null);
    try {
      const id = await startNew();
      setSelectedId(id);
      setBrowserNotice('Đã tạo phiên Facebook. Giữ tab Facebook đã đăng nhập trong Chrome — extension sẽ tự động chạy nhiệm vụ.');
    } catch (e) {
      setBrowserNotice(e instanceof Error ? e.message : 'Không tạo được phiên mới');
    } finally {
      setNewLoading(false);
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
        actions={[]}
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
          // "Your device" = a connector YOU paired drives this account's session.
          // This is what lets the member pick their own row out of the workspace
          // list when the FB-scraped names are ambiguous/garbage.
          const isMine = connectors.some(
            (c) => c.createdBy === currentUserId && isDashboardStreamConnector(c) &&
              (c.assignedAccountId === w.accountId || c.assignedAccountId === 0),
          );
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
              <span style={{ color: 'var(--text)', fontWeight: 500, fontSize: 13 }}>
                {identityLabel || `Tài khoản #${w.accountId}`}
              </span>
              {isMine && (
                <span className="tag tag-ok" style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
                  <Monitor size={11} /> Thiết bị của bạn
                </span>
              )}
              <span style={{ flex: 1 }} />
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
                        setBrowserNotice('Đã kết nối tab Facebook. Extension sẽ tự động chạy nhiệm vụ.');
                      })
                      .catch(err => setBrowserNotice(err instanceof Error ? err.message : 'Không kết nối được tab Facebook'));
                  }}
                  disabled={actionLoading.has(w.accountId)}
                  className={`btn btn-sm ${rowHasOnlineConnector ? 'btn-primary' : 'btn-ghost'}`}
                >
                  {actionLoading.has(w.accountId) ? <RefreshCw size={12} className="spin" /> : <LogIn size={12} />}
                  {actionLoading.has(w.accountId)
                    ? rowHasOnlineConnector ? 'Đang kết nối...' : 'Đang kiểm tra...'
                    : rowHasOnlineConnector ? 'Bắt đầu' : 'Chưa sẵn sàng'}
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
