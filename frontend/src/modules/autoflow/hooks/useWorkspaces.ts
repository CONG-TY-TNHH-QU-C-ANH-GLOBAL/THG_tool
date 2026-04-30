import { useState, useEffect, useCallback } from 'react';
import { getWorkspaces, startNewWorkspace, startWorkspace, stopWorkspace, setWorkspaceLoggedIn, syncWorkspaceSession } from '../services/workspacesService';
import { Workspace, WorkspaceSessionSnapshot } from '../types';

export function useWorkspaces() {
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState<Set<number>>(new Set());

  const refresh = useCallback(async () => {
    setLoading(true);
    const data = await getWorkspaces();
    setWorkspaces(data);
    setLoading(false);
  }, []);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 5000);
    return () => clearInterval(interval);
  }, [refresh]);

  const clearActionLoading = (id: number) => {
    setActionLoading(prev => {
      const next = new Set(prev);
      next.delete(id);
      return next;
    });
  };

  const start = async (id: number) => {
    setActionLoading(prev => new Set(prev).add(id));
    try {
      await startWorkspace(id);
      await refresh();
    } finally {
      clearActionLoading(id);
    }
  };

  const stop = async (id: number) => {
    setActionLoading(prev => new Set(prev).add(id));
    try {
      await stopWorkspace(id);
      await refresh();
    } finally {
      clearActionLoading(id);
    }
  };

  const startNew = async (): Promise<number> => {
    const result = await startNewWorkspace();
    await refresh();
    return result.accountId;
  };

  const markLoggedIn = async (id: number) => {
    setActionLoading(prev => new Set(prev).add(id));
    try {
      await setWorkspaceLoggedIn(id);
      await refresh();
    } finally {
      clearActionLoading(id);
    }
  };

  const syncSession = async (id: number): Promise<WorkspaceSessionSnapshot> => {
    const snapshot = await syncWorkspaceSession(id);
    await refresh();
    return snapshot;
  };

  return { workspaces, loading, actionLoading, refresh, start, startNew, stop, markLoggedIn, syncSession };
}
