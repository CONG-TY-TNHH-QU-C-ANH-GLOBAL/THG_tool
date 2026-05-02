import { useCallback, useEffect, useState } from 'react';
import { useFiles } from '../../hooks/useFiles';
import { useDataSources } from '../../hooks/useDataSources';
import { getBusinessContext, saveBusinessContext, type BusinessContext } from '../../services/settingsService';
import BusinessMemoryPanel from '../data/BusinessMemoryPanel';
import ContextSummaryPanel from '../data/ContextSummaryPanel';
import DataSourcesPanel from '../data/DataSourcesPanel';
import DataStatsGrid from '../data/DataStatsGrid';
import FileUploadPanel from '../data/FileUploadPanel';
import PrivateFilesTable from '../data/PrivateFilesTable';

interface DataPrivateViewProps { orgId: string; }

export default function DataPrivateView({ orgId }: DataPrivateViewProps) {
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

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
      <DataStatsGrid files={files} sources={sources} />
      <BusinessMemoryPanel
        context={businessContext}
        message={contextMsg}
        isSaving={savingContext}
        onChange={patch => setBusinessContext(prev => ({ ...prev, ...patch }))}
        onSave={saveContext}
      />
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
  );
}
