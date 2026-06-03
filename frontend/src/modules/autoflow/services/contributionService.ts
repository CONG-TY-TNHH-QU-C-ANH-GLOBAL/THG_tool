/**
 * Contribution leaderboard service — DERIVED attribution (PR5).
 *
 * Distinct from the KPI leaderboard (staff_kpi, weighted by kpi_config). This
 * one is a projection over the append-only action_ledger, keyed by the
 * IMMUTABLE created_by: it answers "who actually executed verified actions",
 * not "who scored points". Reassigning an account never rewrites history.
 *
 * Champion here is analytics-only — it confers NO routing priority, NO lead
 * ownership, NO execution privilege (Ownership ⊥ Champion).
 *
 * GET /api/contribution-leaderboard?days=N&limit=M
 *   → { leaderboard: [{ user_id, user_name, total, by_type }], champion, count }
 */
import { get } from './api';

export interface ContributionRow {
  userId: number;
  userName: string;
  total: number;
  byType: Record<string, number>;
}

export interface ContributionLeaderboard {
  rows: ContributionRow[];
  champion: string;
  count: number;
}

interface RawRow {
  user_id: number;
  user_name?: string;
  total?: number;
  by_type?: Record<string, number>;
}

export async function getContributionLeaderboard(days = 30, limit = 50): Promise<ContributionLeaderboard> {
  const r = await get<{ leaderboard?: RawRow[]; champion?: string; count?: number }>(
    `/contribution-leaderboard?days=${days}&limit=${limit}`,
  );
  return {
    rows: (r.leaderboard ?? []).map(row => ({
      userId: row.user_id,
      userName: row.user_name ?? `#${row.user_id}`,
      total: row.total ?? 0,
      byType: row.by_type ?? {},
    })),
    champion: r.champion ?? '',
    count: r.count ?? 0,
  };
}
