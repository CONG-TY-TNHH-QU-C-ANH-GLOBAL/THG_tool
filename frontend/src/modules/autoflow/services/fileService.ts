import type { FileRecord } from '../types';
import * as api from './api';

interface BackendFile {
  id: number; name: string; size_bytes: number; mime_type: string; created_at: string;
}
interface FilesResponse { files: BackendFile[]; count: number; }

function toFileRecord(b: BackendFile): FileRecord {
  const kb = b.size_bytes / 1024;
  const size = kb >= 1024 ? `${(kb / 1024).toFixed(1)} MB` : `${kb.toFixed(0)} KB`;
  return {
    id: b.id,
    name: b.name,
    size,
    date: new Date(b.created_at).toLocaleDateString('vi', { day: '2-digit', month: '2-digit' }),
  };
}

export async function getFiles(orgId: string): Promise<FileRecord[]> {
  void orgId;
  try {
    const res = await api.get<FilesResponse>('/files');
    return (res.files ?? []).map(toFileRecord);
  } catch {
    return [];
  }
}

export async function uploadFile(orgId: string, file: File): Promise<FileRecord> {
  void orgId;
  const res = await api.upload<BackendFile>('/files', file);
  return toFileRecord(res);
}

export async function deleteFile(orgId: string, fileId: number): Promise<void> {
  void orgId;
  await api.del(`/files/${fileId}`);
}
