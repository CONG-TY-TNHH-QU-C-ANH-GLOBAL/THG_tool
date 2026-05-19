'use client';

import { useCallback, useEffect, useState } from 'react';
import {
  Database,
  RefreshCw,
  Trash2,
  ExternalLink,
  CheckCircle2,
  AlertTriangle,
  Loader2,
  Plus,
  Clock,
} from 'lucide-react';
import {
  deleteKnowledgeSource,
  listKnowledgeSources,
  syncKnowledgeSource,
  type HealthStatus,
  type KnowledgeSource,
  type SyncResult,
} from '../../services/knowledgeService';
import ConnectCatalogWizard from './ConnectCatalogWizard';

interface KnowledgeSourcesPanelProps {
  /** Read-only mode hides action buttons (used for non-admin members). */
  readonly?: boolean;
}

interface ToastEntry {
  id: number;
  tone: 'ok' | 'warn' | 'err';
  text: string;
}

/**
 * KnowledgeSourcesPanel surfaces every connected catalog/data source
 * for the workspace and the actions an operator can take on them
 * (manual sync, remove). It is the "live operations" view over the
 * /api/knowledge/sources surface PR-3 introduced.
 *
 * Health status pill semantics:
 *   - healthy   → cyan glow + last sync time.
 *   - syncing   → blue spinner + "Đang đồng bộ".
 *   - stale     → amber + last_error message (recoverable error).
 *   - error     → red + last_error message (permanent — operator must act).
 *   - needs_auth→ red + "Cần cấu hình lại auth".
 *
 * Numbers are sourced from the source row, not the SyncResult — the
 * row carries the persisted state regardless of when the panel
 * mounted; SyncResult is only used for the toast right after manual
 * sync completes.
 */
