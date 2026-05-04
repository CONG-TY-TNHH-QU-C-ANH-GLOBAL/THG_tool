import { useCallback, useRef, useState, type ClipboardEvent, type FormEvent, type KeyboardEvent, type MouseEvent, type WheelEvent } from 'react';
import { Laptop, Mail, Monitor, RefreshCw } from 'lucide-react';
import { theme } from '../../constants/styles';
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
          setInputStatus(`ÄÃ£ gá»­i thao tÃ¡c dá»± phÃ²ng #${res.id}`);
        } catch (err) {
          setInputStatus(err instanceof Error ? err.message : 'KhÃ´ng gá»­i Ä‘Æ°á»£c thao tÃ¡c Ä‘áº¿n THG Local Runtime');
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
      setInputStatus('HÃ£y Ä‘Äƒng nháº­p trá»±c tiáº¿p trong cá»­a sá»• Chrome local trÃªn mÃ¡y nhÃ¢n viÃªn');
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
    <div className="af-live-browser-frame" style={{ background: '#020617', borderRadius: 12, overflow: 'hidden', border: `1px solid ${theme.border}` }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '10px 12px', background: theme.surface, borderBottom: `1px solid ${theme.border}` }}>
        <span style={{ width: 8, height: 8, borderRadius: '50%', background: screen?.imageData ? '#4ade80' : theme.textFaint }} />
        <Monitor size={14} color="#5eead4" />
        <div style={{ minWidth: 0, flex: 1 }}>
          <p style={{ color: theme.text, fontSize: 13, fontWeight: 800 }}>Chrome tháº­t {accountName ? `- ${accountName}` : ''}</p>
          <p style={{ color: theme.textMuted, fontSize: 11, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {screen?.currentUrl || 'Äang chá» Chrome local má»Ÿ Facebook trÃªn mÃ¡y nhÃ¢n viÃªn'}
          </p>
        </div>
        {!remoteInputEnabled && screen?.imageData && <span style={{ color: '#fcd34d', border: '1px solid #f59e0b55', background: '#78350f33', borderRadius: 6, padding: '3px 8px', fontSize: 11 }}>login trÃªn Chrome local</span>}
        {remoteInputEnabled && inputActive && <span style={{ color: '#5eead4', border: '1px solid #14b8a644', background: '#134e4a33', borderRadius: 6, padding: '3px 8px', fontSize: 11 }}>remote fallback</span>}
        {screenIdentityLabel && <span title={screenIdentityLabel} style={{ color: '#bfdbfe', border: '1px solid #3b82f644', background: '#1e3a8a33', borderRadius: 6, padding: '3px 8px', fontSize: 11, display: 'inline-flex', alignItems: 'center', gap: 4, maxWidth: 240, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}><Mail size={11} />{screenIdentityLabel}</span>}
        {screen?.chromeError && <span style={{ color: '#fca5a5', fontSize: 11, maxWidth: 240, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{screen.chromeError}</span>}
        {inputStatus && <span style={{ color: '#fca5a5', fontSize: 11, maxWidth: 280, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{inputStatus}</span>}
        {age !== null && <span style={{ color: age < 30 ? '#86efac' : '#fcd34d', fontSize: 11 }}>{age}s trÆ°á»›c</span>}
        <button onClick={onRefresh} style={{ display: 'inline-flex', alignItems: 'center', gap: 5, padding: '6px 10px', background: 'transparent', border: `1px solid ${theme.border}`, borderRadius: 8, color: theme.textMuted, fontSize: 12, cursor: 'pointer' }}>
          <RefreshCw size={12} className={loading ? 'spin' : ''} /> LÃ m má»›i
        </button>
      </div>
      {screen?.actions && screen.actions.length > 0 && (
        <div style={{ display: 'flex', gap: 8, alignItems: 'center', overflowX: 'auto', padding: '8px 12px', background: '#020617', borderBottom: `1px solid ${theme.border}` }}>
          <span style={{ color: theme.textMuted, fontSize: 11, whiteSpace: 'nowrap' }}>Automation:</span>
          {screen.actions.slice(0, 5).map(action => {
            const tone = actionStatusTone(action.status);
            return (
              <span
                key={action.id}
                title={action.errorMsg || `${actionTypeLabel(action.type)} #${action.id}`}
                style={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 6,
                  border: `1px solid ${tone.border}`,
                  background: tone.bg,
                  color: tone.color,
                  borderRadius: 7,
                  padding: '4px 8px',
                  fontSize: 11,
                  whiteSpace: 'nowrap',
                }}
              >
                #{action.id} {actionTypeLabel(action.type)}
                <span style={{ color: tone.color, opacity: 0.9 }}>{tone.label}</span>
                <span style={{ color: theme.textFaint }}>{formatLastSeen(actionTime(action))}</span>
              </span>
            );
          })}
        </div>
      )}
      <div
        ref={surfaceRef}
        tabIndex={0}
        style={{ position: 'relative', minHeight: 420, display: 'grid', placeItems: 'center', background: '#000', outline: 'none' }}
      >
        <textarea
          ref={keyboardRef}
          aria-label="THG remote browser keyboard"
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
              alt="Local Chrome Facebook"
              style={{ width: '100%', height: 'auto', display: 'block', background: '#000', userSelect: 'none' }}
            />
            <div
              aria-label="THG remote browser control surface"
              onMouseDown={handlePointerDown}
              onWheel={handleWheel}
              onContextMenu={e => e.preventDefault()}
              style={{ position: 'absolute', inset: 0, cursor: remoteInputEnabled ? 'crosshair' : 'default', background: 'transparent' }}
            />
          </div>
        ) : (
          <div style={{ textAlign: 'center', padding: 28, maxWidth: 520 }}>
            <Laptop size={34} color="#5eead4" style={{ marginBottom: 12 }} />
            <p style={{ color: theme.text, fontSize: 14, fontWeight: 800, marginBottom: 6 }}>Äang chá» Chrome local trÃªn mÃ¡y nhÃ¢n viÃªn</p>
            <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.6 }}>
              Báº¥m Má»Ÿ Chrome local, Ä‘Äƒng nháº­p Facebook trong cá»­a sá»• Chrome vá»«a má»Ÿ trÃªn mÃ¡y Ä‘Ã³. Sau khi Facebook sáºµn sÃ ng, Chrome local sáº½ tá»± áº©n vÃ  dashboard nháº­n stream vá» Ä‘Ã¢y.
            </p>
          </div>
        )}
      </div>
    </div>
  );
}

