import { get, post, put } from './api';

export interface CrawlIntent {
  id: number;
  org_id: number;
  account_id: number;
  name: string;
  prompt: string;
  intent: string;
  source_type: string;
  source_url: string;
  source_label: string;
  keywords: string[];
  interval_minutes: number;
  max_items: number;
  enabled: boolean;
  status: 'active' | 'paused' | 'archived' | 'failed' | 'cooldown';
  next_run_at: string;
  last_run_at?: string;
  last_task_id: string;
  last_error: string;
}

interface CrawlIntentResponse {
  intents: CrawlIntent[];
  count: number;
}

export interface CreateMissionInput {
  prompt: string;
  source_url: string;
  name?: string;
  interval_minutes?: number;
  max_items?: number;
  keywords?: string[];
  account_id?: number;
}

export interface CreateMissionResult {
  intent: CrawlIntent;
  created: boolean;
}

export async function getCrawlIntents(): Promise<CrawlIntent[]> {
  const res = await get<CrawlIntentResponse>('/crawl-intents');
  return res.intents ?? [];
}

export async function createMission(input: CreateMissionInput): Promise<CreateMissionResult> {
  return post<CreateMissionResult>('/crawl-intents', input);
}

export async function pauseMission(id: number): Promise<void> {
  await post(`/crawl-intents/${id}/pause`, {});
}

export async function resumeMission(id: number): Promise<void> {
  await post(`/crawl-intents/${id}/resume`, {});
}

export async function archiveMission(id: number): Promise<void> {
  await post(`/crawl-intents/${id}/archive`, {});
}

export async function setCrawlIntentEnabled(id: number, enabled: boolean): Promise<void> {
  await put(`/crawl-intents/${id}/enabled`, { enabled });
}
