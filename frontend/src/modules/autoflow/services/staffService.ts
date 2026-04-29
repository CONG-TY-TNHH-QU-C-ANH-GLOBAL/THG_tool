import type { StaffMember, MemberStatus } from '../types';
import * as api from './api';

interface BackendStaff {
  id: number; name: string; email: string; role: string;
  status: string; joined: string; convs: number; converted: number; cmts: number;
}
interface StaffResponse { staff: BackendStaff[]; count: number; }

function toStaff(b: BackendStaff): StaffMember {
  return {
    id: b.id,
    name: b.name,
    email: b.email,
    role: b.role as StaffMember['role'],
    status: (b.status === 'Suspended' ? 'Suspended' : 'Active') as MemberStatus,
    joined: b.joined,
    convs: b.convs,
    converted: b.converted,
    cmts: b.cmts,
  };
}

export async function getStaff(orgId: string): Promise<StaffMember[]> {
  void orgId;
  try {
    const res = await api.get<StaffResponse>('/staff');
    return (res.staff ?? []).map(toStaff);
  } catch {
    return [];
  }
}

export async function addStaff(orgId: string, data: Pick<StaffMember, 'name' | 'email' | 'role'>): Promise<StaffMember> {
  void orgId;
  void data;
  throw new Error('staff creation is not wired to production API');
}

export async function updateStaffStatus(orgId: string, staffId: number, status: MemberStatus): Promise<StaffMember> {
  void orgId;
  void staffId;
  void status;
  throw new Error('staff status update is not wired to production API');
}

export async function deleteStaff(orgId: string, staffId: number): Promise<void> {
  void orgId;
  void staffId;
  throw new Error('staff deletion is not wired to production API');
}
