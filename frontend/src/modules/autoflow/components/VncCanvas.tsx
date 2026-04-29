import { useEffect, useRef, useState } from 'react';
import { theme } from '../constants/styles';
import { refreshToken } from '../services/authService';
import { useAuthStore } from '../stores/authStore';

type VncStatus = 'connecting' | 'handshake' | 'ready' | 'closed' | 'error';

interface VncCanvasProps {
  accountId: number;
  accountName: string;
  cdpPort?: number;
  vncPort?: number;
  errorMsg?: string;
}

interface Rect {
  x: number;
  y: number;
  w: number;
  h: number;
  encoding: number;
  pixels?: Uint8Array;
}

const textEncoder = new TextEncoder();

function readU16(buf: Uint8Array, offset: number): number {
  return (buf[offset] << 8) | buf[offset + 1];
}

function readU32(buf: Uint8Array, offset: number): number {
  return (
    ((buf[offset] << 24) >>> 0) |
    (buf[offset + 1] << 16) |
    (buf[offset + 2] << 8) |
    buf[offset + 3]
  ) >>> 0;
}

function readI32(buf: Uint8Array, offset: number): number {
  const n = readU32(buf, offset);
  return n > 0x7fffffff ? n - 0x100000000 : n;
}

function writeU16(buf: Uint8Array, offset: number, value: number) {
  buf[offset] = (value >> 8) & 0xff;
  buf[offset + 1] = value & 0xff;
}

function writeU32(buf: Uint8Array, offset: number, value: number) {
  buf[offset] = (value >>> 24) & 0xff;
  buf[offset + 1] = (value >>> 16) & 0xff;
  buf[offset + 2] = (value >>> 8) & 0xff;
  buf[offset + 3] = value & 0xff;
}

function keyToKeysym(key: string): number | null {
  const named: Record<string, number> = {
    Backspace: 0xff08,
    Tab: 0xff09,
    Enter: 0xff0d,
    Escape: 0xff1b,
    Delete: 0xffff,
    Home: 0xff50,
    ArrowLeft: 0xff51,
    ArrowUp: 0xff52,
    ArrowRight: 0xff53,
    ArrowDown: 0xff54,
    PageUp: 0xff55,
    PageDown: 0xff56,
    End: 0xff57,
    Insert: 0xff63,
    Shift: 0xffe1,
    Control: 0xffe3,
    Alt: 0xffe9,
    Meta: 0xffe7,
  };
  if (named[key]) return named[key];
  if (/^F([1-9]|1[0-2])$/.test(key)) return 0xffbe + Number(key.slice(1)) - 1;
  if (key.length === 1) {
    const cp = key.codePointAt(0);
    if (!cp) return null;
    return cp <= 0xff ? cp : 0x01000000 | cp;
  }
  return null;
}

class RfbClient {
  private buffer = new Uint8Array(0);
  private state: 'protocol' | 'security' | 'security-result' | 'server-init' | 'normal' = 'protocol';
  private width = 1280;
  private height = 800;

  constructor(
    private readonly ws: WebSocket,
    private readonly canvas: HTMLCanvasElement,
    private readonly onStatus: (status: string) => void,
    private readonly onReady: (w: number, h: number, name: string) => void,
    private readonly onFrame: () => void,
  ) {}

  accept(chunk: Uint8Array) {
    const merged = new Uint8Array(this.buffer.length + chunk.length);
    merged.set(this.buffer);
    merged.set(chunk, this.buffer.length);
    this.buffer = merged;
    this.process();
  }

  pointer(mask: number, x: number, y: number) {
    const msg = new Uint8Array(6);
    msg[0] = 5;
    msg[1] = mask & 0xff;
    writeU16(msg, 2, Math.max(0, Math.min(this.width - 1, Math.round(x))));
    writeU16(msg, 4, Math.max(0, Math.min(this.height - 1, Math.round(y))));
    this.send(msg);
  }

  key(keysym: number, down: boolean) {
    const msg = new Uint8Array(8);
    msg[0] = 4;
    msg[1] = down ? 1 : 0;
    writeU32(msg, 4, keysym >>> 0);
    this.send(msg);
  }

