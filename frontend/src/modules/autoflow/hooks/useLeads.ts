import { useState, useEffect, useCallback } from 'react';
import type { Lead, LeadStatus } from '../types';
import { getLeads } from '../services/leadsService';

export function useLeads(orgId: string, statusFilter: LeadStatus | 'All' = 'All') {
  const [leads, setLeads] = useState<Lead[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetch = useCallback(() => {
    let cancelled = false;
    setIsLoading(true);
    setError(null);
    getLeads(orgId, statusFilter === 'All' ? undefined : statusFilter)
      .then(data => { if (!cancelled) setLeads(data); })
      .catch(err => { if (!cancelled) setError(err instanceof Error ? err : new Error(String(err))); })
      .finally(() => { if (!cancelled) setIsLoading(false); });
    return () => { cancelled = true; };
  }, [orgId, statusFilter]);

  useEffect(() => fetch(), [fetch]);

  return { leads, isLoading, error, refetch: fetch };
}
