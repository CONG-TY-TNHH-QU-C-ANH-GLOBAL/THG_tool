import type { DataSource, DataSourceType } from '../types';
import * as api from './api';

interface DataSourcesResponse {
  sources: DataSource[];
  count: number;
}

export async function getDataSources(orgId: string): Promise<DataSource[]> {
  void orgId;
  const res = await api.get<DataSourcesResponse>('/data-sources');
  return res.sources ?? [];
}

export async function createDataSource(orgId: string, body: { type: DataSourceType; name: string; source_url: string }): Promise<DataSource> {
  void orgId;
  return api.post<DataSource>('/data-sources', body);
}

export async function syncDataSource(orgId: string, id: number): Promise<DataSource> {
  void orgId;
  return api.post<DataSource>(`/data-sources/${id}/sync`, {});
}

export async function deleteDataSource(orgId: string, id: number): Promise<void> {
  void orgId;
  await api.del(`/data-sources/${id}`);
}