  private process() {
    let offset = 0;

    main:
    while (true) {
      const remaining = this.buffer.length - offset;
      switch (this.state) {
        case 'protocol':
          if (remaining < 12) break main;
          this.send(textEncoder.encode('RFB 003.008\n'));
          this.onStatus('Đang bắt tay VNC...');
          offset += 12;
          this.state = 'security';
          continue;

        case 'security': {
          if (remaining < 1) break main;
          const count = this.buffer[offset];
          if (count === 0) {
            if (remaining < 5) break main;
            const reasonLen = readU32(this.buffer, offset + 1);
            if (remaining < 5 + reasonLen) break main;
            throw new Error('VNC refused connection');
          }
          if (remaining < 1 + count) break main;
          const types = Array.from(this.buffer.slice(offset + 1, offset + 1 + count));
          if (!types.includes(1)) throw new Error('VNC requires unsupported authentication');
          this.send(new Uint8Array([1]));
          this.onStatus('Đang xác thực VNC...');
          offset += 1 + count;
          this.state = 'security-result';
          continue;
        }

        case 'security-result':
          if (remaining < 4) break main;
          if (readU32(this.buffer, offset) !== 0) throw new Error('VNC authentication failed');
          this.send(new Uint8Array([1]));
          this.onStatus('Đang khởi tạo desktop...');
          offset += 4;
          this.state = 'server-init';
          continue;

        case 'server-init': {
          if (remaining < 24) break main;
          const width = readU16(this.buffer, offset);
          const height = readU16(this.buffer, offset + 2);
          const nameLen = readU32(this.buffer, offset + 20);
          if (remaining < 24 + nameLen) break main;
          const nameBytes = this.buffer.slice(offset + 24, offset + 24 + nameLen);
          const name = new TextDecoder().decode(nameBytes);
          this.resize(width, height);
          this.setPixelFormat();
          this.setEncodings();
          this.requestUpdate(false);
          this.onReady(width, height, name);
          offset += 24 + nameLen;
          this.state = 'normal';
          continue;
        }

        case 'normal': {
          if (remaining < 1) break main;
          const type = this.buffer[offset];
          if (type === 0) {
            const parsed = this.tryParseFramebuffer(offset);
            if (!parsed) break main;
            this.draw(parsed.rects);
            offset = parsed.next;
            this.requestUpdate(true);
            continue;
          }
          if (type === 2) {
            offset += 1;
            continue;
          }
          if (type === 3) {
            if (remaining < 8) break main;
            const len = readU32(this.buffer, offset + 4);
            if (remaining < 8 + len) break main;
            offset += 8 + len;
            continue;
          }
          throw new Error(`Unsupported VNC message type ${type}`);
        }
      }
    }

    if (offset > 0) {
      this.buffer = this.buffer.slice(offset);
    }
  }

  private tryParseFramebuffer(offset: number): { next: number; rects: Rect[] } | null {
    if (this.buffer.length - offset < 4) return null;
    const count = readU16(this.buffer, offset + 2);
    let p = offset + 4;
    const rects: Rect[] = [];

    for (let i = 0; i < count; i++) {
      if (this.buffer.length - p < 12) return null;
      const x = readU16(this.buffer, p);
      const y = readU16(this.buffer, p + 2);
      const w = readU16(this.buffer, p + 4);
      const h = readU16(this.buffer, p + 6);
      const encoding = readI32(this.buffer, p + 8);
      p += 12;

      if (encoding === 0) {
        const bytes = w * h * 4;
        if (this.buffer.length - p < bytes) return null;
        rects.push({ x, y, w, h, encoding, pixels: this.buffer.slice(p, p + bytes) });
        p += bytes;
        continue;
      }

      if (encoding === -223 || encoding === -224) {
        rects.push({ x, y, w, h, encoding });
        continue;
      }

      throw new Error(`Unsupported VNC encoding ${encoding}`);
    }

    return { next: p, rects };
  }

  private draw(rects: Rect[]) {
    const ctx = this.canvas.getContext('2d');
    if (!ctx) return;

    for (const rect of rects) {
      if (rect.encoding === -223) {
        this.resize(rect.w, rect.h);
        continue;
      }
      if (rect.encoding === -224 || !rect.pixels || rect.w === 0 || rect.h === 0) continue;

      const image = ctx.createImageData(rect.w, rect.h);
      const rgba = image.data;
      for (let src = 0, dst = 0; src < rect.pixels.length; src += 4, dst += 4) {
        rgba[dst] = rect.pixels[src + 2];
        rgba[dst + 1] = rect.pixels[src + 1];
        rgba[dst + 2] = rect.pixels[src];
        rgba[dst + 3] = 255;
      }
      ctx.putImageData(image, rect.x, rect.y);
      this.onFrame();
    }
  }

