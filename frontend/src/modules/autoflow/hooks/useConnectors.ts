import { useCallback, useEffect, useState } from 'react';
import type { LocalConnector } from '../types';
import { createLocalConnectorPairingCode, getLocalConnectors } from '../services/connectorsService';

export function useConnectors() {
  const [connectors, setConnectors] = useState<LocalConnector[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      setConnectors(await getLocalConnectors());
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
    const timer = setInterval(refresh, 10000);
    return () => clearInterval(timer);
  }, [refresh]);

  const createPairingCode = async (name: string, accountId?: number) => {
    setCreating(true);
    try {
      const result = await createLocalConnectorPairingCode(name, accountId);
      await refresh();
      return result;
    } finally {
      setCreating(false);
    }
  };

  return { connectors, loading, creating, refresh, createPairingCode };
}
