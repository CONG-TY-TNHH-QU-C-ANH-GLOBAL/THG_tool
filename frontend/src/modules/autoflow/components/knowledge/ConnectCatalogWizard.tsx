'use client';

import { useMemo, useState } from 'react';
import { Database, Sparkles, Loader2, AlertCircle, X } from 'lucide-react';
import {
  createKnowledgeSource,
  syncKnowledgeSource,
  type KnowledgeSource,
  type SyncResult,
} from '../../services/knowledgeService';
import { ADAPTER_PRESETS, findPreset, type AdapterPreset } from './adapterPresets';

interface ConnectCatalogWizardProps {
  onConnected: (source: KnowledgeSource, firstSync: SyncResult | null, err: Error | null) => void;
  onCancel: () => void;
}

/**
 * Generic Connect Product Catalog wizard.
 *
 * No vendor-specific UI. The wizard offers ADAPTER_PRESETS as a
 * dropdown — saved field_map blueprints for known backends — and an
 * always-available Custom option. THG Fulfill's hub is one preset
 * among others, not a branded button.
 *
 * Three steps in a single form (no multi-page flow yet — operators
 * told us multi-step wizards feel bureaucratic for one config blob):
 *
 *   1. Pick a preset.
 *   2. Confirm the base URL + give the source a label.
 *   3. Submit → POST /knowledge/sources → POST /:id/sync.
 *
 * The first sync result rolls up into the parent toast via
 * onConnected so the operator sees "X products synced" immediately
 * instead of waiting for the next scheduled tick.
 */