  private resize(width: number, height: number) {
    this.width = Math.max(1, width);
    this.height = Math.max(1, height);
    this.canvas.width = this.width;
    this.canvas.height = this.height;
    this.canvas.style.aspectRatio = `${this.width} / ${this.height}`;
  }

  private setPixelFormat() {
    const msg = new Uint8Array(20);
    msg[0] = 0;
    msg[4] = 32;
    msg[5] = 24;
    msg[6] = 0;
    msg[7] = 1;
    writeU16(msg, 8, 255);
    writeU16(msg, 10, 255);
    writeU16(msg, 12, 255);
    msg[14] = 16;
    msg[15] = 8;
    msg[16] = 0;
    this.send(msg);
  }

  private setEncodings() {
    const msg = new Uint8Array(8);
    msg[0] = 2;
    writeU16(msg, 2, 1);
    writeU32(msg, 4, 0);
    this.send(msg);
  }

  private requestUpdate(incremental: boolean) {
    const msg = new Uint8Array(10);
    msg[0] = 3;
    msg[1] = incremental ? 1 : 0;
    writeU16(msg, 2, 0);
    writeU16(msg, 4, 0);
    writeU16(msg, 6, this.width);
    writeU16(msg, 8, this.height);
    this.send(msg);
  }

  private send(bytes: Uint8Array) {
    if (this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(bytes);
    }
  }
}

