import { useState, useEffect } from 'react';
import type { FacebookStatus } from '../types';
import { getFacebookStatus, connectFacebook, disconnectFacebook } from '../services/facebookService';

export function useFacebookSession(orgId: string) {
  const [status, setStatus] = useState<FacebookStatus>({ connected: false });
  const [isConnecting, setIsConnecting] = useState(false);

  useEffect(() => {
    let cancelled = false;
    getFacebookStatus(orgId).then(s => { if (!cancelled) setStatus(s); });
    return () => { cancelled = true; };
  }, [orgId]);

  const connect = async () => {
    setIsConnecting(true);
    try {
      const s = await connectFacebook(orgId);
      setStatus(s);
    } finally {
      setIsConnecting(false);
    }
  };

  const disconnect = async () => {
    await disconnectFacebook(orgId);
    setStatus({ connected: false });
  };

  return { status, isConnecting, connect, disconnect };
}
