import { useCallback, useEffect, useState } from 'react';
import { useFiles } from '../../hooks/useFiles';
import { useDataSources } from '../../hooks/useDataSources';
import { getBusinessContext, saveBusinessContext } from '../../services/settingsService';
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
  const [businessProfile, setBusinessProfile] = useState('');
  const [privateFilesSummary, setPrivateFilesSummary] = useState('');
  const [dataSourcesSummary, setDataSourcesSummary] = useState('');
  const [contextMsg, setContextMsg] = useState('');
  const [savingContext, setSavingContext] = useState(false);

  const loadContext = useCallback(async () => {
    const ctx = await getBusinessContext();
    setBusinessProfile(ctx.business_profile || '');
    setPrivateFilesSummary(ctx.private_files || '');
    setDataSourcesSummary(ctx.data_sources || '');
  }, []);

  useEffect(() => {
    let cancelled = false;
    getBusinessContext()
      .then(ctx => {
        if (cancelled) return;
        setBusinessProfile(ctx.business_profile || '');
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
      await saveBusinessContext(businessProfile);
      setContextMsg('Đã lưu business memory.');
    } catch (err) {
      setContextMsg(err instanceof Error ? err.message : 'Không lưu được context.');
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
        value={businessProfile}
        message={contextMsg}
        isSaving={savingContext}
        onChange={setBusinessProfile}
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
