import type { StaffMember, MemberStatus } from '../types';
import { MOCK_STAFF } from './mockData';
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
    return [...MOCK_STAFF];
  }
}

export async function addStaff(orgId: string, data: Pick<StaffMember, 'name' | 'email' | 'role'>): Promise<StaffMember> {
  void orgId;
  const member: StaffMember = {
    ...data, id: Date.now(), status: 'Active',
    joined: new Date().toLocaleDateString('vi'), convs: 0, converted: 0, cmts: 0,
  };
  MOCK_STAFF.push(member);
  return member;
}

export async function updateStaffStatus(orgId: string, staffId: number, status: MemberStatus): Promise<StaffMember> {
  void orgId;
  const idx = MOCK_STAFF.findIndex(s => s.id === staffId);
  if (idx !== -1) MOCK_STAFF[idx] = { ...MOCK_STAFF[idx], status };
  return MOCK_STAFF[idx];
}

export async function deleteStaff(orgId: string, staffId: number): Promise<void> {
  void orgId;
  const idx = MOCK_STAFF.findIndex(s => s.id === staffId);
  if (idx !== -1) MOCK_STAFF.splice(idx, 1);
}
