import { useCallback, useEffect, useState } from 'react';
import { ChevronDown, ChevronRight } from 'lucide-react';
import { useFiles } from '../../hooks/useFiles';
import { useDataSources } from '../../hooks/useDataSources';
import {
  getBusinessContext, saveBusinessContext, type BusinessContext,
  getOrgPolicy, updateOrgPolicy, type OutboundMode,
  type InferBusinessContextResult, INFERRED_FIELD_KEYS, type InferredFieldKey,
} from '../../services/settingsService';
import BusinessMemoryPanel, { type BusinessConfidences } from '../data/BusinessMemoryPanel';
import ContextSummaryPanel from '../data/ContextSummaryPanel';
import DataSourcesPanel from '../data/DataSourcesPanel';
import DataStatsGrid from '../data/DataStatsGrid';
import FileUploadPanel from '../data/FileUploadPanel';
import MagicOmnibox from '../data/MagicOmnibox';
import OutboundPolicyPanel from '../data/OutboundPolicyPanel';
import PrivateFilesTable from '../data/PrivateFilesTable';

interface DataPrivateViewProps { orgId: string; isAdmin: boolean; }

export default function DataPrivateView({ orgId, isAdmin }: DataPrivateViewProps) {
  const { files, isUploading, upload, remove } = useFiles(orgId);
  const { sources, isLoading, isSyncing, add, sync, remove: removeSource } = useDataSources(orgId);
  const [businessContext, setBusinessContext] = useState<BusinessContext>({
    business_profile: '',
    business_name: '',
    business_industry: '',
    services: '',
    target_customers: '',
    target_author_role: 'customers',
    target_signals: '',
    negative_signals: '',
    business_location: '',
    markets: '',
    business_usp: '',
    tone: '',
    approval_policy: '',
    reject_rules: '',
    private_files: '',
    data_sources: '',
  });
  const [privateFilesSummary, setPrivateFilesSummary] = useState('');
  const [dataSourcesSummary, setDataSourcesSummary] = useState('');
  const [contextMsg, setContextMsg] = useState('');
  const [savingContext, setSavingContext] = useState(false);
  const [outboundMode, setOutboundMode] = useState<OutboundMode>('draft');
  const [policyMsg, setPolicyMsg] = useState('');
  const [savingPolicy, setSavingPolicy] = useState(false);
  const [confidences, setConfidences] = useState<BusinessConfidences>({});
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [inferSummary, setInferSummary] = useState<string>('');

  const loadContext = useCallback(async () => {
    const ctx = await getBusinessContext();
    setBusinessContext(prev => ({ ...prev, ...ctx, target_author_role: ctx.target_author_role || 'customers' }));
    setPrivateFilesSummary(ctx.private_files || '');
    setDataSourcesSummary(ctx.data_sources || '');
  }, []);

  useEffect(() => {
    let cancelled = false;
    getBusinessContext()
      .then(ctx => {
        if (cancelled) return;
        setBusinessContext(prev => ({ ...prev, ...ctx, target_author_role: ctx.target_author_role || 'customers' }));
        setPrivateFilesSummary(ctx.private_files || '');
        setDataSourcesSummary(ctx.data_sources || '');
      })
      .catch(() => {});
    return () => { cancelled = true; };
  }, [orgId, files.length]);

  useEffect(() => {
    let cancelled = false;
    getOrgPolicy()
      .then(p => { if (!cancelled) setOutboundMode(p.outbound_mode); })
      .catch(() => {});
    return () => { cancelled = true; };
  }, [orgId]);

  const savePolicy = async (next: OutboundMode) => {
    setSavingPolicy(true);
    setPolicyMsg('');
    try {
      const res = await updateOrgPolicy({ outbound_mode: next });
      setOutboundMode(res.outbound_mode);
      setPolicyMsg(res.outbound_mode === 'auto'
        ? 'Đã bật tự động duyệt — outbound sẽ chạy ngay không qua hàng chờ.'
        : 'Đã tắt tự động duyệt — mọi outbound vào hàng chờ admin duyệt.');
    } catch (err) {
      setPolicyMsg(err instanceof Error ? err.message : 'Không cập nhật được chính sách outbound.');
    } finally {
      setSavingPolicy(false);
    }
  };

  const handleFiles = (fileList: FileList | null) => {
    if (!fileList) return;
    Array.from(fileList).forEach(async file => {
      await upload(file);
      await loadContext().catch(() => {});
    });
  };

  const saveContext = async () => {
    setSavingContext(true);
    setContextMsg('');
    try {
      await saveBusinessContext(businessContext);
      setContextMsg('Đã lưu định vị doanh nghiệp.');
    } catch (err) {
      setContextMsg(err instanceof Error ? err.message : 'Không lưu được định vị doanh nghiệp.');
    } finally {
      setSavingContext(false);
    }
  };

  const syncSource = async (id: number) => {
    const updated = await sync(id);
    await loadContext().catch(() => {});
    return updated;
  };

  const deleteSource = async (id: number) => {
    await removeSource(id);
    await loadContext().catch(() => {});
  };

  const deleteFile = async (id: number) => {
    await remove(id);
    await loadContext().catch(() => {});
  };

  // When MagicOmnibox returns a proposal we merge non-empty values into
  // the form state and stash the per-field confidences. Empty proposals
  // are skipped so we never wipe out a value the user already typed.
  const handleInferred = (result: InferBusinessContextResult) => {
    const patch: Partial<BusinessContext> = {};
    const nextConfidences: BusinessConfidences = { ...confidences };
    INFERRED_FIELD_KEYS.forEach((key) => {
      const field = result[key];
      if (field && field.value) {
        (patch as Record<string, string>)[key] = field.value;
        nextConfidences[key] = field.confidence;
      }
    });
    setBusinessContext(prev => ({ ...prev, ...patch }));
    setConfidences(nextConfidences);
    setAdvancedOpen(true);
    setInferSummary(result.source_summary || '');
  };

  // Editing a field invalidates the AI confidence tag for that field —
  // the value is now user-owned. We also drop the inferSummary banner
  // once the user starts touching anything (it's not a permanent badge).
  const handleContextChange = (patch: Partial<BusinessContext>) => {
    setBusinessContext(prev => ({ ...prev, ...patch }));
    setConfidences(prev => {
      const next = { ...prev };
      Object.keys(patch).forEach((key) => {
        if ((INFERRED_FIELD_KEYS as readonly string[]).includes(key)) {
          delete next[key as InferredFieldKey];
        }
      });
      return next;
    });
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      {isAdmin && <MagicOmnibox onInferred={handleInferred} />}

      {inferSummary && (
        <div className="cyber-oracle-summary" style={{
          padding: '10px 14px',
          borderRadius: 12,
          background: 'linear-gradient(135deg, rgba(79,70,229,0.06), rgba(6,182,212,0.06))',
          border: '1px solid rgba(6,182,212,0.18)',
          fontSize: 13,
          color: 'var(--text)',
          lineHeight: 1.5,
        }}>
          <span style={{ fontWeight: 700, color: '#06B6D4', marginRight: 8 }}>AI tóm tắt:</span>
          {inferSummary}
        </div>
      )}

      <DataStatsGrid files={files} sources={sources} />
      <OutboundPolicyPanel
        mode={outboundMode}
        message={policyMsg}
        isSaving={savingPolicy}
        isAdmin={isAdmin}
        onChange={savePolicy}
      />
      <BusinessMemoryPanel
        context={businessContext}
        message={contextMsg}
        isSaving={savingContext}
        confidences={confidences}
        onChange={handleContextChange}
        onSave={saveContext}
      />

      <button
        type="button"
        onClick={() => setAdvancedOpen((v) => !v)}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 6,
          padding: '6px 10px',
          alignSelf: 'flex-start',
          background: 'transparent',
          border: 0,
          color: 'var(--text-mute)',
          fontSize: 12,
          cursor: 'pointer',
        }}
      >
        {advancedOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        Nguồn dữ liệu &amp; tệp đính kèm (nâng cao)
      </button>

      {advancedOpen && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          <DataSourcesPanel
            sources={sources}
            isLoading={isLoading}
            isSyncing={isSyncing}
            onAdd={add}
            onSync={syncSource}
            onRemove={deleteSource}
          />
          <FileUploadPanel isUploading={isUploading} onUpload={handleFiles} />
          <ContextSummaryPanel privateFilesSummary={privateFilesSummary} dataSourcesSummary={dataSourcesSummary} />
          <PrivateFilesTable files={files} onRemove={deleteFile} />
        </div>
      )}
    </div>
  );
}
