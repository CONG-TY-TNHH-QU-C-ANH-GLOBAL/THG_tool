// Data hook for the Telegram integration page. Loads status + bindings + alerts (+ audit for
// admins) and exposes control-plane mutations. After any mutation it refetches the affected slice
// (no risky optimistic writes) so the UI always reflects server truth.
import { useCallback, useEffect, useState } from 'react';
import * as tg from '../services/telegramIntegrationApi';

export interface TelegramData {
  status: tg.TelegramStatus | null;
  bindings: tg.TelegramBinding[];
  canManageAll: boolean;
  alerts: tg.TelegramAlertPrefs | null;
  audit: tg.TelegramAuditEvent[];
  loading: boolean;
  error: string | null;
}

export function useTelegramIntegration(isAdmin: boolean) {
  const [data, setData] = useState<TelegramData>({
    status: null, bindings: [], canManageAll: false, alerts: null, audit: [], loading: true, error: null,
  });

  const load = useCallback(async () => {
    setData((d) => ({ ...d, loading: true, error: null }));
    try {
      const [status, bindings, alerts] = await Promise.all([
        tg.getStatus(), tg.getBindings(), tg.getAlerts(),
      ]);
      const audit = isAdmin ? await tg.getAudit().catch(() => []) : [];
      setData({
        status, bindings: bindings.bindings, canManageAll: bindings.can_manage_all,
        alerts, audit, loading: false, error: null,
      });
    } catch (e) {
      setData((d) => ({ ...d, loading: false, error: e instanceof Error ? e.message : String(e) }));
    }
  }, [isAdmin]);

  useEffect(() => { void load(); }, [load]);

  const setEnabled = useCallback(async (enabled: boolean) => {
    const status = enabled ? await tg.enableIntegration() : await tg.disableIntegration();
    setData((d) => ({ ...d, status }));
  }, []);

  const revoke = useCallback(async (id: number) => {
    await tg.revokeBinding(id);
    await load();
  }, [load]);

  const saveAlerts = useCallback(async (body: { alerts_enabled: boolean; channel_filter: string; alert_types: string[] }) => {
    const alerts = await tg.updateAlerts(body);
    setData((d) => ({ ...d, alerts }));
  }, []);

  return { ...data, reload: load, setEnabled, revoke, saveAlerts };
}
