import type { StaffInvite, StaffMember, MemberStatus } from '../types';
import * as api from './api';

interface BackendStaff {
  id: number; org_id?: number; name: string; email: string; role: string;
  status: string; joined: string; convs: number; converted: number; cmts: number;
}
interface StaffResponse { staff: BackendStaff[]; count: number; }

interface BackendInvite {
  id: number;
  email: string;
  role: string;
  token: string;
  invite_url: string;
  invite_full_url?: string;
  created_by: number;
  expires_at: string;
  created_at: string;
  email_status?: string;
  email_error?: string;
}
interface InviteResponse { invites: BackendInvite[]; count: number; }

function toStaff(b: BackendStaff): StaffMember {
  return {
    id: b.id,
    orgId: b.org_id ?? 0,
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

function toInvite(inv: BackendInvite): StaffInvite {
  return {
    id: inv.id,
    email: inv.email,
    role: inv.role,
    token: inv.token,
    inviteUrl: inv.invite_url,
    inviteFullUrl: inv.invite_full_url,
    createdBy: inv.created_by,
    expiresAt: inv.expires_at,
    createdAt: inv.created_at,
    emailStatus: inv.email_status,
    emailError: inv.email_error,
  };
}

export async function getStaff(orgId: string): Promise<StaffMember[]> {
  void orgId;
  const res = await api.get<StaffResponse>('/staff');
  return (res.staff ?? []).map(toStaff);
}

export async function getStaffInvites(orgId: string): Promise<StaffInvite[]> {
  void orgId;
  const res = await api.get<InviteResponse>('/org/invites');
  return (res.invites ?? []).map(toInvite);
}

export async function inviteStaff(orgId: string, data: Pick<StaffMember, 'email' | 'role'>): Promise<StaffInvite> {
  void orgId;
  const role = data.role.toLowerCase().includes('admin') || data.role.toLowerCase().includes('lead') ? 'admin' : 'sales';
  const res = await api.post<BackendInvite>('/org/invites', {
    email: data.email,
    role,
  });
  return toInvite(res);
}

export async function revokeStaffInvite(orgId: string, inviteId: number): Promise<void> {
  void orgId;
  await api.del(`/org/invites/${inviteId}`);
}

export async function resendStaffInvite(orgId: string, inviteId: number): Promise<StaffInvite> {
  void orgId;
  const res = await api.post<BackendInvite>(`/org/invites/${inviteId}/resend`, {});
  return toInvite(res);
}

export async function updateStaffStatus(orgId: string, staffId: number, status: MemberStatus): Promise<StaffMember> {
  void orgId;
  const active = status === 'Active';
  await api.put<{ status: string }>(`/auth/users/${staffId}`, { active });
  const users = await getStaff(orgId);
  return users.find(u => u.id === staffId) ?? {
    id: staffId,
    orgId: 0,
    name: '',
    email: '',
    role: 'sales',
    status,
    joined: '',
    convs: 0,
    converted: 0,
    cmts: 0,
  };
}

export async function deleteStaff(orgId: string, staffId: number): Promise<void> {
  void orgId;
  await api.del(`/auth/users/${staffId}`);
}
