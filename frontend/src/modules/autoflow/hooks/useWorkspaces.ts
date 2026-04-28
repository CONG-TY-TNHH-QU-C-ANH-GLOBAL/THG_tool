import { useState, useEffect, useCallback } from 'react';
import { getWorkspaces, startWorkspace, stopWorkspace, setWorkspaceLoggedIn } from '../services/workspacesService';
import { Workspace } from '../types';

export function useWorkspaces() {
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState<Set<number>>(new Set());

  const refresh = useCallback(() => {
    setLoading(true);
    getWorkspaces().then(data => {
      setWorkspaces(data);
      setLoading(false);
    });
  }, []);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 5000);
    return () => clearInterval(interval);
  }, [refresh]);

  const start = async (id: number) => {
    setActionLoading(prev => new Set(prev).add(id));
    await startWorkspace(id);
    refresh();
  };

  const stop = async (id: number) => {
    setActionLoading(prev => new Set(prev).add(id));
    await stopWorkspace(id);
    refresh();
  };

  const markLoggedIn = async (id: number) => {
    setActionLoading(prev => new Set(prev).add(id));
    await setWorkspaceLoggedIn(id);
    refresh();
  };

  return { workspaces, loading, actionLoading, refresh, start, stop, markLoggedIn };
}