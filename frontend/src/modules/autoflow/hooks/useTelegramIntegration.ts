// Data hook for the channel-first Telegram integration page. Loads status + destinations (PRIMARY)
// + personal bindings (secondary) + audit (admins). Mutations refetch the affected slice (no risky
// optimistic writes) so the UI always reflects server truth.
import { useCallback, useEffect, useState } from 'react';
import * as tg from '../services/telegramIntegrationApi';

export interface TelegramData {
  status: tg.TelegramStatus | null;
  destinations: tg.TelegramDestination[];
  availableEventTypes: string[];
  availableFilters: string[];
  bindings: tg.TelegramBinding[];
  canManageAll: boolean;
  audit: tg.TelegramAuditEvent[];
  loading: boolean;
  error: string | null;
}

const EMPTY: TelegramData = {
  status: null, destinations: [], availableEventTypes: [], availableFilters: [],
  bindings: [], canManageAll: false, audit: [], loading: true, error: null,
};

export function useTelegramIntegration(isAdmin: boolean) {
  const [data, setData] = useState<TelegramData>(EMPTY);

  const load = useCallback(async () => {
    setData((d) => ({ ...d, loading: true, error: null }));
    try {
      const [status, dest, bindings] = await Promise.all([
        tg.getStatus(), tg.getDestinations(), tg.getBindings(),
      ]);
      const audit = isAdmin ? await tg.getAudit().catch(() => []) : [];
      setData({
        status, destinations: dest.destinations, availableEventTypes: dest.available_event_types,
        availableFilters: dest.available_filters, bindings: bindings.bindings,
        canManageAll: bindings.can_manage_all, audit, loading: false, error: null,
      });
    } catch (e) {
      setData((d) => ({ ...d, loading: false, error: e instanceof Error ? e.message : String(e) }));
    }
  }, [isAdmin]);

  useEffect(() => { void load(); }, [load]);

  const savePreferences = useCallback(async (id: number, body: { event_types: string[]; channel_filter: string }) => {
    await tg.updateDestinationPreferences(id, body);
    await load();
  }, [load]);

  const disconnect = useCallback(async (id: number) => {
    await tg.disconnectDestination(id);
    await load();
  }, [load]);

  const revoke = useCallback(async (id: number) => {
    await tg.revokeBinding(id);
    await load();
  }, [load]);

  return { ...data, reload: load, savePreferences, disconnect, revoke };
}
