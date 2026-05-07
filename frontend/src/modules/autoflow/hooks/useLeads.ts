import { useState, useEffect, useCallback, useRef } from 'react';
import type { Lead, LeadStatus } from '../types';
import { getLeads, deleteLead as deleteLeadService } from '../services/leadsService';

const CRAWL_DISPATCH_KEY = 'autoflow:last_crawl_dispatch';
const POLL_INTERVAL_MS = 15_000;
const POLL_DURATION_MS = 5 * 60 * 1000;

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

  // Keep a ref so the polling interval always calls the latest fetch without
  // restarting the interval every time statusFilter or orgId changes.
  const fetchRef = useRef(fetch);
  useEffect(() => { fetchRef.current = fetch; }, [fetch]);

  // After a crawl is dispatched from WorkspaceChatView, poll every 15 s for
  // up to 5 minutes so new leads surface automatically without a manual refresh.
  useEffect(() => {
    const lastDispatch = Number(sessionStorage.getItem(CRAWL_DISPATCH_KEY) ?? '0');
    const remaining = lastDispatch ? POLL_DURATION_MS - (Date.now() - lastDispatch) : 0;
    if (remaining <= 0) return;

    const timer = setInterval(() => { fetchRef.current(); }, POLL_INTERVAL_MS);
    const expiry = setTimeout(() => clearInterval(timer), remaining);
    return () => { clearInterval(timer); clearTimeout(expiry); };
  }, [orgId]);

  const remove = useCallback(async (leadId: number) => {
    await deleteLeadService(orgId, leadId);
    setLeads(prev => prev.filter(l => l.id !== leadId));
  }, [orgId]);

  return { leads, isLoading, error, refetch: fetch, remove };
}