export default function ConnectCatalogWizard({ onConnected, onCancel }: Readonly<ConnectCatalogWizardProps>) {
  const [presetId, setPresetId] = useState<string>(ADAPTER_PRESETS[0]?.id ?? '');
  const preset = useMemo<AdapterPreset | undefined>(() => findPreset(presetId), [presetId]);
  const [baseUrl, setBaseUrl] = useState<string>(preset?.baseUrl ?? '');
  const [label, setLabel] = useState<string>('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handlePresetChange = (id: string) => {
    setPresetId(id);
    const next = findPreset(id);
    setBaseUrl(next?.baseUrl ?? '');
  };

  const canSubmit = !!preset && baseUrl.trim() !== '' && label.trim() !== '' && !submitting;

  const submit = async () => {
    if (!preset || !canSubmit) return;
    setSubmitting(true);
    setError(null);
    try {
      const config = preset.buildConfig(baseUrl.trim());
      const created = await createKnowledgeSource({
        type: preset.adapter,
        label: label.trim(),
        connection_config: config,
        sync_policy: 'manual',
      });
      // Fire and surface the first sync — failures here are not fatal
      // to the wizard, the source exists and the operator can retry.
      let firstSync: SyncResult | null = null;
      let syncErr: Error | null = null;
      try {
        firstSync = await syncKnowledgeSource(created.id);
      } catch (e) {
        syncErr = e instanceof Error ? e : new Error(String(e));
      }
      onConnected(created, firstSync, syncErr);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Không tạo được nguồn dữ liệu.');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="cyber-oracle-card" style={{
      position: 'relative',
      padding: 22,
      borderRadius: 16,
      background: 'linear-gradient(135deg, rgba(79,70,229,0.04), rgba(6,182,212,0.04))',
      border: '1px solid rgba(79,70,229,0.18)',
      boxShadow: '0 8px 24px -10px rgba(79,70,229,0.18)',
    }}>
      <button
        type="button"
        onClick={onCancel}
        aria-label="Đóng"
        style={{
          position: 'absolute', top: 12, right: 12, background: 'transparent', border: 0,
          padding: 4, color: 'var(--text-faint)', cursor: 'pointer',
        }}
      >
        <X size={16} />
      </button>

      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
        <Database size={16} style={{ color: '#4F46E5' }} />
        <span style={{ fontSize: 11, fontWeight: 700, letterSpacing: '0.1em', color: '#4F46E5' }}>
          KẾT NỐI NGUỒN CATALOG
        </span>
      </div>
      <h3 style={{ margin: 0, fontSize: 18, fontWeight: 700, color: 'var(--text)', letterSpacing: '-0.01em' }}>
        Đồng bộ sản phẩm vào Workspace Knowledge
      </h3>
      <p style={{ margin: '6px 0 16px', fontSize: 13, color: 'var(--text-mute)', lineHeight: 1.5 }}>
        Chọn loại backend → điều chỉnh URL → đặt tên. Hệ thống sẽ cào sản phẩm vào kho của workspace để agent dùng khi soạn comment.
      </p>

      <form onSubmit={(e) => { e.preventDefault(); void submit(); }} style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
        <div>
          <label style={{ display: 'block', fontSize: 11, color: 'var(--text-faint)', fontWeight: 700, marginBottom: 6 }}>
            Loại backend
          </label>
          <select
            value={presetId}
            onChange={(e) => handlePresetChange(e.target.value)}
            className="input"
            style={{ width: '100%' }}
            disabled={submitting}
          >
            {ADAPTER_PRESETS.map((p) => (
              <option key={p.id} value={p.id}>
                {p.label}
              </option>
            ))}
          </select>
          {preset && (
            <p style={{ fontSize: 11.5, color: 'var(--text-faint)', marginTop: 4 }}>{preset.description}</p>
          )}
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: 12 }}>
          <div>
            <label style={{ display: 'block', fontSize: 11, color: 'var(--text-faint)', fontWeight: 700, marginBottom: 6 }}>
              URL gốc
            </label>
            <input
              type="url"
              value={baseUrl}
              onChange={(e) => setBaseUrl(e.target.value)}
              placeholder="https://..."
              className="input"
              style={{ width: '100%' }}
              disabled={submitting}
            />
          </div>
          <div>
            <label style={{ display: 'block', fontSize: 11, color: 'var(--text-faint)', fontWeight: 700, marginBottom: 6 }}>
              Tên nguồn (chỉ workspace bạn thấy)
            </label>
            <input
              type="text"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder="VD: Catalog chính"
              className="input"
              style={{ width: '100%' }}
              disabled={submitting}
            />
          </div>
        </div>

        {error && (
          <div style={{
            display: 'flex', gap: 8, alignItems: 'flex-start',
            padding: '10px 12px', fontSize: 12.5,
            color: 'var(--hot)', background: 'rgba(220,40,40,0.08)', borderRadius: 10,
          }}>
            <AlertCircle size={14} style={{ flexShrink: 0, marginTop: 1 }} />
            <span>{error}</span>
          </div>
        )}

        <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end' }}>
          <button type="button" onClick={onCancel} disabled={submitting} className="btn btn-ghost">
            Hủy
          </button>
          <button
            type="submit"
            disabled={!canSubmit}
            style={{
              padding: '10px 20px',
              borderRadius: 12,
              border: 0,
              cursor: canSubmit ? 'pointer' : 'not-allowed',
              background: canSubmit
                ? 'linear-gradient(135deg, #4F46E5, #06B6D4)'
                : 'var(--bg-elev-2)',
              color: canSubmit ? '#FFFFFF' : 'var(--text-faint)',
              fontWeight: 600,
              fontSize: 13,
              display: 'flex',
              alignItems: 'center',
              gap: 8,
              boxShadow: canSubmit ? '0 4px 12px rgba(79,70,229,0.3)' : 'none',
              transition: 'all 0.15s',
            }}
          >
            {submitting ? <Loader2 size={14} className="spin" /> : <Sparkles size={14} />}
            {submitting ? 'Đang kết nối + đồng bộ...' : 'Kết nối & đồng bộ'}
          </button>
        </div>
      </form>

      <style jsx>{`
        @keyframes spin { to { transform: rotate(360deg); } }
        :global(.spin) { animation: spin 0.8s linear infinite; }
      `}</style>
    </div>
  );
}