export default function KnowledgeSourcesPanel({ readonly }: KnowledgeSourcesPanelProps) {
  const [sources, setSources] = useState<KnowledgeSource[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [wizardOpen, setWizardOpen] = useState<boolean>(false);
  const [busyIds, setBusyIds] = useState<Set<number>>(new Set());
  const [toasts, setToasts] = useState<ToastEntry[]>([]);

  const refresh = useCallback(async () => {
    try {
      const list = await listKnowledgeSources();
      setSources(list);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Không tải được danh sách nguồn.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void refresh(); }, [refresh]);

  const pushToast = (tone: ToastEntry['tone'], text: string) => {
    const id = Date.now() + Math.random();
    setToasts((prev) => [...prev, { id, tone, text }]);
    setTimeout(() => setToasts((prev) => prev.filter((x) => x.id !== id)), 5500);
  };

  const setBusy = (id: number, busy: boolean) => {
    setBusyIds((prev) => {
      const next = new Set(prev);
      if (busy) next.add(id);
      else next.delete(id);
      return next;
    });
  };

  const onSync = async (s: KnowledgeSource) => {
    setBusy(s.id, true);
    try {
      const result = await syncKnowledgeSource(s.id);
      pushToast('ok', summariseSyncResult(s.label, result));
    } catch (e) {
      pushToast('err', e instanceof Error ? e.message : 'Đồng bộ thất bại.');
    } finally {
      setBusy(s.id, false);
      void refresh();
    }
  };

  const onDelete = async (s: KnowledgeSource) => {
    if (!confirm(`Xoá nguồn "${s.label}" và toàn bộ sản phẩm đã đồng bộ từ nó?`)) return;
    setBusy(s.id, true);
    try {
      const res = await deleteKnowledgeSource(s.id);
      pushToast('warn', `Đã xoá "${s.label}". ${res.assets_deleted} asset bị xoá theo.`);
    } catch (e) {
      pushToast('err', e instanceof Error ? e.message : 'Xoá thất bại.');
    } finally {
      setBusy(s.id, false);
      void refresh();
    }
  };

  const handleWizardConnected = (created: KnowledgeSource, firstSync: SyncResult | null, syncErr: Error | null) => {
    setWizardOpen(false);
    void refresh();
    if (syncErr) {
      pushToast('warn', `Đã tạo "${created.label}" nhưng đồng bộ đầu lỗi: ${syncErr.message}`);
    } else if (firstSync) {
      pushToast('ok', summariseSyncResult(created.label, firstSync));
    } else {
      pushToast('ok', `Đã tạo "${created.label}".`);
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14, position: 'relative' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <Database size={14} style={{ color: '#06B6D4' }} />
          <h3 style={{ margin: 0, fontSize: 13, fontWeight: 700, color: 'var(--text)' }}>Nguồn catalog đã kết nối</h3>
          <span style={{ fontSize: 11, color: 'var(--text-faint)' }}>· {sources.length}</span>
        </div>
        {!readonly && (
          <button
            type="button"
            onClick={() => setWizardOpen((v) => !v)}
            style={{
              padding: '7px 12px',
              borderRadius: 10,
              border: 0,
              cursor: 'pointer',
              background: wizardOpen ? 'var(--bg-elev-2)' : 'linear-gradient(135deg, #4F46E5, #06B6D4)',
              color: wizardOpen ? 'var(--text)' : '#FFFFFF',
              fontWeight: 600,
              fontSize: 12,
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              boxShadow: wizardOpen ? 'none' : '0 4px 12px rgba(79,70,229,0.25)',
            }}
          >
            <Plus size={12} />
            {wizardOpen ? 'Đóng' : 'Kết nối nguồn mới'}
          </button>
        )}
      </div>

      {wizardOpen && !readonly && (
        <ConnectCatalogWizard onConnected={handleWizardConnected} onCancel={() => setWizardOpen(false)} />
      )}

      {error && (
        <div style={{
          display: 'flex', gap: 8, alignItems: 'flex-start',
          padding: '10px 12px', fontSize: 12.5,
          color: 'var(--hot)', background: 'rgba(220,40,40,0.08)', borderRadius: 10,
        }}>
          <AlertTriangle size={14} style={{ flexShrink: 0, marginTop: 1 }} />
          <span>{error}</span>
        </div>
      )}

      {loading && (
        <div style={{ fontSize: 12, color: 'var(--text-mute)', display: 'flex', alignItems: 'center', gap: 6 }}>
          <Loader2 size={12} className="spin" /> Đang tải...
        </div>
      )}

      {!loading && sources.length === 0 && !wizardOpen && !readonly && (
        <div style={{
          padding: 20,
          borderRadius: 12,
          background: 'var(--bg-elev)',
          border: '1px dashed var(--line-strong)',
          textAlign: 'center',
          color: 'var(--text-mute)',
          fontSize: 13,
        }}>
          Chưa có nguồn catalog nào. Bấm "Kết nối nguồn mới" để bắt đầu — workspace sẽ tự đồng bộ sản phẩm vào kho.
        </div>
      )}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: 12 }}>
        {sources.map((s) => (
          <SourceCard
            key={s.id}
            source={s}
            busy={busyIds.has(s.id)}
            readonly={readonly}
            onSync={() => void onSync(s)}
            onDelete={() => void onDelete(s)}
          />
        ))}
      </div>

      {/* Toasts */}
      <div style={{ position: 'fixed', bottom: 16, right: 16, display: 'flex', flexDirection: 'column', gap: 8, zIndex: 1000 }}>
        {toasts.map((t) => (
          <div
            key={t.id}
            style={{
              padding: '10px 14px',
              borderRadius: 10,
              minWidth: 240,
              maxWidth: 420,
              fontSize: 12.5,
              color: 'var(--text)',
              background: 'var(--bg-elev-2)',
              borderLeft: `3px solid ${
                t.tone === 'ok' ? '#10B981' : t.tone === 'warn' ? '#F59E0B' : '#EF4444'
              }`,
              boxShadow: '0 8px 24px -8px rgba(0,0,0,0.25)',
            }}
          >
            {t.text}
          </div>
        ))}
      </div>

      <style jsx>{`
        @keyframes spin { to { transform: rotate(360deg); } }
        :global(.spin) { animation: spin 0.8s linear infinite; }
      `}</style>
    </div>
  );
}

// ── SourceCard ─────────────────────────────────────────────────────

function SourceCard({
  source, busy, readonly, onSync, onDelete,
}: {
  source: KnowledgeSource;
  busy: boolean;
  readonly?: boolean;
  onSync: () => void;
  onDelete: () => void;
}) {
  const pill = healthPill(source.health_status);
  const baseUrl = readBaseURL(source.connection_config);
  return (
    <div style={{
      padding: 16,
      borderRadius: 14,
      background: 'var(--bg-elev)',
      border: '1px solid var(--line)',
      boxShadow: '0 1px 2px rgba(0,0,0,0.04)',
      display: 'flex',
      flexDirection: 'column',
      gap: 10,
    }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 10 }}>
        <div style={{ minWidth: 0, flex: 1 }}>
          <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {source.label}
          </div>
          <div style={{ fontSize: 11, color: 'var(--text-faint)', marginTop: 2, display: 'flex', alignItems: 'center', gap: 4 }}>
            <span className="mono">{source.type}</span>
            {baseUrl && (
              <>
                <span style={{ opacity: 0.5 }}>·</span>
                <a
                  href={baseUrl}
                  target="_blank"
                  rel="noreferrer"
                  className="mono"
                  style={{ color: 'var(--text-faint)', textDecoration: 'none', display: 'inline-flex', alignItems: 'center', gap: 3, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: 180 }}
                >
                  {shortenURL(baseUrl)} <ExternalLink size={10} />
                </a>
              </>
            )}
          </div>
        </div>
        <span style={{
          fontSize: 10,
          fontWeight: 700,
          letterSpacing: '0.06em',
          textTransform: 'uppercase',
          padding: '3px 8px',
          borderRadius: 999,
          color: pill.fg,
          background: pill.bg,
          whiteSpace: 'nowrap',
        }}>
          {pill.label}
        </span>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 6, fontSize: 11 }}>
        <Stat icon={<CheckCircle2 size={11} style={{ color: 'var(--ok)' }} />} label="Sản phẩm" value={String(source.last_asset_count)} />
        <Stat icon={<Clock size={11} style={{ color: 'var(--text-faint)' }} />} label="Lượt cuối" value={formatRelative(source.last_sync_at)} />
      </div>

      {source.health_message && (
        <div style={{
          fontSize: 11,
          color: source.health_status === 'error' || source.health_status === 'needs_auth' ? 'var(--hot)' : 'var(--text-mute)',
          background: source.health_status === 'error' || source.health_status === 'needs_auth'
            ? 'rgba(220,40,40,0.06)' : 'var(--bg-elev-2)',
          padding: '6px 8px',
          borderRadius: 6,
          lineHeight: 1.4,
        }}>
          {source.health_message}
        </div>
      )}

      {!readonly && (
        <div style={{ display: 'flex', gap: 6, marginTop: 'auto' }}>
          <button
            type="button"
            onClick={onSync}
            disabled={busy}
            className="btn btn-ghost btn-sm"
            style={{ display: 'flex', alignItems: 'center', gap: 4 }}
          >
            {busy ? <Loader2 size={12} className="spin" /> : <RefreshCw size={12} />}
            {busy ? 'Đang đồng bộ...' : 'Đồng bộ'}
          </button>
          <button
            type="button"
            onClick={onDelete}
            disabled={busy}
            className="btn btn-ghost btn-sm"
            style={{ display: 'flex', alignItems: 'center', gap: 4, color: 'var(--hot)' }}
          >
            <Trash2 size={12} /> Xoá
          </button>
        </div>
      )}
    </div>
  );
}

