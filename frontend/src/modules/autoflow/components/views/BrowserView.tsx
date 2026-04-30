import { useEffect, useState } from 'react';
import { theme } from '../../constants/styles';
import { useWorkspaces } from '../../hooks/useWorkspaces';
import type { WorkspaceSessionSnapshot } from '../../types';
import { ArrowRight, Cpu, Monitor, StopCircle, LogIn, RefreshCw, CheckCircle, Plus, ShieldCheck } from 'lucide-react';
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
    case 'error': return 'error';
    default: return state || '';
  }
}

function stateTone(state?: string) {
  if (state === 'error') return { color: '#fca5a5', bg: '#7f1d1d55', border: '#ef444466' };
  if (state === 'initializing') return { color: '#fde68a', bg: '#78350f44', border: '#f59e0b55' };
  return { color: '#a7f3d0', bg: '#064e3b44', border: '#10b98155' };
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
        <p style={{ color: '#67e8f9', fontSize: 11, fontWeight: 800, letterSpacing: '0.14em', marginBottom: 8 }}>CYBERTECH SIGNAL</p>
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
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [newLoading, setNewLoading] = useState(false);
  const [sessionInfo, setSessionInfo] = useState<WorkspaceSessionSnapshot | null>(null);
  const [syncLoading, setSyncLoading] = useState(false);
  const [syncError, setSyncError] = useState<string | null>(null);

  const selectedWs = workspaces.find(w => w.accountId === selectedId);

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
    if (selectedId === null || !selectedWs?.running) {
      setSessionInfo(null);
      setSyncError(null);
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
        }
      } catch (e) {
        if (!cancelled) {
          setSyncError(e instanceof Error ? e.message : 'Không đồng bộ được session');
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
  }, [selectedId, selectedWs?.running]); // eslint-disable-line react-hooks/exhaustive-deps

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
    } catch (e) {
      setSyncError(e instanceof Error ? e.message : 'Không đồng bộ được session');
    } finally {
      setSyncLoading(false);
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <div style={{ display: 'flex', gap: 16, padding: '8px 14px', background: theme.surface, borderRadius: 10, border: `1px solid ${theme.border}`, alignItems: 'center' }}>
        <span style={{ color: theme.textMuted, fontSize: 12 }}>Tài khoản: <strong style={{ color: theme.text }}>{workspaces.length}</strong></span>
        <span style={{ color: theme.textMuted, fontSize: 12 }}>Đang chạy: <strong style={{ color: running > 0 ? '#4ade80' : theme.textFaint }}>{running}</strong></span>
        <button onClick={refresh} style={{ marginLeft: 'auto', background: 'none', border: 'none', color: theme.textFaint, cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 4, fontSize: 12 }}>
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
              <p style={{ color: sessionInfo?.loggedIn || selectedWs.loggedIn ? '#4ade80' : theme.textMuted, fontSize: 12, fontWeight: 600, display: 'flex', alignItems: 'center', gap: 5 }}>
                <ShieldCheck size={12} /> {sessionInfo?.loggedIn || selectedWs.loggedIn ? 'Đã lưu' : 'Chưa xác thực'}
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
                {syncError || sessionInfo?.cookieError || (syncLoading ? 'đang đồng bộ' : 'sẵn sàng')}
              </p>
            </div>
            <button
              onClick={() => void handleManualSync()}
              disabled={syncLoading}
              style={{ display: 'flex', alignItems: 'center', gap: 5, padding: '6px 10px', background: 'transparent', border: `1px solid ${theme.border}`, borderRadius: 8, color: theme.textMuted, fontSize: 12, cursor: syncLoading ? 'wait' : 'pointer', whiteSpace: 'nowrap' }}
            >
              <RefreshCw size={12} className={syncLoading ? 'spin' : ''} /> Đồng bộ
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
            {selectedWs.loggedIn ? (
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
