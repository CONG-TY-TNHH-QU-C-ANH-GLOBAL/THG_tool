import { useState, useEffect, useMemo, useCallback } from 'react';
import type { StaffMember, KPIConfig, ScoredStaff } from '../types';
import { getStaff } from '../services/staffService';
import { getKpiConfig, saveKpiConfig, computeLeaderboard } from '../services/kpiService';

export function useLeaderboard(orgId: string) {
  const [staff, setStaff] = useState<StaffMember[]>([]);
  const [config, setConfig] = useState<KPIConfig>({ conv:10,conv2:50,cmt:2,bonus:1000,bonusAmt:500000,pen:300,penAmt:100000 });
  const [isSaving, setIsSaving] = useState(false);

  useEffect(() => {
    let cancelled = false;
    Promise.all([getStaff(orgId), getKpiConfig(orgId)]).then(([s, c]) => {
      if (!cancelled) { setStaff(s); setConfig(c); }
    });
    return () => { cancelled = true; };
  }, [orgId]);

  const scored: ScoredStaff[] = useMemo(
    () => computeLeaderboard(staff, config),
    [staff, config]
  );

  const updateConfig = useCallback(async (next: KPIConfig) => {
    setIsSaving(true);
    try {
      const saved = await saveKpiConfig(orgId, next);
      setConfig(saved);
    } finally {
      setIsSaving(false);
    }
  }, [orgId]);

  return { scored, config, updateConfig, isSaving };
}
