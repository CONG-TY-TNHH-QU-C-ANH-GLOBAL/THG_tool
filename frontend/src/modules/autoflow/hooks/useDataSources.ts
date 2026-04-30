import { useCallback, useEffect, useState } from 'react';
import type { DataSource, DataSourceType } from '../types';
import { createDataSource, deleteDataSource, getDataSources, syncDataSource } from '../services/dataSourceService';

export function useDataSources(orgId: string) {
  const [sources, setSources] = useState<DataSource[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isSyncing, setIsSyncing] = useState<number | null>(null);

  const reload = useCallback(async () => {
    setIsLoading(true);
    try {
      setSources(await getDataSources(orgId));
    } catch {
      setSources([]);
    } finally {
      setIsLoading(false);
    }
  }, [orgId]);

  useEffect(() => {
    let cancelled = false;
    setIsLoading(true);
    getDataSources(orgId)
      .then(data => { if (!cancelled) setSources(data); })
      .catch(() => { if (!cancelled) setSources([]); })
      .finally(() => { if (!cancelled) setIsLoading(false); });
    return () => { cancelled = true; };
  }, [orgId]);

  const add = useCallback(async (body: { type: DataSourceType; name: string; source_url: string }) => {
    const created = await createDataSource(orgId, body);
    setSources(prev => [created, ...prev]);
    return created;
  }, [orgId]);

  const sync = useCallback(async (id: number) => {
    setIsSyncing(id);
    try {
      const updated = await syncDataSource(orgId, id);
      setSources(prev => prev.map(s => s.id === id ? updated : s));
      return updated;
    } finally {
      setIsSyncing(null);
    }
  }, [orgId]);

  const remove = useCallback(async (id: number) => {
    await deleteDataSource(orgId, id);
    setSources(prev => prev.filter(s => s.id !== id));
  }, [orgId]);

  return { sources, isLoading, isSyncing, reload, add, sync, remove };
}
