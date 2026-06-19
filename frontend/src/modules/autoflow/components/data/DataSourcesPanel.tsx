import { useState } from 'react';
import { Database, ExternalLink, FileSpreadsheet, FolderOpen, RefreshCw, Trash2 } from 'lucide-react';
import type { DataSource, DataSourceType } from '../../types';
import { Badge, Row } from '../ui';
import { alpha, cardStyle, inputStyle, primaryBtn, secondaryBtn, theme } from '../../constants/styles';

interface DataSourcesPanelProps {
  sources: DataSource[];
  isLoading: boolean;
  isSyncing: number | null;
  onAdd: (body: { type: DataSourceType; name: string; source_url: string }) => Promise<DataSource>;
  onSync: (id: number) => Promise<DataSource>;
  onRemove: (id: number) => Promise<void>;
}

function formatDate(value?: string) {
  if (!value) return 'Chưa sync';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString('vi-VN', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
}

function sourceLabel(type: DataSourceType) {
  return type === 'google_sheet' ? 'Google Sheet' : 'Google Drive';
}

export default function DataSourcesPanel({ sources, isLoading, isSyncing, onAdd, onSync, onRemove }: Readonly<DataSourcesPanelProps>) {
  const [form, setForm] = useState<{ type: DataSourceType; name: string; source_url: string }>({
    type: 'google_sheet',
    name: '',
    source_url: '',
  });
  const [message, setMessage] = useState('');

  const submit = async () => {
    setMessage('');
    if (!form.source_url.trim()) {
      setMessage('Dán link Google Sheet hoặc Drive trước khi thêm nguồn.');
      return;
    }
    try {
      await onAdd({ ...form, name: form.name.trim(), source_url: form.source_url.trim() });
      setForm({ type: 'google_sheet', name: '', source_url: '' });
      setMessage('Đã thêm nguồn dữ liệu.');
    } catch (err) {
      setMessage(err instanceof Error ? err.message : 'Không thêm được nguồn dữ liệu.');
    }
  };

  return (
    <div style={cardStyle()}>
      <Row style={{ justifyContent: 'space-between', gap: 12, marginBottom: 14 }}>
        <Row style={{ gap: 8 }}>
          <Database size={15} color={theme.primaryLight} />
          <p style={{ color: theme.text, fontWeight: 600, fontSize: 13 }}>Data connectors</p>
        </Row>
        <span style={{ color: theme.textFaint, fontSize: 12 }}>{sources.length} nguồn</span>
      </Row>

      <div style={{ display: 'grid', gridTemplateColumns: '150px 1fr 1.4fr auto', gap: 9, alignItems: 'end', marginBottom: 12 }}>
        <div>
          <p style={{ color: theme.textFaint, fontSize: 11, marginBottom: 5 }}>Loại nguồn</p>
          <select
            value={form.type}
            onChange={e => setForm(p => ({ ...p, type: e.target.value as DataSourceType }))}
            style={{ ...inputStyle, padding: '9px 10px', fontSize: 12 }}
          >
            <option value="google_sheet">Google Sheet</option>
            <option value="google_drive">Google Drive</option>
          </select>
        </div>
        <div>
          <p style={{ color: theme.textFaint, fontSize: 11, marginBottom: 5 }}>Tên</p>
          <input
            value={form.name}
            onChange={e => setForm(p => ({ ...p, name: e.target.value }))}
            placeholder="Bảng giá, Media folder..."
            style={{ ...inputStyle, padding: '9px 10px', fontSize: 12 }}
          />
        </div>
        <div>
          <p style={{ color: theme.textFaint, fontSize: 11, marginBottom: 5 }}>Link Google</p>
          <input
            value={form.source_url}
            onChange={e => setForm(p => ({ ...p, source_url: e.target.value }))}
            placeholder="https://docs.google.com/spreadsheets/... hoặc https://drive.google.com/drive/folders/..."
            style={{ ...inputStyle, padding: '9px 10px', fontSize: 12 }}
          />
        </div>
        <button onClick={submit} style={primaryBtn({ padding: '9px 14px', fontSize: 12 })}>Thêm nguồn</button>
      </div>

      {message && <p style={{ color: message.startsWith('Đã') ? theme.green : theme.red, fontSize: 12, marginBottom: 12 }}>{message}</p>}

      <div style={{ border: `1px solid ${theme.border}`, borderRadius: 10, overflow: 'hidden' }}>
        {isLoading ? (
          <p style={{ color: theme.textMuted, fontSize: 13, padding: 18, textAlign: 'center' }}>Đang tải nguồn dữ liệu...</p>
        ) : sources.length === 0 ? (
          <p style={{ color: theme.textMuted, fontSize: 13, padding: 18, textAlign: 'center' }}>Chưa có connector. Bắt đầu bằng bảng giá Google Sheet hoặc folder media trên Drive.</p>
        ) : sources.map(src => (
          <div key={src.id} style={{ padding: 13, borderBottom: `1px solid ${theme.borderAlt}` }}>
            <Row style={{ gap: 10, alignItems: 'flex-start' }}>
              <div style={{ width: 34, height: 34, background: alpha(theme.primary, 10), border: `1px solid ${alpha(theme.primary, 22)}`, borderRadius: 9, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                {src.type === 'google_sheet' ? <FileSpreadsheet size={16} color={theme.primaryLight} /> : <FolderOpen size={16} color={theme.primaryLight} />}
              </div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <Row style={{ gap: 8, flexWrap: 'wrap' }}>
                  <p style={{ color: theme.text, fontSize: 13, fontWeight: 600 }}>{src.name}</p>
                  <Badge label={src.status} />
                  <span style={{ color: theme.textFaint, fontSize: 11 }}>{sourceLabel(src.type)} · {src.item_count} items · {formatDate(src.last_sync_at)}</span>
                </Row>
                {src.summary && <p style={{ color: theme.textMuted, fontSize: 12, lineHeight: 1.5, marginTop: 6, whiteSpace: 'pre-wrap', maxHeight: 72, overflow: 'hidden' }}>{src.summary}</p>}
                {src.last_error && <p style={{ color: theme.red, fontSize: 12, marginTop: 6 }}>{src.last_error}</p>}
              </div>
              <Row style={{ gap: 6, flexShrink: 0 }}>
                <a href={src.source_url} target="_blank" rel="noopener noreferrer" style={secondaryBtn({ padding: '6px 9px', fontSize: 11 })}>
                  <ExternalLink size={12} />
                </a>
                <button disabled={isSyncing === src.id} onClick={() => onSync(src.id)} style={secondaryBtn({ padding: '6px 9px', fontSize: 11, opacity: isSyncing === src.id ? 0.6 : 1 })}>
                  <RefreshCw size={12} />
                </button>
                <button onClick={() => onRemove(src.id)} style={secondaryBtn({ padding: '6px 9px', fontSize: 11, color: theme.red })}>
                  <Trash2 size={12} />
                </button>
              </Row>
            </Row>
          </div>
        ))}
      </div>
    </div>
  );
}
