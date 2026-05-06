import { get } from './api';

export interface SystemInfo {
  headless: boolean;
  chrome_extension_store_url?: string;
  chrome_extension_id?: string;
}

export async function getSystemInfo(): Promise<SystemInfo> {
  return get('/system/info');
}
