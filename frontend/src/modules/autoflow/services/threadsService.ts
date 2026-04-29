import type { Thread, Message } from '../types';
import * as api from './api';

interface BackendThread {
  id: number; profile_name: string; profile_url: string;
  status: string; unread_count: number; last_message: string; last_at: string;
}
interface BackendMessage {
  id: number; direction: string; content: string; ai_generated: boolean; created_at: string;
}
interface ThreadsResponse { threads: BackendThread[]; count: number; }
interface MessagesResponse { messages: BackendMessage[]; }

const STATUS_MAP: Record<string, Thread['status']> = {
  active: 'Active', converted: 'Converted', pending: 'Pending',
  open: 'Active', closed: 'Converted',
};

function toThread(t: BackendThread): Thread {
  return {
    id: t.id,
    lead: t.profile_name || `Lead #${t.id}`,
    agent: '',
    last: t.last_message,
    time: new Date(t.last_at).toLocaleTimeString('vi', { hour: '2-digit', minute: '2-digit' }),
    unread: t.unread_count,
    status: STATUS_MAP[t.status?.toLowerCase()] ?? 'Pending',
  };
}

function toMessage(m: BackendMessage): Message {
  return {
    from: m.direction === 'outbound' ? 'agent' : 'lead',
    text: m.content,
    time: new Date(m.created_at).toLocaleTimeString('vi', { hour: '2-digit', minute: '2-digit' }),
  };
}

export async function getThreads(orgId: string): Promise<Thread[]> {
  void orgId;
  try {
    const res = await api.get<ThreadsResponse>('/threads');
    return (res.threads ?? []).map(toThread);
  } catch {
    return [];
  }
}

export async function getMessages(orgId: string, threadId: number): Promise<Message[]> {
  void orgId;
  try {
    const res = await api.get<MessagesResponse>(`/threads/${threadId}/messages`);
    return (res.messages ?? []).map(toMessage);
  } catch {
    return [];
  }
}

export async function sendMessage(orgId: string, threadId: number, content: string): Promise<Message> {
  void orgId;
  try {
    const res = await api.post<BackendMessage>(`/threads/${threadId}/messages`, { content });
    return toMessage(res);
  } catch {
    throw new Error('send message failed');
  }
}