export default function VncCanvas({ accountId, accountName, cdpPort, vncPort, errorMsg }: VncCanvasProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const rfbRef = useRef<RfbClient | null>(null);
  const buttonMaskRef = useRef(0);
  const remoteSizeRef = useRef({ w: 1280, h: 800 });
  const [status, setStatus] = useState<VncStatus>('connecting');
  const [message, setMessage] = useState('Đang mở desktop...');
  const [error, setError] = useState<string | null>(null);
  const [hasFrame, setHasFrame] = useState(false);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    let cancelled = false;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let attempts = 0;

    const connect = async () => {
      attempts += 1;
      setStatus('connecting');
      setError(null);
      setHasFrame(false);
      setMessage(attempts === 1 ? 'Đang mở VNC desktop...' : `Đang kết nối lại VNC (${attempts})...`);

      const currentToken = useAuthStore.getState().token ?? '';
      const token = await refreshToken().catch(() => currentToken);
      if (cancelled) return;

      const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const ws = new WebSocket(`${proto}//${window.location.host}/ws/vnc/${accountId}?token=${encodeURIComponent(token || currentToken)}`);
      ws.binaryType = 'arraybuffer';
      wsRef.current = ws;

      ws.onopen = () => {
        if (cancelled) return;
        setStatus('handshake');
        setMessage('Đã nối tới VNC, đang chờ desktop...');
        rfbRef.current = new RfbClient(
          ws,
          canvas,
          setMessage,
          (w, h) => {
            remoteSizeRef.current = { w, h };
            setStatus('ready');
            setMessage('Desktop đã sẵn sàng');
          },
          () => setHasFrame(true),
        );
      };

      ws.onmessage = async (ev) => {
        if (cancelled || wsRef.current !== ws) return;
        try {
          if (typeof ev.data === 'string') {
            setStatus('error');
            setError(ev.data);
            return;
          }
          const data = ev.data instanceof Blob
            ? new Uint8Array(await ev.data.arrayBuffer())
            : new Uint8Array(ev.data as ArrayBuffer);
          rfbRef.current?.accept(data);
        } catch (e) {
          setStatus('error');
          setError(e instanceof Error ? e.message : 'VNC stream error');
          ws.close();
        }
      };

      ws.onerror = () => {
        if (cancelled) return;
        setStatus('error');
        setError('Không kết nối được VNC WebSocket');
      };

      ws.onclose = () => {
        if (cancelled || wsRef.current !== ws) return;
        rfbRef.current = null;
        setStatus(prev => prev === 'error' ? 'error' : 'closed');
        if (attempts < 6) {
          reconnectTimer = setTimeout(connect, Math.min(1000 * attempts, 5000));
        }
      };
    };

    void connect();

    return () => {
      cancelled = true;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      rfbRef.current = null;
      wsRef.current?.close();
      wsRef.current = null;
    };
  }, [accountId]);

  const canvasPoint = (clientX: number, clientY: number) => {
    const canvas = canvasRef.current;
    if (!canvas) return { x: 0, y: 0 };
    const rect = canvas.getBoundingClientRect();
    const remote = remoteSizeRef.current;
    return {
      x: ((clientX - rect.left) * remote.w) / rect.width,
      y: ((clientY - rect.top) * remote.h) / rect.height,
    };
  };

  const sendPointer = (mask: number, clientX: number, clientY: number) => {
    const p = canvasPoint(clientX, clientY);
    rfbRef.current?.pointer(mask, p.x, p.y);
  };

  const buttonBit = (button: number) => {
    if (button === 0) return 1;
    if (button === 1) return 2;
    if (button === 2) return 4;
    return 0;
  };

  const statusColor = status === 'ready' ? '#4ade80' : status === 'error' ? '#fca5a5' : '#f59e0b';

  return (
    <>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '7px 12px', background: theme.surfaceAlt, borderBottom: `1px solid ${theme.border}` }}>
        <div style={{ display: 'flex', gap: 5 }}>
          {['#ef4444', '#f59e0b', '#22c55e'].map(c => <div key={c} style={{ width: 10, height: 10, borderRadius: '50%', background: c }} />)}
        </div>
        <span style={{ color: theme.textFaint, fontSize: 12, flex: 1 }}>facebook.com — {accountName}</span>
        <span style={{ fontSize: 11, color: statusColor }}>
          ● {status === 'ready' ? 'Đã kết nối VNC' : status === 'error' ? 'Lỗi VNC' : 'Đang kết nối VNC'}
        </span>
      </div>

      <div style={{ position: 'relative', minHeight: 360, background: '#050505' }}>
        <canvas
          ref={canvasRef}
          tabIndex={0}
          style={{ display: 'block', width: '100%', aspectRatio: '16 / 10', background: '#050505', cursor: 'default', outline: 'none' }}
          onMouseDown={e => {
            e.currentTarget.focus();
            buttonMaskRef.current |= buttonBit(e.button);
            sendPointer(buttonMaskRef.current, e.clientX, e.clientY);
          }}
          onMouseUp={e => {
            buttonMaskRef.current &= ~buttonBit(e.button);
            sendPointer(buttonMaskRef.current, e.clientX, e.clientY);
          }}
          onMouseMove={e => sendPointer(buttonMaskRef.current, e.clientX, e.clientY)}
          onMouseLeave={e => {
            buttonMaskRef.current = 0;
            sendPointer(0, e.clientX, e.clientY);
          }}
          onWheel={e => {
            e.preventDefault();
            const mask = e.deltaY < 0 ? 8 : 16;
            sendPointer(mask, e.clientX, e.clientY);
            sendPointer(0, e.clientX, e.clientY);
          }}
          onKeyDown={e => {
            const keysym = keyToKeysym(e.key);
            if (keysym == null) return;
            e.preventDefault();
            rfbRef.current?.key(keysym, true);
          }}
          onKeyUp={e => {
            const keysym = keyToKeysym(e.key);
            if (keysym == null) return;
            e.preventDefault();
            rfbRef.current?.key(keysym, false);
          }}
          onContextMenu={e => e.preventDefault()}
        />
        {(!hasFrame || error) && (
          <div style={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center', padding: 24, textAlign: 'center', pointerEvents: 'none' }}>
            <div style={{ color: error || errorMsg ? '#fca5a5' : theme.textMuted, fontSize: 13, lineHeight: 1.6 }}>
              {error ? `⚠ ${error}` : message}
              {!hasFrame && errorMsg && <div style={{ color: '#fca5a5', marginTop: 8 }}>{errorMsg}</div>}
              <div style={{ color: theme.textFaint, marginTop: 8 }}>vnc:{vncPort ?? '-'} · cdp:{cdpPort ?? '-'}</div>
            </div>
          </div>
        )}
      </div>
    </>
  );
}
