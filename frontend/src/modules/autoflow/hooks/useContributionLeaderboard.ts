import { useCallback, useEffect, useState } from 'react';
import { getContributionLeaderboard, type ContributionLeaderboard } from '../services/contributionService';

/**
 * Loads the derived contribution leaderboard (action_ledger / created_by).
 * `days` is the lookback window (0 = all-time). Refetches when it changes.
 */
export function useContributionLeaderboard(days = 30) {
  const [data, setData] = useState<ContributionLeaderboard | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const refresh = useCallback(async (window: number) => {
    setLoading(true);
    setError('');
    try {
      setData(await getContributionLeaderboard(window));
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Không tải được bảng đóng góp.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const d = await getContributionLeaderboard(days);
        if (!cancelled) setData(d);
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : 'Không tải được bảng đóng góp.');
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [days]);

  return { data, loading, error, refresh };
}