function Stat({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) {
  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 4, color: 'var(--text-faint)' }}>
        {icon}
        <span>{label}</span>
      </div>
      <div className="mono" style={{ color: 'var(--text)', marginTop: 2 }}>{value}</div>
    </div>
  );
}

// ── helpers ────────────────────────────────────────────────────────

function healthPill(s: HealthStatus): { label: string; fg: string; bg: string } {
  switch (s) {
    case 'healthy':   return { label: 'Khỏe', fg: '#06B6D4', bg: 'rgba(6,182,212,0.12)' };
    case 'syncing':   return { label: 'Đang sync', fg: '#4F46E5', bg: 'rgba(79,70,229,0.12)' };
    case 'stale':     return { label: 'Lỗi tạm', fg: '#F59E0B', bg: 'rgba(245,158,11,0.14)' };
    case 'error':     return { label: 'Lỗi', fg: '#EF4444', bg: 'rgba(239,68,68,0.12)' };
    case 'needs_auth':return { label: 'Cần auth', fg: '#EF4444', bg: 'rgba(239,68,68,0.12)' };
    default:          return { label: s, fg: 'var(--text-mute)', bg: 'var(--bg-elev-2)' };
  }
}

function readBaseURL(config: unknown): string {
  if (config && typeof config === 'object' && 'base_url' in config) {
    const v = (config as { base_url?: unknown }).base_url;
    if (typeof v === 'string') return v;
  }
  return '';
}

function shortenURL(u: string, max = 40): string {
  try {
    const url = new URL(u);
    const compact = url.host + url.pathname;
    if (compact.length <= max) return compact;
    return compact.slice(0, max - 1) + '…';
  } catch {
    return u.length <= max ? u : u.slice(0, max - 1) + '…';
  }
}

function formatRelative(iso: string | null): string {
  if (!iso) return 'chưa có';
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return '—';
  const diff = Date.now() - t;
  if (diff < 0) return 'sắp tới';
  const s = Math.floor(diff / 1000);
  if (s < 60) return `${s}s trước`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} phút trước`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h trước`;
  return `${Math.floor(h / 24)} ngày trước`;
}

function summariseSyncResult(label: string, r: SyncResult): string {
  const seen = r.assets_seen;
  const created = r.assets_created;
  const rejected = r.assets_rejected;
  if (seen === 0) return `"${label}": không có sản phẩm nào trong nguồn.`;
  if (rejected === 0) return `"${label}": ${created}/${seen} sản phẩm đã đồng bộ thành công.`;
  return `"${label}": ${created} OK · ${rejected} bị loại · xem chi tiết trên thẻ.`;
}
