import { useState, useEffect, useCallback } from 'react';
import type { FileRecord } from '../types';
import { getFiles, uploadFile, deleteFile } from '../services/fileService';

export function useFiles(orgId: string) {
  const [files, setFiles] = useState<FileRecord[]>([]);
  const [isUploading, setIsUploading] = useState(false);

  useEffect(() => {
    let cancelled = false;
    getFiles(orgId).then(data => { if (!cancelled) setFiles(data); });
    return () => { cancelled = true; };
  }, [orgId]);

  const upload = useCallback(async (file: File) => {
    setIsUploading(true);
    try {
      const record = await uploadFile(orgId, file);
      setFiles(prev => [...prev, record]);
    } finally {
      setIsUploading(false);
    }
  }, [orgId]);

  const remove = useCallback(async (fileId: number) => {
    await deleteFile(orgId, fileId);
    setFiles(prev => prev.filter(f => f.id !== fileId));
  }, [orgId]);

  return { files, isUploading, upload, remove };
}
