/**
 * In-app notifications (SaaS UX Hardening PR-1) — bell list, unread
 * badge, mark-read. Server enforces visibility (personal rows + org
 * rows for admins); the client renders exactly what it gets.
 */
import { get, post } from './api';

export type NotificationType =
  | 'workspace_invite_received'
  | 'workspace_invite_accepted'
  | 'workspace_joined'
  | 'extension_update_required'
  | string;

export interface AppNotification {
  id: number;
  org_id: number;
  user_id: number;
  type: NotificationType;
  title: string;
  body: string;
  payload_json: string;
  read_at?: string | null;
  created_at: string;
}

export interface InvitePayload {
  token?: string;
  invite_id?: string;
  org_name?: string;
  role?: string;
  inviter_name?: string;
}

export function parsePayload<T>(n: AppNotification): T | null {
  try {
    return JSON.parse(n.payload_json || '{}') as T;
  } catch {
    return null;
  }
}

export async function listNotifications(limit = 30): Promise<{ notifications: AppNotification[]; unread: number }> {
  const r = await get<{ notifications?: AppNotification[]; unread?: number }>(`/notifications?limit=${limit}`);
  return { notifications: r.notifications ?? [], unread: r.unread ?? 0 };
}

export async function markNotificationRead(id: number): Promise<void> {
  await post(`/notifications/${id}/read`, {});
}

export async function markAllNotificationsRead(): Promise<void> {
  await post('/notifications/read-all', {});
}
