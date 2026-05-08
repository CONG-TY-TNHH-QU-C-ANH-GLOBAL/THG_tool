'use client';

import { useEffect, useMemo, useState } from 'react';
import { Download, RefreshCw } from 'lucide-react';
import styles from './extension-beta.module.css';

type BetaInfo = {
  enabled?: boolean;
  name?: string;
  version?: string;
  package_url?: string;
  size_bytes?: number;
  updated_at?: string;
  error?: string;
};

function formatSize(value?: number) {
  if (!value || value < 1) return '';
  if (value < 1024 * 1024) return `${Math.round(value / 1024)} KB`;
  return `${(value / (1024 * 1024)).toFixed(1)} MB`;
}

function formatTime(value?: string) {
  if (!value) return '';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleString('vi-VN', {
    hour12: false,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export default function BetaPackageInfo() {
  const [info, setInfo] = useState<BetaInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [stamp, setStamp] = useState(() => String(Date.now()));

  useEffect(() => {
    let alive = true;
    setLoading(true);
    fetch(`/api/system/extension-beta-info?ts=${stamp}`, { cache: 'no-store' })
      .then(async (res) => {
        const body = await res.json().catch(() => ({}));
        if (!res.ok) throw new Error(body?.error || `HTTP ${res.status}`);
        return body as BetaInfo;
      })
      .then((body) => {
        if (alive) setInfo(body);
      })
      .catch((err) => {
        if (alive) setInfo({ error: err?.message || String(err) });
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [stamp]);

  const downloadURL = useMemo(() => {
    const base = info?.package_url || '/api/system/extension-beta-package';
    const joiner = base.includes('?') ? '&' : '?';
    return `${base}${joiner}ts=${stamp}`;
  }, [info?.package_url, stamp]);

  return (
    <div className={styles.versionBox}>
      <div>
        <div className={styles.versionLabel}>Beta package đang phát</div>
        <div className={styles.versionValue}>
          {loading ? 'Đang kiểm tra...' : info?.version || info?.error || 'Không có version'}
        </div>
        {info?.updated_at ? (
          <div className={styles.versionMeta}>
            Cập nhật {formatTime(info.updated_at)}
            {formatSize(info.size_bytes) ? ` · ${formatSize(info.size_bytes)}` : ''}
          </div>
        ) : null}
      </div>
      <div className={styles.versionActions}>
        <button
          type="button"
          className={styles.iconButton}
          aria-label="Kiểm tra lại beta package"
          onClick={() => setStamp(String(Date.now()))}
        >
          <RefreshCw size={17} />
        </button>
        <a className={styles.primary} href={downloadURL}>
          <Download size={18} />
          Tải đúng bản này
        </a>
      </div>
    </div>
  );
}
