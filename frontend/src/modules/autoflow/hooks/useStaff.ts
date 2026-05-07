import { useState, useEffect, useCallback } from 'react';
import type { StaffInvite, StaffMember, MemberStatus } from '../types';
import { getStaff, getStaffInvites, inviteStaff, resendStaffInvite, revokeStaffInvite, updateStaffRole, updateStaffStatus, deleteStaff } from '../services/staffService';

export function useStaff(orgId: string) {
  const [staff, setStaff] = useState<StaffMember[]>([]);
  const [invites, setInvites] = useState<StaffInvite[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setIsLoading(true);
    Promise.allSettled([getStaff(orgId), getStaffInvites(orgId)])
      .then(([members, pending]) => {
        if (!cancelled) {
          if (members.status === 'fulfilled') setStaff(members.value);
          if (pending.status === 'fulfilled') setInvites(pending.value);
          setIsLoading(false);
        }
      })
      .catch(() => {
        if (!cancelled) setIsLoading(false);
      });
    return () => { cancelled = true; };
  }, [orgId]);

  const invite = useCallback(async (data: Pick<StaffMember, 'email' | 'role'>) => {
    const created = await inviteStaff(orgId, data);
    setInvites(prev => [created, ...prev]);
    return created;
  }, [orgId]);

  const revokeInvite = useCallback(async (inviteId: number) => {
    await revokeStaffInvite(orgId, inviteId);
    setInvites(prev => prev.filter(inv => inv.id !== inviteId));
  }, [orgId]);

  const resendInvite = useCallback(async (inviteId: number) => {
    const updated = await resendStaffInvite(orgId, inviteId);
    setInvites(prev => prev.map(inv => inv.id === inviteId ? { ...inv, ...updated } : inv));
    return updated;
  }, [orgId]);

  const toggleStatus = useCallback(async (staffId: number) => {
    const current = staff.find(s => s.id === staffId);
    if (!current) return;
    const next: MemberStatus = current.status === 'Active' ? 'Suspended' : 'Active';
    const updated = await updateStaffStatus(orgId, staffId, next);
    setStaff(prev => prev.map(s => s.id === staffId ? updated : s));
  }, [orgId, staff]);

  const remove = useCallback(async (staffId: number) => {
    await deleteStaff(orgId, staffId);
    setStaff(prev => prev.filter(s => s.id !== staffId));
  }, [orgId]);

  const changeRole = useCallback(async (staffId: number, role: 'admin' | 'sales') => {
    await updateStaffRole(orgId, staffId, role);
    setStaff(prev => prev.map(s => s.id === staffId ? { ...s, role } : s));
  }, [orgId]);

  return { staff, invites, isLoading, invite, resendInvite, revokeInvite, toggleStatus, remove, changeRole };
}
