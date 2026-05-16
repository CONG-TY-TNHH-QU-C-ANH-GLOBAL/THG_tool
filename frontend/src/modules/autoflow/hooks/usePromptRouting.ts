import { useCallback, useEffect, useRef, useState } from 'react';
import {
  getPromptRoutingConflicts,
  getPromptRoutingDistribution,
  getPromptRoutingMissingSignals,
  getPromptRoutingRecent,
  type ConflictsResponse,
  type DistributionResponse,
  type MissingSignalsResponse,
  type PromptRoutingRow,
} from '../services/promptRoutingService';

// Watchpoint B hook — loads the four routing observability surfaces in
// parallel. Same cadence pattern as useExecutionReality (60s refresh,
// allSettled so partial failures don't blank the whole panel).

const REFRESH_INTERVAL_MS = 60_000;

interface State {
  distribution: DistributionResponse | null;
  recent: PromptRoutingRow[];
  conflicts: ConflictsResponse | null;
  missing: MissingSignalsResponse | null;
  isLoading: boolean;
  error: Error | null;
  windowHours: number;
}

export function usePromptRouting(initialHours: number = 24) {
  const [state, setState] = useState<State>({
    distribution: null,
    recent: [],
    conflicts: null,
    missing: null,
    isLoading: true,
    error: null,
    windowHours: initialHours,
  });
  const cancelledRef = useRef(false);

  const fetchAll = useCallback(async (hours: number) => {
    setState(prev => ({ ...prev, isLoading: true, error: null, windowHours: hours }));
    try {
      const [distRes, recentRes, conflictsRes, missingRes] = await Promise.allSettled([
        getPromptRoutingDistribution(hours),
        getPromptRoutingRecent(hours, 100),
        getPromptRoutingConflicts(hours),
        getPromptRoutingMissingSignals(hours),
      ]);
      if (cancelledRef.current) return;
      const allFailed = distRes.status === 'rejected'
        && recentRes.status === 'rejected'
        && conflictsRes.status === 'rejected'
        && missingRes.status === 'rejected';
      setState(prev => ({
        ...prev,
        distribution: distRes.status === 'fulfilled' ? distRes.value : null,
        recent: recentRes.status === 'fulfilled' ? recentRes.value.rows : [],
        conflicts: conflictsRes.status === 'fulfilled' ? conflictsRes.value : null,
        missing: missingRes.status === 'fulfilled' ? missingRes.value : null,
        isLoading: false,
        error: allFailed ? new Error('All routing observability endpoints failed') : null,
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
