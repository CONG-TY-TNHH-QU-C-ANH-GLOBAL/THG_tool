import type { DataSource, FileRecord } from '../../types';
import { cardStyle, theme } from '../../constants/styles';

interface DataStatsGridProps {
  files: FileRecord[];
  sources: DataSource[];
}

function formatBytes(bytes: number) {
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  if (bytes >= 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${bytes} B`;
}

export default function DataStatsGrid({ files, sources }: DataStatsGridProps) {
  const totalSize = files.reduce((sum, f) => sum + (f.sizeBytes || 0), 0);
  const syncedSources = sources.filter(s => s.status === 'synced').length;

  const stats = [
    { label: 'Tệp đã tải', value: files.length, color: '#fff' },
    { label: 'Tổng dung lượng', value: formatBytes(totalSize), color: theme.primaryLight },
    { label: 'Nguồn đã sync', value: `${syncedSources}/${sources.length}`, color: '#4ade80' },
  ];

  return (
    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3,1fr)', gap: 11 }}>
      {stats.map(s => (
        <div key={s.label} style={cardStyle()}>
          <p style={{ color: theme.textFaint, fontSize: 11, marginBottom: 4 }}>{s.label}</p>
          <p style={{ fontSize: 22, fontWeight: 700, color: s.color }}>{s.value}</p>
        </div>
      ))}
    </div>
  );
}
