import { useState, useEffect, useCallback } from 'react';
import type { StaffMember, MemberStatus } from '../types';
import { getStaff, addStaff, updateStaffStatus, deleteStaff } from '../services/staffService';

export function useStaff(orgId: string) {
  const [staff, setStaff] = useState<StaffMember[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setIsLoading(true);
    getStaff(orgId).then(data => {
      if (!cancelled) { setStaff(data); setIsLoading(false); }
    });
    return () => { cancelled = true; };
  }, [orgId]);

  const add = useCallback(async (data: Pick<StaffMember, 'name' | 'email' | 'role'>) => {
    const member = await addStaff(orgId, data);
    setStaff(prev => [...prev, member]);
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

  return { staff, isLoading, add, toggleStatus, remove };
}
