import { useCallback, useEffect, useRef, useState } from 'react';
import {
  getAccountHealth,
  getOutcomeDistribution,
  getRecentExecutionAttempts,
  type AccountHealthRow,
  type DistributionResponse,
  type ExecutionAttemptRow,
} from '../services/executionService';

// Step 4a hook — loads the three observation surfaces in parallel and
// refreshes them on a fixed cadence. Cadence is deliberately conservative
// (60s) — this is operational observation, not a real-time monitor.
// The operator can press the manual refresh button if they want fresher
// data after running a test action.

const REFRESH_INTERVAL_MS = 60_000;

interface ExecutionRealityState {
  distribution: DistributionResponse | null;
  attempts: ExecutionAttemptRow[];
  accounts: AccountHealthRow[];
  isLoading: boolean;
  error: Error | null;
  windowHours: number;
}

export function useExecutionReality(initialHours: number = 24) {
  const [state, setState] = useState<ExecutionRealityState>({
    distribution: null,
    attempts: [],
    accounts: [],
    isLoading: true,
    error: null,
    windowHours: initialHours,
  });
  const cancelledRef = useRef(false);

  const fetchAll = useCallback(async (hours: number) => {
    setState(prev => ({ ...prev, isLoading: true, error: null, windowHours: hours }));
    try {
      // Parallel: distribution, recent attempts, account health. Any one
      // failing degrades that panel only — keep partial render rather
      // than blocking the whole view.
      const [distRes, recentRes, healthRes] = await Promise.allSettled([
        getOutcomeDistribution(hours),
        getRecentExecutionAttempts(hours, 100),
        getAccountHealth(),
      ]);
      if (cancelledRef.current) return;

      setState(prev => ({
        ...prev,
        distribution: distRes.status === 'fulfilled' ? distRes.value : null,
        attempts: recentRes.status === 'fulfilled' ? recentRes.value.attempts : [],
        accounts: healthRes.status === 'fulfilled' ? healthRes.value.accounts : [],
        isLoading: false,
        error: distRes.status === 'rejected' && recentRes.status === 'rejected' && healthRes.status === 'rejected'
          ? new Error('All observability endpoints failed')
          : null,
      }));
    } catch (err) {
      if (cancelledRef.current) return;
      setState(prev => ({
        ...prev,
        isLoading: false,
        error: err instanceof Error ? err : new Error(String(err)),
      }));
    }
  }, []);

  useEffect(() => {
    cancelledRef.current = false;
    void fetchAll(initialHours);
    const timer = window.setInterval(() => void fetchAll(initialHours), REFRESH_INTERVAL_MS);
    return () => {
      cancelledRef.current = true;
      window.clearInterval(timer);
    };
  }, [fetchAll, initialHours]);

  const setWindowHours = useCallback((hours: number) => {
    void fetchAll(hours);
  }, [fetchAll]);

  const refetch = useCallback(() => fetchAll(state.windowHours), [fetchAll, state.windowHours]);

  return { ...state, setWindowHours, refetch };
}
