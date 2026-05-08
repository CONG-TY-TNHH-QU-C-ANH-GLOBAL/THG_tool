import { useCallback, useRef, useState, type ClipboardEvent, type FormEvent, type KeyboardEvent, type MouseEvent, type WheelEvent } from 'react';
import { Laptop, Mail, Monitor, RefreshCw } from 'lucide-react';
import { sendConnectorInput } from '../../services/connectorsService';
import type { LocalConnectorAction, LocalConnectorScreen } from '../../types';
import { actionStatusTone, actionTime, actionTypeLabel, facebookIdentityLabel, formatLastSeen, isRemoteControlKey } from './browserHelpers';

export function LocalChromeViewer({
  screen,
  accountId,
  accountName,
  accountEmail,
  loading,
  onRefresh,
}: {
  screen: LocalConnectorScreen | null;
  accountId: number;
  accountName?: string;
  accountEmail?: string;
  loading: boolean;
  onRefresh: () => void;
}) {
  const imgRef = useRef<HTMLImageElement | null>(null);
  const surfaceRef = useRef<HTMLDivElement | null>(null);
  const keyboardRef = useRef<HTMLTextAreaElement | null>(null);
  const inputQueueRef = useRef<Promise<void>>(Promise.resolve());
  const lastWheelAtRef = useRef(0);
  const [inputStatus, setInputStatus] = useState<string | null>(null);
  const [inputActive, setInputActive] = useState(false);
  const age = screen?.updatedAt ? Math.max(0, Math.round((Date.now() - new Date(screen.updatedAt).getTime()) / 1000)) : null;
  const remoteInputEnabled = Boolean(screen?.imageData && screen.fbUserId && screen.streamStatus === 'facebook_logged_in');
  const screenIdentityLabel = facebookIdentityLabel({
    displayName: screen?.fbDisplayName,
    username: screen?.fbUsername,
    email: accountEmail,
    fbUserId: screen?.fbUserId,
  });

  const queueInput = useCallback((type: 'click' | 'key' | 'text' | 'scroll', payload: Record<string, unknown>) => {
    if (!screen?.imageData || !remoteInputEnabled) return;
    inputQueueRef.current = inputQueueRef.current
      .catch(() => undefined)
      .then(async () => {
        try {
          const res = await sendConnectorInput(accountId, type, payload);
          setInputStatus(`Đã gửi thao tác #${res.id}`);
        } catch (err) {
          setInputStatus(err instanceof Error ? err.message : 'Không gửi được thao tác đến Chrome Extension');
        }
      });
  }, [accountId, remoteInputEnabled, screen?.imageData]);

  const focusRemoteKeyboard = () => {
    window.setTimeout(() => {
      try {
        keyboardRef.current?.focus({ preventScroll: true });
      } catch {
        keyboardRef.current?.focus();
      }
    }, 0);
  };

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
    if (!remoteInputEnabled) {
      setInputStatus('Hãy đăng nhập Facebook trực tiếp trên Chrome đã cài extension.');
      return;
    }
    setInputActive(true);
    surfaceRef.current?.focus();
    focusRemoteKeyboard();
    const point = imagePoint(e.clientX, e.clientY);
    if (!point) return;
    void queueInput('click', {
      x: point.x,
      y: point.y,
      image_width: imgRef.current?.naturalWidth ?? 0,
      image_height: imgRef.current?.naturalHeight ?? 0,
      button: e.button === 2 ? 'right' : e.button === 1 ? 'middle' : 'left',
      clicks: Math.max(1, e.detail || 1),
    });
  };

  const handleKeyboardInput = (e: FormEvent<HTMLTextAreaElement>) => {
    if (!screen?.imageData || !remoteInputEnabled) return;
    const el = e.currentTarget;
    const text = el.value;
    if (!text) return;
    el.value = '';
    void queueInput('text', { text: text.slice(0, 256) });
  };

  const handleKeyboardPaste = (e: ClipboardEvent<HTMLTextAreaElement>) => {
    if (!screen?.imageData || !remoteInputEnabled) return;
    const text = e.clipboardData.getData('text');
    if (!text) return;
    e.preventDefault();
    void queueInput('text', { text: text.slice(0, 256) });
  };

  const handleKeyboardKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (!screen?.imageData || !remoteInputEnabled) return;
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'v') {
      return;
    }
    if (isRemoteControlKey(e.key) || e.ctrlKey || e.metaKey) {
      e.preventDefault();
      void queueInput('key', {
        key: e.key,
        code: e.code,
        ctrl_key: e.ctrlKey,
        alt_key: e.altKey,
        shift_key: e.shiftKey,
        meta_key: e.metaKey,
      });
    }
  };

  const handleWheel = (e: WheelEvent<HTMLImageElement>) => {
    if (!screen?.imageData || !remoteInputEnabled) return;
    const now = Date.now();
    if (now - lastWheelAtRef.current < 120) return;
    lastWheelAtRef.current = now;
    setInputActive(true);
    const point = imagePoint(e.clientX, e.clientY) ?? { x: 0, y: 0 };
    void queueInput('scroll', {
      x: point.x,
      y: point.y,
      image_width: imgRef.current?.naturalWidth ?? 0,
      image_height: imgRef.current?.naturalHeight ?? 0,
      delta_x: e.deltaX,
      delta_y: e.deltaY,
    });
  };

  return (
    <div className="card" style={{ padding: 0, overflow: 'hidden', background: 'var(--screen-bg)' }}>
      <header
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 'var(--s-3)',
          padding: 'var(--s-3) var(--s-4)',
          background: 'var(--bg-elev-2)',
          borderBottom: '1px solid var(--line)',
          flexWrap: 'wrap',
        }}
      >
        <span
          style={{
            width: 8,
            height: 8,
            borderRadius: '50%',
            background: screen?.imageData ? 'var(--ok)' : 'var(--text-faint)',
            flexShrink: 0,
          }}
        />
        <Monitor size={14} color="var(--accent)" />
        <div style={{ minWidth: 0, flex: 1 }}>
          <p style={{ margin: 0, color: 'var(--text)', fontSize: 13, fontWeight: 600 }}>
            Facebook thật{accountName ? ` · ${accountName}` : ''}
          </p>
          <p
            className="mono"
            style={{
              margin: 0,
              color: 'var(--text-mute)',
              fontSize: 11,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {screen?.currentUrl || 'Đang chờ Chrome Extension gửi stream Facebook...'}
          </p>
        </div>
        {!remoteInputEnabled && screen?.imageData && (
          <span className="tag tag-warm">Cần đăng nhập trên Chrome</span>
        )}
        {remoteInputEnabled && inputActive && <span className="tag tag-ok">Đang điều khiển</span>}
        {screenIdentityLabel && (
          <span
            title={screenIdentityLabel}
            className="tag tag-cold"
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: 4,
              maxWidth: 240,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            <Mail size={11} />
            {screenIdentityLabel}
          </span>
        )}
        {screen?.chromeError && (
          <span
            style={{
              color: 'var(--hot)',
              fontSize: 11,
              maxWidth: 240,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {screen.chromeError}
          </span>
        )}
        {inputStatus && (
          <span
            style={{
              color: 'var(--hot)',
              fontSize: 11,
              maxWidth: 280,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {inputStatus}
          </span>
        )}
        {age !== null && (
          <span style={{ color: age < 30 ? 'var(--ok)' : 'var(--warn)', fontSize: 11 }}>
            {age}s trước
          </span>
        )}
        <button type="button" className="btn btn-ghost btn-sm" onClick={onRefresh}>
          <RefreshCw size={12} className={loading ? 'spin' : ''} />
          Làm mới
        </button>
      </header>

      {screen?.actions && screen.actions.length > 0 && (
        <div
          style={{
            display: 'flex',
            gap: 'var(--s-2)',
            alignItems: 'center',
            overflowX: 'auto',
            padding: 'var(--s-2) var(--s-4)',
            background: 'var(--screen-bg)',
            borderBottom: '1px solid var(--line)',
          }}
        >
          <span className="field-label" style={{ whiteSpace: 'nowrap' }}>Automation</span>
          {screen.actions.slice(0, 5).map((action: LocalConnectorAction) => {
            const tone = actionStatusTone(action.status);
            return (
              <span
                key={action.id}
                title={action.errorMsg || `${actionTypeLabel(action.type)} #${action.id}`}
                className="tag"
                style={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 6,
                  border: `1px solid ${tone.border}`,
                  background: tone.bg,
                  color: tone.color,
                  whiteSpace: 'nowrap',
                }}
              >
                #{action.id} {actionTypeLabel(action.type)}
                <span style={{ opacity: 0.85 }}>{tone.label}</span>
                <span style={{ color: 'var(--text-faint)' }}>{formatLastSeen(actionTime(action))}</span>
              </span>
            );
          })}
        </div>
      )}

      <div
        ref={surfaceRef}
        tabIndex={0}
        style={{ position: 'relative', minHeight: 420, display: 'grid', placeItems: 'center', background: 'var(--screen-bg)', outline: 'none' }}
      >
        <textarea
          ref={keyboardRef}
          aria-label="Remote browser keyboard"
          autoCapitalize="off"
          autoComplete="off"
          autoCorrect="off"
          spellCheck={false}
          onInput={handleKeyboardInput}
          onPaste={handleKeyboardPaste}
          onKeyDown={handleKeyboardKeyDown}
          style={{
            position: 'absolute',
            width: 1,
            height: 1,
            opacity: 0,
            pointerEvents: 'none',
            resize: 'none',
          }}
        />
        {screen?.imageData ? (
          <div style={{ position: 'relative', width: '100%', lineHeight: 0 }}>
            <img
              ref={imgRef}
              src={screen.imageData}
              alt="Facebook stream"
              style={{ width: '100%', height: 'auto', display: 'block', background: 'var(--screen-bg)', userSelect: 'none' }}
            />
            <div
              aria-label="Remote browser control surface"
              onMouseDown={handlePointerDown}
              onWheel={handleWheel}
              onContextMenu={(e) => e.preventDefault()}
              style={{ position: 'absolute', inset: 0, cursor: remoteInputEnabled ? 'crosshair' : 'default', background: 'transparent' }}
            />
          </div>
        ) : (
          <div style={{ textAlign: 'center', padding: 'var(--s-8)', maxWidth: 520 }}>
            <Laptop size={34} color="var(--accent)" style={{ marginBottom: 'var(--s-3)' }} />
            <p style={{ margin: 0, color: 'var(--text)', fontSize: 14, fontWeight: 600 }}>
              Đang chờ Chrome Extension
            </p>
            <p style={{ margin: '6px 0 0', color: 'var(--text-mute)', fontSize: 12.5, lineHeight: 1.55 }}>
              Mở tab Facebook đã đăng nhập trong Chrome có cài extension. Dashboard sẽ nhận ảnh stream và action log từ tab Facebook thật ngay tại đây.
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
