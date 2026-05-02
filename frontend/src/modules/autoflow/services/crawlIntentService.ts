import { get, put } from './api';

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
  next_run_at: string;
  last_run_at?: string;
  last_task_id: string;
  last_error: string;
}

interface CrawlIntentResponse {
  intents: CrawlIntent[];
  count: number;
}

export async function getCrawlIntents(): Promise<CrawlIntent[]> {
  const res = await get<CrawlIntentResponse>('/crawl-intents');
  return res.intents ?? [];
}

export async function setCrawlIntentEnabled(id: number, enabled: boolean): Promise<void> {
  await put(`/crawl-intents/${id}/enabled`, { enabled });
}
