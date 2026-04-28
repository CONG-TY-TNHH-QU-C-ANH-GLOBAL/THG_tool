import { useEffect, useRef, useState, useCallback } from 'react';
import { theme } from '../../constants/styles';
import { useWorkspaces } from '../../hooks/useWorkspaces';
import { useAuthStore } from '../../stores/authStore';
import { Monitor, StopCircle, LogIn, RefreshCw, CheckCircle, Plus } from 'lucide-react';
import '../../autoflow.css';

interface BrowserViewProps { orgId: string; }

type WsStatus = 'disconnected' | 'connecting' | 'connected';

export default function BrowserView({ orgId }: BrowserViewProps) {
  void orgId;
  const { workspaces, actionLoading, refresh, start, startNew, stop, markLoggedIn } = useWorkspaces();
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [wsStatus, setWsStatus] = useState<WsStatus>('disconnected');
  const [wsError, setWsError] = useState<string | null>(null);
  const [newLoading, setNewLoading] = useState(false);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const wsRef = useRef<WebSocket | null>(null);

  const selectedWs = workspaces.find(w => w.accountId === selectedId);

  // Open/close screen WebSocket when selection changes
  useEffect(() => {
    wsRef.current?.close();
    wsRef.current = null;
    setWsStatus('disconnected');
    setWsError(null);

    if (selectedId === null || !selectedWs?.running) return;

    const token = useAuthStore.getState().token ?? '';
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(`${proto}//${window.location.host}/ws/screen/${selectedId}?token=${token}`);

    setWsStatus('connecting');
    ws.onopen = () => setWsStatus('connected');
    ws.onclose = () => setWsStatus('disconnected');
    ws.onerror = () => setWsStatus('disconnected');

    ws.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data as string);
        if (msg.type === 'error') {
          setWsError(msg.msg as string);
        } else if (msg.type === 'frame' && canvasRef.current) {
          setWsError(null);
          const ctx = canvasRef.current.getContext('2d');
          if (!ctx) return;
          const img = new Image();
          img.onload = () => {
            if (!canvasRef.current) return;
            canvasRef.current.width = msg.w as number;
            canvasRef.current.height = msg.h as number;
            ctx.drawImage(img, 0, 0);
          };
          img.src = `data:image/jpeg;base64,${msg.data as string}`;
        }
      } catch { /* ignore parse errors */ }
    };

    wsRef.current = ws;
    return () => { ws.close(); wsRef.current = null; };
  }, [selectedId, selectedWs?.running]);

  // Keyboard forwarding
  useEffect(() => {
    if (selectedId === null) return;
    const send = (action: string) => (e: KeyboardEvent) => {
      if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return;
      e.preventDefault();
      wsRef.current.send(JSON.stringify({
        type: 'key', action,
        key: e.key, code: e.code,
        modifiers: (e.altKey ? 1 : 0) | (e.ctrlKey ? 2 : 0) | (e.metaKey ? 4 : 0) | (e.shiftKey ? 8 : 0),
      }));
    };
    const kd = send('keyDown');
    const ku = send('keyUp');
    document.addEventListener('keydown', kd);
    document.addEventListener('keyup', ku);
    return () => { document.removeEventListener('keydown', kd); document.removeEventListener('keyup', ku); };
  }, [selectedId]);

  const sendMouse = useCallback((action: string) => (e: React.MouseEvent<HTMLCanvasElement>) => {
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return;
    const rect = e.currentTarget.getBoundingClientRect();
    wsRef.current.send(JSON.stringify({
      type: 'mouse', action,
      x: e.clientX - rect.left,
      y: e.clientY - rect.top,
      button: e.button === 0 ? 'left' : e.button === 2 ? 'right' : 'middle',
      buttons: e.buttons,
    }));
  }, []);

  // Wheel must be non-passive to allow preventDefault; attach manually via useEffect.
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || selectedId === null || !selectedWs?.running) return;
    const handler = (e: WheelEvent) => {
      e.preventDefault();
      wsRef.current?.send(JSON.stringify({ type: 'wheel', x: e.clientX, y: e.clientY, deltaX: e.deltaX, deltaY: e.deltaY }));
    };
    canvas.addEventListener('wheel', handler, { passive: false });
    return () => canvas.removeEventListener('wheel', handler);
  }, [selectedId, selectedWs?.running]);

  const running = workspaces.filter(w => w.running).length;

  const handleNewSession = async () => {
    setNewLoading(true);
    try {
      const id = await startNew();
      setSelectedId(id);
    } catch { /* error shown via canvas wsError */ } finally { setNewLoading(false); }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>

      {/* Status bar */}
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

      {/* Account list */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        {workspaces.length === 0 && !newLoading && (
          <div style={{ textAlign: 'center', padding: 40 }}>
            <p style={{ color: theme.textMuted, fontSize: 13, marginBottom: 16 }}>Chưa có phiên Facebook nào</p>
            <button
              onClick={() => void handleNewSession()}
              style={{ display: 'inline-flex', alignItems: 'center', gap: 8, padding: '10px 24px', background: '#16a34a', border: 'none', borderRadius: 10, color: '#fff', fontSize: 14, fontWeight: 600, cursor: 'pointer' }}
            >
              <Plus size={16} /> Bắt đầu phiên Facebook mới
            </button>
          </div>
        )}
        {workspaces.map(w => (
          <div
            key={w.accountId}
            onClick={() => w.running && setSelectedId(prev => prev === w.accountId ? null : w.accountId)}
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
              <span style={{ fontSize: 11, color: '#60a5fa', background: '#1e3a5f33', border: '1px solid #3b82f644', padding: '2px 8px', borderRadius: 6 }}>
                <CheckCircle size={10} style={{ marginRight: 3 }} />Đã đăng nhập
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
        ))}
      </div>

      {/* Live screen viewer */}
      {selectedId !== null && selectedWs?.running && (
        <div style={{ background: '#000', borderRadius: 12, overflow: 'hidden', border: `1px solid ${theme.border}` }}>
          {/* Title bar */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '7px 12px', background: theme.surfaceAlt, borderBottom: `1px solid ${theme.border}` }}>
            <div style={{ display: 'flex', gap: 5 }}>
              {['#ef4444', '#f59e0b', '#22c55e'].map(c => <div key={c} style={{ width: 10, height: 10, borderRadius: '50%', background: c }} />)}
            </div>
            <span style={{ color: theme.textFaint, fontSize: 12, flex: 1 }}>🔒 facebook.com — {selectedWs.accountName}</span>
            <span style={{ fontSize: 11, color: wsStatus === 'connected' ? '#4ade80' : '#f59e0b' }}>
              {wsStatus === 'connected' ? '● Đã kết nối' : wsStatus === 'connecting' ? '● Đang kết nối...' : '● Mất kết nối'}
            </span>
          </div>

          {/* Canvas / Error */}
          {wsError ? (
            <div style={{ padding: 24, textAlign: 'center', color: '#fca5a5', fontSize: 13 }}>
              ⚠ {wsError}
            </div>
          ) : (
            <canvas
              ref={canvasRef}
              style={{ display: 'block', width: '100%', cursor: 'crosshair' }}
              tabIndex={0}
              onMouseMove={sendMouse('mouseMoved')}
              onMouseDown={sendMouse('mousePressed')}
              onMouseUp={sendMouse('mouseReleased')}
              onContextMenu={e => e.preventDefault()}
            />
          )}

          {/* Toolbar */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '8px 14px', background: theme.surfaceAlt, borderTop: `1px solid ${theme.border}` }}>
            {!selectedWs.loggedIn && (
              <button
                onClick={() => void markLoggedIn(selectedId)}
                style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '6px 14px', background: '#16a34a', border: 'none', borderRadius: 8, color: '#fff', fontSize: 13, fontWeight: 600, cursor: 'pointer' }}
              >
                <CheckCircle size={14} /> Đánh dấu đã đăng nhập
              </button>
            )}
            {selectedWs.loggedIn && (
              <span style={{ color: '#4ade80', fontSize: 12, display: 'flex', alignItems: 'center', gap: 5 }}>
                <CheckCircle size={13} /> Session đã được lưu
              </span>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
