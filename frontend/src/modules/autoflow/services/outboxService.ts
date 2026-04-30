import { del, get, put } from './api';

export interface OutboundMessage {
  id: number;
  type: string;           // 'comment' | 'inbox' | 'group_post'
  platform: string;
  account_id: number;
  target_url: string;
  target_name: string;
  content: string;
  context: string;
  image_path: string;
  status: string;         // 'draft' | 'approved' | 'sent' | 'failed' | 'rejected'
  ai_model: string;
  sent_at: string;
  created_at: string;
}

export interface OutboxResponse {
  messages: OutboundMessage[];
  count: number;
  counts: Record<string, number>;
}

export async function getOutbox(params?: { type?: string; status?: string; limit?: number }): Promise<OutboxResponse> {
  const q = new URLSearchParams();
  if (params?.type) q.set('type', params.type);
  if (params?.status) q.set('status', params.status);
  if (params?.limit) q.set('limit', String(params.limit));
  const qs = q.toString();
  return get<OutboxResponse>('/outbox' + (qs ? '?' + qs : ''));
}

export async function approveOutbox(id: number): Promise<void> {
  await put(`/outbox/${id}/approve`, {});
}

export async function rejectOutbox(id: number): Promise<void> {
  await put(`/outbox/${id}/reject`, {});
}

export async function updateOutboxContent(id: number, content: string): Promise<void> {
  await put(`/outbox/${id}/content`, { content });
}

export async function deleteOutbox(id: number): Promise<void> {
  await del(`/outbox/${id}`);
}
