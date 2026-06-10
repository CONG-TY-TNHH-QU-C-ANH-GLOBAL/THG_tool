import { useState, useEffect } from 'react';
import type { Lead } from '../types';
import { getArchivedLeads, unarchiveLead as unarchiveService } from '../services/lifecycleService';

// useArchivedLeads lazily loads the "Đã lưu trữ" tab — only when the tab is active, so the
// default (act-now) view never pays for the archived query. See PR-4 in the spec.
export function useArchivedLeads(enabled: boolean) {
  const [archived, setArchived] = useState<Lead[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    setIsLoading(true);
    getArchivedLeads(100, 0)
      .then((data) => { if (!cancelled) setArchived(data); })
      .finally(() => { if (!cancelled) setIsLoading(false); });
    return () => { cancelled = true; };
  }, [enabled, reloadKey]);

  const restore = async (leadId: number) => {
    await unarchiveService(leadId);
    setArchived((prev) => prev.filter((l) => l.id !== leadId));
  };

  return { archived, isLoading, refetch: () => setReloadKey((k) => k + 1), restore };
}
