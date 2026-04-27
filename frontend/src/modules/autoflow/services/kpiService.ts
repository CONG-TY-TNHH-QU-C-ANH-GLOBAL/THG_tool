import type { KPIConfig, StaffMember, ScoredStaff } from '../types';
import { DEFAULT_KPI_CONFIG } from './mockData';
import * as api from './api';

interface BackendKPIConfig {
  conv_pts: number; conv2_pts: number; cmt_pts: number;
  bonus_pts: number; bonus_amt: number; pen_pts: number; pen_amt: number;
}

function toKPIConfig(b: BackendKPIConfig): KPIConfig {
  return {
    conv: b.conv_pts,
    conv2: b.conv2_pts,
    cmt: b.cmt_pts,
    bonus: b.bonus_pts,
    bonusAmt: b.bonus_amt,
    pen: b.pen_pts,
    penAmt: b.pen_amt,
  };
}

function fromKPIConfig(c: KPIConfig): BackendKPIConfig {
  return {
    conv_pts: c.conv,
    conv2_pts: c.conv2,
    cmt_pts: c.cmt,
    bonus_pts: c.bonus,
    bonus_amt: c.bonusAmt,
    pen_pts: c.pen,
    pen_amt: c.penAmt,
  };
}

export async function getKpiConfig(orgId: string): Promise<KPIConfig> {
  void orgId;
  try {
    const res = await api.get<BackendKPIConfig>('/kpi/config');
    return toKPIConfig(res);
  } catch {
    return { ...DEFAULT_KPI_CONFIG };
  }
}

export async function saveKpiConfig(orgId: string, config: KPIConfig): Promise<KPIConfig> {
  void orgId;
  try {
    await api.put<{ ok: boolean }>('/kpi/config', fromKPIConfig(config));
    return config;
  } catch {
    return config;
  }
}

/** Pure function — no side effects, safe to call in useMemo */
export function computeLeaderboard(staff: StaffMember[], config: KPIConfig): ScoredStaff[] {
  return [...staff]
    .map(s => ({
      ...s,
      pts: (s.convs * config.conv) + (s.converted * config.conv2) + (s.cmts * config.cmt),
    }))
    .sort((a, b) => b.pts - a.pts);
}
