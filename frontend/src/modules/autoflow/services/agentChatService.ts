import { del, get, post } from './api';

export interface AgentChatHistoryItem {
  id: number;
  source: string;
  userPrompt: string;
  aiResponse: string;
  actionTaken: string;
  actionArgs: string;
  success: boolean;
  createdAt: string;
}

interface BackendHistoryItem {
  id: number;
  source: string;
  user_prompt: string;
  ai_response: string;
  action_taken: string;
  action_args: string;
  success: boolean;
  created_at: string;
}

function toHistoryItem(item: BackendHistoryItem): AgentChatHistoryItem {
  return {
    id: item.id,
    source: item.source,
    userPrompt: item.user_prompt,
    aiResponse: item.ai_response,
    actionTaken: item.action_taken,
    actionArgs: item.action_args,
    success: item.success,
    createdAt: item.created_at,
  };
}

export async function getAgentHistory(limit = 20): Promise<AgentChatHistoryItem[]> {
  const res = await get<{ history: BackendHistoryItem[] }>(`/ai/history?limit=${limit}`);
  return (res.history ?? []).map(toHistoryItem).reverse();
}

export async function sendAgentPrompt(prompt: string, accountId?: number): Promise<string> {
  const res = await post<{ response: string }>('/ai/prompt', { prompt, account_id: accountId });
  return res.response;
}

export async function deleteAgentHistoryItem(id: number): Promise<void> {
  await del(`/ai/history/${id}`);
}

export async function clearAgentHistory(): Promise<void> {
  await del('/ai/history');
}
